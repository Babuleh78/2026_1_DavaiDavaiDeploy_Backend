package repo

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	uuid "github.com/satori/go.uuid"
	"log/slog"
	"math"
	"time"
)

func (u *UserRepository) CreateDance(ctx context.Context, id, title, status, difficulty, videoPath string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("create_dance", func() error {
		_, e := u.db.Exec(ctx, CreateDanceQuery, id, title, status, difficulty, videoPath)
		return e
	})
	if err != nil {
		logger.Error("failed to create dance", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetDanceVideoPath(ctx context.Context, id string) (string, error) {
	var videoPath string
	err := appmetrics.ObserveDBQuery("get_dance_video_path", func() error {
		return u.db.QueryRow(ctx, GetDanceVideoPathQuery, id).Scan(&videoPath)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", users.ErrorNotFound
		}
		return "", users.ErrorInternalServerError
	}
	return videoPath, nil
}

func (u *UserRepository) UpdateDanceStatus(ctx context.Context, id, status string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("update_dance_status", func() error {
		_, e := u.db.Exec(ctx, UpdateDanceStatusQuery, status, id)
		return e
	})
	if err != nil {
		logger.Error("failed to update dance status", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetDanceStatus(ctx context.Context, id string) (string, error) {
	var status string
	err := appmetrics.ObserveDBQuery("get_dance_status", func() error {
		return u.db.QueryRow(ctx, GetDanceStatusQuery, id).Scan(&status)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", users.ErrorNotFound
		}
		return "", users.ErrorInternalServerError
	}
	return status, nil
}

func (u *UserRepository) GetDanceCreatedAt(ctx context.Context, danceID string) (time.Time, error) {
	var createdAt time.Time
	err := appmetrics.ObserveDBQuery("get_dance_created_at", func() error {
		return u.db.QueryRow(ctx, "SELECT created_at FROM dances WHERE id = $1", danceID).Scan(&createdAt)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return time.Time{}, users.ErrorNotFound
		}
		return time.Time{}, users.ErrorInternalServerError
	}
	return createdAt, nil
}

func (u *UserRepository) GetPublishedDanceIDs(ctx context.Context, ids []string) (map[string]struct{}, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	result := make(map[string]struct{})
	if len(ids) == 0 {
		return result, nil
	}
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_published_dance_ids", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetPublishedDanceIDsQuery, ids)
		return e
	})
	if err != nil {
		logger.Error("failed to query published dance ids", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			logger.Error("failed to scan published dance id", "error", err)
			return nil, users.ErrorInternalServerError
		}
		result[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetPublishedDanceIDs", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) UpdateDanceDifficulty(ctx context.Context, danceID, difficulty string, difficultyScore int) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("update_dance_difficulty", func() error {
		_, e := u.db.Exec(ctx, UpdateDanceDifficultyQuery, danceID, difficulty, difficultyScore)
		return e
	})
	if err != nil {
		logger.Error("failed to update dance difficulty", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func danceCatalogOrderClause(sort string) string {
	switch sort {
	case "newest", "new":
		return "d.created_at DESC NULLS LAST"
	default:
		return popular7dExpr
	}
}

func danceCatalogDifficultyHaving(sort string, threshold int) string {
	expr := fmt.Sprintf(
		`CASE WHEN COALESCE(MAX(dr.rating_count), 0) >= %d
              THEN ROUND((MAX(dr.avg_diff) - 2) / 8.0 * 100)::int
              ELSE d.difficulty_score END`,
		threshold,
	)
	signal := fmt.Sprintf(
		`(EXISTS (SELECT 1 FROM dance_uploads du WHERE du.dance_id = d.id)
          OR COALESCE(MAX(dr.rating_count), 0) >= %d)`,
		threshold,
	)
	switch sort {
	case "easy":
		return "HAVING " + expr + " < 34 AND " + signal
	case "medium":
		return "HAVING " + expr + " BETWEEN 34 AND 66 AND " + signal
	case "hard":
		return "HAVING " + expr + " >= 67 AND " + signal
	default:
		return ""
	}
}

func difficultyLabelFromScore(score int) string {
	switch {
	case score < 34:
		return "easy"
	case score < 67:
		return "medium"
	default:
		return "hard"
	}
}

func (u *UserRepository) GetDanceCatalog(ctx context.Context, sort, search string, page, limit int) ([]models.DanceCatalogItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	offset := (page - 1) * limit
	searchFilter := ""
	args := []interface{}{limit, offset}
	if search != "" {
		searchFilter = " AND d.title ILIKE $3"
		args = append(args, "%"+search+"%")
	}

	query := fmt.Sprintf(`
        SELECT
            d.id,
            d.title,
            d.created_at,
            COALESCE(COUNT(a.id), 0)::bigint                    AS attempt_count,
            COALESCE(AVG(a.score), 0)::float                    AS avg_score,
            COALESCE(MAX(vc.view_count), 0)::bigint             AS view_count,
            COALESCE(MAX(lk.like_count), 0)::bigint             AS like_count,
            d.duration_sec,
            (COALESCE(MAX(dr.rating_count), 0) >= %d)           AS difficulty_by_users,
            CASE WHEN COALESCE(MAX(dr.rating_count), 0) >= %d
                 THEN ROUND((MAX(dr.avg_diff) - 2) / 8.0 * 100)::int
                 ELSE d.difficulty_score END                    AS effective_difficulty_score,
            EXISTS (SELECT 1 FROM dance_uploads du WHERE du.dance_id = d.id) AS has_author
        FROM dances d
        LEFT JOIN dance_attempts a ON a.dance_id = d.id
        LEFT JOIN (
            SELECT dance_id, COUNT(*)::bigint AS view_count
            FROM dance_views
            GROUP BY dance_id
        ) vc ON vc.dance_id = d.id
        LEFT JOIN (
            SELECT dance_id, COUNT(DISTINCT user_id)::bigint AS like_count
            FROM dance_likes
            GROUP BY dance_id
        ) lk ON lk.dance_id = d.id
        LEFT JOIN (
            SELECT video_id,
                   COUNT(*)::int AS rating_count,
                   AVG((physical + speed + coordination + repeatability) / 4.0) AS avg_diff
            FROM dance_ratings
            GROUP BY video_id
        ) dr ON dr.video_id = d.id
        WHERE d.status = 'published'%s
        GROUP BY d.id, d.title, d.difficulty_score, d.duration_sec, d.created_at
        %s
        ORDER BY %s
        LIMIT $1 OFFSET $2
    `,
		minRatingsForCrowdDifficulty,
		minRatingsForCrowdDifficulty,
		searchFilter,
		danceCatalogDifficultyHaving(sort, minRatingsForCrowdDifficulty),
		danceCatalogOrderClause(sort),
	)

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_dance_catalog", func() error {
		var e error
		rows, e = u.db.Query(ctx, query, args...)
		return e
	})
	if err != nil {
		logger.Error("failed to get dance catalog", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.DanceCatalogItem
	for rows.Next() {
		var item models.DanceCatalogItem
		var hasAuthor bool
		if err := rows.Scan(&item.ID, &item.Title, &item.CreatedAt, &item.AttemptCount, &item.AvgScore, &item.ViewCount, &item.LikeCount, &item.DurationSec, &item.DifficultyByUsers, &item.DifficultyScore, &hasAuthor); err != nil {
			logger.Error("failed to scan catalog item", "error", err)
			return nil, users.ErrorInternalServerError
		}

		if hasAuthor || item.DifficultyByUsers {
			item.Difficulty = difficultyLabelFromScore(item.DifficultyScore)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDanceCatalog", "error", err)
		return nil, users.ErrorInternalServerError
	}
	if items == nil {
		items = []models.DanceCatalogItem{}
	}
	return items, nil
}

func (u *UserRepository) GetDanceCatalogCount(ctx context.Context, sort, search string) (int, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	having := danceCatalogDifficultyHaving(sort, minRatingsForCrowdDifficulty)

	if having == "" {
		var count int
		var err error
		if search != "" {
			err = appmetrics.ObserveDBQuery("get_dance_catalog_count_search", func() error {
				return u.db.QueryRow(ctx,
					"SELECT COUNT(*)::int FROM dances WHERE status = 'published' AND title ILIKE $1",
					"%"+search+"%",
				).Scan(&count)
			})
		} else {
			err = appmetrics.ObserveDBQuery("get_dance_catalog_count", func() error {
				return u.db.QueryRow(ctx, GetDanceCatalogCountQuery).Scan(&count)
			})
		}
		if err != nil {
			logger.Error("failed to get catalog count", "error", err)
			return 0, users.ErrorInternalServerError
		}
		return count, nil
	}

	searchFilter := ""
	args := []interface{}{}
	if search != "" {
		searchFilter = " AND d.title ILIKE $1"
		args = append(args, "%"+search+"%")
	}
	query := fmt.Sprintf(`
        SELECT COUNT(*)::int FROM (
            SELECT d.id
            FROM dances d
            LEFT JOIN (
                SELECT video_id,
                       COUNT(*)::int AS rating_count,
                       AVG((physical + speed + coordination + repeatability) / 4.0) AS avg_diff
                FROM dance_ratings
                GROUP BY video_id
            ) dr ON dr.video_id = d.id
            WHERE d.status = 'published'%s
            GROUP BY d.id, d.difficulty_score
            %s
        ) t
    `, searchFilter, having)

	var count int
	err := appmetrics.ObserveDBQuery("get_dance_catalog_count_filtered", func() error {
		return u.db.QueryRow(ctx, query, args...).Scan(&count)
	})
	if err != nil {
		logger.Error("failed to get filtered catalog count", "error", err)
		return 0, users.ErrorInternalServerError
	}
	return count, nil
}

func (u *UserRepository) GetDancesEnrichedInfo(ctx context.Context, ids []string) (map[string]models.DanceEnrichedInfo, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_dances_enriched_info", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetDancesEnrichedInfoQuery, ids)
		return e
	})
	if err != nil {
		logger.Error("failed to get enriched info", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := make(map[string]models.DanceEnrichedInfo, len(ids))
	for rows.Next() {
		var id string
		var info models.DanceEnrichedInfo
		if err := rows.Scan(&id, &info.Title, &info.AttemptCount, &info.AvgScore, &info.ViewCount, &info.LikeCount); err != nil {
			logger.Error("failed to scan enriched info", "error", err)
			return nil, users.ErrorInternalServerError
		}
		result[id] = info
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDancesEnrichedInfo", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) GetDanceTrending(ctx context.Context) ([]models.DanceCatalogItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_dance_trending", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetDanceTrendingQuery)
		return e
	})
	if err != nil {
		logger.Error("failed to get trending dances", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.DanceCatalogItem
	for rows.Next() {
		var item models.DanceCatalogItem
		if err := rows.Scan(&item.ID, &item.Title, &item.AttemptCount, &item.AvgScore, &item.ViewCount, &item.LikeCount); err != nil {
			logger.Error("failed to scan trending item", "error", err)
			return nil, users.ErrorInternalServerError
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDanceTrending", "error", err)
		return nil, users.ErrorInternalServerError
	}
	if items == nil {
		items = []models.DanceCatalogItem{}
	}
	return items, nil
}

func (u *UserRepository) GetDanceStats(ctx context.Context, danceID string) (*models.DanceStats, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var s models.DanceStats
	err := appmetrics.ObserveDBQuery("get_dance_stats", func() error {
		return u.db.QueryRow(ctx, GetDanceStatsQuery, danceID).Scan(
			&s.AttemptCount, &s.AvgScore, &s.TopScore, &s.TopUser, &s.ViewCount,
		)
	})
	if err != nil {
		logger.Error("failed to get dance stats", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return &s, nil
}

func (u *UserRepository) GetUploadedDancesByUser(ctx context.Context, userID uuid.UUID) ([]models.UploadedDance, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	query := fmt.Sprintf(GetUploadedDancesByUserQuery, minRatingsForCrowdDifficulty, minRatingsForCrowdDifficulty)
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_uploaded_dances_by_user", func() error {
		var e error
		rows, e = u.db.Query(ctx, query, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to get uploaded dances", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()
	var result []models.UploadedDance
	for rows.Next() {
		var d models.UploadedDance
		var effectiveScore int
		if err := rows.Scan(
			&d.DanceID, &d.Title, &d.Status, &d.VideoPath, &d.UploadedAt,
			&d.AttemptCount, &d.AvgScore, &d.LikeCount, &d.ViewCount,
			&d.DifficultyByUsers, &effectiveScore,
		); err != nil {
			logger.Error("failed to scan uploaded dance", "error", err)
			return nil, users.ErrorInternalServerError
		}
		d.Difficulty = difficultyLabelFromScore(effectiveScore)
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetUploadedDancesByUser", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) DeleteDanceAndRelated(ctx context.Context, danceID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	queries := []string{
		"DELETE FROM dance_likes WHERE dance_id = $1",
		"DELETE FROM dance_attempts WHERE dance_id = $1",
		"DELETE FROM saved_attempts WHERE dance_id = $1",
		"DELETE FROM dance_views WHERE dance_id = $1",
		"DELETE FROM dance_ratings WHERE video_id = $1",
		"DELETE FROM dance_features WHERE video_id = $1",
		"DELETE FROM notifications WHERE dance_id = $1",
		"DELETE FROM search_history WHERE dance_id = $1",
		"DELETE FROM dances WHERE id = $1",
	}

	pool, ok := u.db.(*pgxpool.Pool)
	if !ok {
		for _, q := range queries {
			qCopy := q
			if err := appmetrics.ObserveDBQuery("delete_dance_and_related", func() error {
				_, e := u.db.Exec(ctx, qCopy, danceID)
				return e
			}); err != nil {
				logger.Error("delete step failed", "query", qCopy, "dance_id", danceID, "error", err)
				return users.ErrorInternalServerError
			}
		}
		return nil
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		logger.Error("failed to begin transaction for DeleteDanceAndRelated", "error", err)
		return users.ErrorInternalServerError
	}
	defer tx.Rollback(ctx)

	for _, q := range queries {
		qCopy := q
		if err := appmetrics.ObserveDBQuery("delete_dance_and_related", func() error {
			_, e := tx.Exec(ctx, qCopy, danceID)
			return e
		}); err != nil {
			logger.Error("delete step failed", "query", qCopy, "dance_id", danceID, "error", err)
			return users.ErrorInternalServerError
		}
	}

	if err := tx.Commit(ctx); err != nil {
		logger.Error("failed to commit transaction for DeleteDanceAndRelated", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetEffectiveDanceDifficulty(ctx context.Context, danceID string) (string, bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	const query = `
        SELECT
            d.difficulty_score,
            COALESCE((
                SELECT COUNT(*)::int FROM dance_ratings WHERE video_id = $1
            ), 0) AS rating_count,
            COALESCE((
                SELECT AVG((physical + speed + coordination + repeatability) / 4.0)
                FROM dance_ratings WHERE video_id = $1
            ), 0)::float AS avg_diff,
            EXISTS (SELECT 1 FROM dance_uploads du WHERE du.dance_id = $1) AS has_author
        FROM dances d
        WHERE d.id = $1
    `
	var score, ratingCount int
	var avgDiff float64
	var hasAuthor bool
	err := appmetrics.ObserveDBQuery("get_effective_dance_difficulty", func() error {
		return u.db.QueryRow(ctx, query, danceID).Scan(&score, &ratingCount, &avgDiff, &hasAuthor)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, users.ErrorNotFound
		}
		logger.Error("failed to get effective difficulty", "error", err)
		return "", false, users.ErrorInternalServerError
	}
	if ratingCount >= minRatingsForCrowdDifficulty {
		score = int(math.Round((avgDiff - 2) / 8.0 * 100))
		return difficultyLabelFromScore(score), true, nil
	}

	if !hasAuthor {
		return "", false, nil
	}
	return difficultyLabelFromScore(score), false, nil
}

func (u *UserRepository) UpdateDanceTitle(ctx context.Context, danceID string, title string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("update_dance_title", func() error {
		_, e := u.db.Exec(ctx, UpdateDanceTitleQuery, danceID, title)
		return e
	})
	if err != nil {
		logger.Error("failed to update dance title", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) UpdateDanceDuration(ctx context.Context, danceID string, durationSec int) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("update_dance_duration", func() error {
		_, e := u.db.Exec(ctx, UpdateDanceDurationQuery, durationSec, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to update dance duration", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) RecordDanceView(ctx context.Context, danceID string, viewerID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	if danceID == "" || viewerID == "" {
		return users.ErrorBadRequest
	}
	err := appmetrics.ObserveDBQuery("record_dance_view", func() error {
		_, e := u.db.Exec(ctx, RecordDanceViewQuery, danceID, viewerID)
		return e
	})
	if err != nil {
		logger.Error("failed to record dance view", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetDanceViewCount(ctx context.Context, danceID string) (int64, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var count int64
	err := appmetrics.ObserveDBQuery("get_dance_view_count", func() error {
		return u.db.QueryRow(ctx, GetDanceViewCountQuery, danceID).Scan(&count)
	})
	if err != nil {
		logger.Error("failed to get dance view count", "error", err)
		return 0, users.ErrorInternalServerError
	}
	return count, nil
}

func (u *UserRepository) LinkDanceUpload(ctx context.Context, userID uuid.UUID, danceID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("link_dance_upload", func() error {
		_, e := u.db.Exec(ctx, LinkDanceUploadQuery, userID, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to link dance upload", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetDanceUploaders(ctx context.Context, danceID string) ([]uuid.UUID, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_dance_uploaders", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetDanceUploadersQuery, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to query dance uploaders", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			logger.Error("failed to scan uploader id", "error", err)
			return nil, users.ErrorInternalServerError
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDanceUploaders", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return ids, nil
}

func (u *UserRepository) GetDanceAuthor(ctx context.Context, danceID string) (*models.DanceAuthor, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var author models.DanceAuthor
	err := appmetrics.ObserveDBQuery("get_dance_author", func() error {
		return u.db.QueryRow(ctx, GetDanceAuthorQuery, danceID).Scan(&author.ID, &author.Login, &author.Avatar)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.Error("failed to query dance author", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return &author, nil
}

func (u *UserRepository) SetDanceModerationReason(ctx context.Context, danceID, reason string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("set_dance_moderation_reason", func() error {
		_, e := u.db.Exec(ctx, SetDanceModerationReasonQuery, reason, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to set moderation reason", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetDanceModerationReason(ctx context.Context, danceID string) (string, error) {
	var reason string
	err := appmetrics.ObserveDBQuery("get_dance_moderation_reason", func() error {
		return u.db.QueryRow(ctx, GetDanceModerationReasonQuery, danceID).Scan(&reason)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", users.ErrorInternalServerError
	}
	return reason, nil
}

func (u *UserRepository) GetDanceTitleByID(ctx context.Context, danceID string) (string, error) {
	var title string
	err := appmetrics.ObserveDBQuery("get_dance_title_by_id", func() error {
		return u.db.QueryRow(ctx, "SELECT title FROM dances WHERE id=$1", danceID).Scan(&title)
	})
	if err != nil {
		return "", err
	}
	return title, nil
}
