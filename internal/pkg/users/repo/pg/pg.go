package repo

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgtype/pgxtype"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	uuid "github.com/satori/go.uuid"
)

type UserRepository struct {
	db pgxtype.Querier
}

func NewUserRepository(db pgxtype.Querier) *UserRepository {
	return &UserRepository{db: db}
}

func (u *UserRepository) GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("get_user_by_id", func() error {
		return u.db.QueryRow(ctx, GetUserByIDQuery, id).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("user not exists")
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("failed to scan user: " + err.Error())
		return models.User{}, users.ErrorInternalServerError
	}

	logger.Info("succesfully got user by id from db")
	return user, nil
}

func (u *UserRepository) GetUserByLogin(ctx context.Context, login string) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("get_user_by_login", func() error {
		return u.db.QueryRow(ctx, GetUserByLoginQuery, login).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("user not exists")
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("failed to scan user: " + err.Error())
		return models.User{}, users.ErrorInternalServerError
	}

	logger.Info("succesfully got user by login from db")
	return user, nil
}

func (u *UserRepository) UpdateUserPassword(ctx context.Context, version int, userID uuid.UUID, passwordHash []byte) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("update_user_password", func() error {
		_, e := u.db.Exec(ctx, UpdateUserPasswordQuery, passwordHash, version, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to update password: " + err.Error())
		return users.ErrorInternalServerError
	}

	logger.Info("succesfully updated password of user from db")
	return nil
}

func (u *UserRepository) UpdateUserProfile(ctx context.Context, userID uuid.UUID, login *string, avatar *string, bumpVersion bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("update_user_profile", func() error {
		_, e := u.db.Exec(ctx, UpdateUserProfileQuery, login, avatar, bumpVersion, userID)
		return e
	})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint") || strings.Contains(msg, "23505") {
			logger.Info("login already taken")
			return users.ErrorBadRequest
		}
		logger.Error("failed to update profile: " + msg)
		return users.ErrorInternalServerError
	}
	logger.Info("successfully updated profile")
	return nil
}

func (u *UserRepository) AddToHistory(ctx context.Context, userID uuid.UUID, danceID string, sourceURL string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("add_to_history", func() error {
		_, e := u.db.Exec(ctx, AddToHistoryQuery, userID, danceID, sourceURL)
		return e
	})
	if err != nil {
		logger.Error("failed to add to history: " + err.Error())
		return users.ErrorInternalServerError
	}
	logger.Info("successfully added to search history")
	return nil
}

func (u *UserRepository) GetHistory(ctx context.Context, userID uuid.UUID) ([]models.SearchHistoryItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_history", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetHistoryQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to get history: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.SearchHistoryItem
	for rows.Next() {
		var item models.SearchHistoryItem
		if err := rows.Scan(&item.ID, &item.UserID, &item.DanceID, &item.Name, &item.SourceURL, &item.CreatedAt, &item.DanceTitle, &item.Score); err != nil {
			logger.Error("failed to scan history item: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetHistory: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) DeleteFromHistory(ctx context.Context, historyID uuid.UUID, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rowsAffected int64
	err := appmetrics.ObserveDBQuery("delete_from_history", func() error {
		result, e := u.db.Exec(ctx, DeleteFromHistoryQuery, historyID, userID)
		if e == nil {
			rowsAffected = result.RowsAffected()
		}
		return e
	})
	if err != nil {
		logger.Error("failed to delete from history: " + err.Error())
		return users.ErrorInternalServerError
	}
	if rowsAffected == 0 {
		return users.ErrorNotFound
	}
	logger.Info("successfully deleted from search history")
	return nil
}

func (u *UserRepository) UpdateHistoryName(ctx context.Context, historyID uuid.UUID, userID uuid.UUID, name string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rowsAffected int64
	err := appmetrics.ObserveDBQuery("update_history_name", func() error {
		result, e := u.db.Exec(ctx, UpdateHistoryNameQuery, name, historyID, userID)
		if e == nil {
			rowsAffected = result.RowsAffected()
		}
		return e
	})
	if err != nil {
		logger.Error("failed to update history name: " + err.Error())
		return users.ErrorInternalServerError
	}
	if rowsAffected == 0 {
		return users.ErrorNotFound
	}
	logger.Info("successfully updated history item name")
	return nil
}

func (u *UserRepository) ToggleLike(ctx context.Context, userID uuid.UUID, danceID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var inserted bool
	err := appmetrics.ObserveDBQuery("toggle_like_insert", func() error {
		return u.db.QueryRow(ctx, ToggleLikeQuery, userID, danceID).Scan(&inserted)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = appmetrics.ObserveDBQuery("toggle_like_delete", func() error {
				_, e := u.db.Exec(ctx, DeleteLikeQuery, userID, danceID)
				return e
			})
			if err != nil {
				logger.Error("failed to delete like: " + err.Error())
				return false, users.ErrorInternalServerError
			}
			logger.Info("like removed")
			return false, nil
		}
		logger.Error("failed to toggle like: " + err.Error())
		return false, users.ErrorInternalServerError
	}

	logger.Info("like added")
	return true, nil
}

func (u *UserRepository) GetLikesCount(ctx context.Context, danceID string) (int64, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var count int64
	err := appmetrics.ObserveDBQuery("get_likes_count", func() error {
		return u.db.QueryRow(ctx, GetLikesCountQuery, danceID).Scan(&count)
	})
	if err != nil {
		logger.Error("failed to get likes count: " + err.Error())
		return 0, users.ErrorInternalServerError
	}
	return count, nil
}

func (u *UserRepository) IsLikedByUser(ctx context.Context, userID uuid.UUID, danceID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var exists bool
	err := appmetrics.ObserveDBQuery("is_liked_by_user", func() error {
		return u.db.QueryRow(ctx, IsLikedByUserQuery, userID, danceID).Scan(&exists)
	})
	if err != nil {
		logger.Error("failed to check like: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return exists, nil
}

func (u *UserRepository) GetTopLikedDances(ctx context.Context, limit int) ([]models.DanceLikeStat, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_top_liked_dances", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetTopLikedDancesQuery, limit)
		return e
	})
	if err != nil {
		logger.Error("failed to get top dances: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var stats []models.DanceLikeStat
	for rows.Next() {
		var s models.DanceLikeStat
		if err := rows.Scan(&s.DanceID, &s.LikesCount); err != nil {
			logger.Error("failed to scan dance stat: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetTopLikedDances: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return stats, nil
}

func (u *UserRepository) GetUserLikedDances(ctx context.Context, userID uuid.UUID) ([]models.DanceLike, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_user_liked_dances", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetUserLikedDancesQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to get liked dances: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var likes []models.DanceLike
	for rows.Next() {
		var l models.DanceLike
		var historyID *string
		if err := rows.Scan(&historyID, &l.DanceID, &l.Name, &l.DanceTitle, &l.CreatedAt); err != nil {
			logger.Error("failed to scan like: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		if historyID != nil {
			l.HistoryID = *historyID
		}
		likes = append(likes, l)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetUserLikedDances: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return likes, nil
}

func (u *UserRepository) CleanHistory(ctx context.Context, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("clean_history", func() error {
		_, e := u.db.Exec(ctx, CleanHistoryQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to clean history: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) SaveRating(ctx context.Context, userID uuid.UUID, input models.SaveRatingInput) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("save_rating", func() error {
		_, e := u.db.Exec(ctx, SaveRatingQuery,
			input.VideoID, userID, input.Physical, input.Speed, input.Coordination, input.Repeatability)
		return e
	})
	if err != nil {
		logger.Error("failed to save rating: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetAggregatedRating(ctx context.Context, videoID string) (*models.RatingResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var r models.RatingResponse
	r.VideoID = videoID
	err := appmetrics.ObserveDBQuery("get_aggregated_rating", func() error {
		return u.db.QueryRow(ctx, GetAggregatedRatingQuery, videoID).Scan(
			&r.AvgPhysical, &r.AvgSpeed, &r.AvgCoordination, &r.AvgRepeatability, &r.AvgScore, &r.TotalRatings,
		)
	})
	if err != nil {
		logger.Error("failed to get rating: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return &r, nil
}

func (u *UserRepository) CreateDance(ctx context.Context, id, title, status, difficulty, videoPath string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("create_dance", func() error {
		_, e := u.db.Exec(ctx, CreateDanceQuery, id, title, status, difficulty, videoPath)
		return e
	})
	if err != nil {
		logger.Error("failed to create dance: " + err.Error())
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
		logger.Error("failed to update dance status: " + err.Error())
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
		logger.Error("failed to query published dance ids: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			logger.Error("failed to scan published dance id: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		result[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetPublishedDanceIDs: " + err.Error())
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
		logger.Error("failed to update dance difficulty: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) RecordDanceAttempt(ctx context.Context, danceID string, userID *uuid.UUID, attemptID string, score float64) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("record_dance_attempt", func() error {
		_, e := u.db.Exec(ctx, RecordDanceAttemptQuery, danceID, userID, attemptID, score)
		return e
	})
	if err != nil {
		logger.Error("failed to record dance attempt: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) CreateCompareTask(ctx context.Context, taskID, danceID, userDanceID, videoKey string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("create_compare_task", func() error {
		_, e := u.db.Exec(ctx, CreateCompareTaskQuery, taskID, danceID, userDanceID, videoKey)
		return e
	})
	if err != nil {
		logger.Error("failed to create compare task: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetCompareTask(ctx context.Context, taskID string) (models.CompareTask, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	task := models.CompareTask{TaskID: taskID}
	err := appmetrics.ObserveDBQuery("get_compare_task", func() error {
		return u.db.QueryRow(ctx, GetCompareTaskQuery, taskID).Scan(
			&task.DanceID, &task.UserDanceID, &task.VideoKey,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.CompareTask{}, users.ErrorNotFound
		}
		logger.Error("failed to get compare task: " + err.Error())
		return models.CompareTask{}, users.ErrorInternalServerError
	}
	return task, nil
}

func (u *UserRepository) MarkCompareTaskFinalized(ctx context.Context, taskID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rowsAffected int64
	err := appmetrics.ObserveDBQuery("mark_compare_task_finalized", func() error {
		tag, e := u.db.Exec(ctx, MarkCompareTaskFinalizedQuery, taskID)
		if e == nil {
			rowsAffected = tag.RowsAffected()
		}
		return e
	})
	if err != nil {
		logger.Error("failed to mark compare task finalized: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return rowsAffected == 1, nil
}

const minRatingsForCrowdDifficulty = 3

const popular7dExpr = "(SELECT COUNT(*) FROM dance_attempts pa WHERE pa.dance_id = d.id AND pa.created_at >= NOW() - INTERVAL '7 days') DESC NULLS LAST"

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
		logger.Error("failed to get dance catalog: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.DanceCatalogItem
	for rows.Next() {
		var item models.DanceCatalogItem
		var hasAuthor bool
		if err := rows.Scan(&item.ID, &item.Title, &item.CreatedAt, &item.AttemptCount, &item.AvgScore, &item.ViewCount, &item.LikeCount, &item.DurationSec, &item.DifficultyByUsers, &item.DifficultyScore, &hasAuthor); err != nil {
			logger.Error("failed to scan catalog item: " + err.Error())
			return nil, users.ErrorInternalServerError
		}

		if hasAuthor || item.DifficultyByUsers {
			item.Difficulty = difficultyLabelFromScore(item.DifficultyScore)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDanceCatalog: " + err.Error())
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
			logger.Error("failed to get catalog count: " + err.Error())
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
		logger.Error("failed to get filtered catalog count: " + err.Error())
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
		logger.Error("failed to get enriched info: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := make(map[string]models.DanceEnrichedInfo, len(ids))
	for rows.Next() {
		var id string
		var info models.DanceEnrichedInfo
		if err := rows.Scan(&id, &info.Title, &info.AttemptCount, &info.AvgScore, &info.ViewCount, &info.LikeCount); err != nil {
			logger.Error("failed to scan enriched info: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		result[id] = info
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDancesEnrichedInfo: " + err.Error())
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
		logger.Error("failed to get trending dances: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.DanceCatalogItem
	for rows.Next() {
		var item models.DanceCatalogItem
		if err := rows.Scan(&item.ID, &item.Title, &item.AttemptCount, &item.AvgScore, &item.ViewCount, &item.LikeCount); err != nil {
			logger.Error("failed to scan trending item: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDanceTrending: " + err.Error())
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
		logger.Error("failed to get dance stats: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return &s, nil
}

func (u *UserRepository) GetDanceLeaderboard(ctx context.Context, danceID string) ([]models.LeaderboardEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_dance_leaderboard", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetLeaderboardQuery, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to get leaderboard: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var entries []models.LeaderboardEntry
	for rows.Next() {
		var e models.LeaderboardEntry
		if err := rows.Scan(&e.Rank, &e.Login, &e.Score, &e.UserID, &e.Avatar); err != nil {
			logger.Error("failed to scan leaderboard row: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDanceLeaderboard: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	if entries == nil {
		entries = []models.LeaderboardEntry{}
	}
	return entries, nil
}

func (u *UserRepository) GetUserDanceRank(ctx context.Context, danceID string, userID uuid.UUID) (*models.LeaderboardEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var e models.LeaderboardEntry
	err := appmetrics.ObserveDBQuery("get_user_dance_rank", func() error {
		return u.db.QueryRow(ctx, GetUserDanceRankQuery, danceID, userID).Scan(&e.Rank, &e.Login, &e.Score, &e.Avatar)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.Error("failed to get user rank: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	e.UserID = userID
	e.IsMe = true
	return &e, nil
}

func (u *UserRepository) GetUserGlobalRank(ctx context.Context, userID uuid.UUID) (int, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rank int
	err := appmetrics.ObserveDBQuery("get_user_global_rank", func() error {
		return u.db.QueryRow(ctx, GetUserGlobalRankQuery, userID).Scan(&rank)
	})
	if err != nil {
		logger.Error("failed to get user global rank: " + err.Error())
		return 0, users.ErrorInternalServerError
	}
	return rank, nil
}

func (u *UserRepository) GetFriendsDanceScores(ctx context.Context, userID uuid.UUID, danceID string) ([]models.FriendScore, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_friends_dance_scores", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetFriendsScoresQuery, userID, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to query friends dance scores: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := []models.FriendScore{}
	for rows.Next() {
		var fs models.FriendScore
		if err := rows.Scan(&fs.Login, &fs.AvatarURL, &fs.BestScore); err != nil {
			logger.Error("failed to scan friend score: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		result = append(result, fs)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetFriendsDanceScores: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) SaveAttempt(ctx context.Context, userID uuid.UUID, attemptID, danceID string, score float64, hasVideo bool, userName string, isPrivate bool, timingScore, amplitudeScore, poseScore float64) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("save_attempt", func() error {
		_, e := u.db.Exec(ctx, SaveAttemptQuery, attemptID, userID, danceID, score, hasVideo, userName, isPrivate, timingScore, amplitudeScore, poseScore)
		return e
	})
	if err != nil {
		logger.Error("failed to save attempt: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) EnsureSavedAttemptForDuel(ctx context.Context, userID uuid.UUID, attemptID, danceID string, score float64, isPrivate bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("ensure_saved_attempt_for_duel", func() error {
		_, e := u.db.Exec(ctx, EnsureSavedAttemptForDuelQuery, attemptID, userID, danceID, score, isPrivate)
		return e
	})
	if err != nil {
		logger.Error("failed to ensure saved attempt for duel: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetUserWeakSpots(ctx context.Context, userID uuid.UUID) (*models.WeakSpots, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var timing, amplitude, pose float64
	err := appmetrics.ObserveDBQuery("get_user_weak_spots", func() error {
		return u.db.QueryRow(ctx, GetUserWeakSpotsQuery, userID).Scan(&timing, &amplitude, &pose)
	})
	if err != nil {
		logger.Warn("get_user_weak_spots query error", "error", err)
		return nil, nil
	}
	if timing+amplitude+pose == 0 {
		return nil, nil
	}
	type metric struct {
		name string
		val  float64
	}
	metrics := []metric{{"timing", timing}, {"amplitude", amplitude}, {"pose", pose}}
	worst := metrics[0]
	for _, m := range metrics[1:] {
		if m.val < worst.val {
			worst = m
		}
	}
	suggestions := map[string]string{
		"timing":    "Поработай над синхронизацией с ритмом — попробуй отсчитывать такты",
		"amplitude": "Попробуй танцы с широкими движениями чтобы улучшить амплитуду",
		"pose":      "Обрати внимание на точность поз в ключевых кадрах",
	}
	return &models.WeakSpots{
		WorstMetric: worst.name,
		Avg:         math.Round(worst.val*10) / 10,
		Suggestion:  suggestions[worst.name],
	}, nil
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
		logger.Error("failed to get uploaded dances: " + err.Error())
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
			logger.Error("failed to scan uploaded dance: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		d.Difficulty = difficultyLabelFromScore(effectiveScore)
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetUploadedDancesByUser: " + err.Error())
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
		logger.Error("failed to begin transaction for DeleteDanceAndRelated: " + err.Error())
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
		logger.Error("failed to commit transaction for DeleteDanceAndRelated: " + err.Error())
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
		logger.Error("failed to get effective difficulty: " + err.Error())
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
		logger.Error("failed to update dance title: " + err.Error())
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
		logger.Error("failed to update dance duration: " + err.Error())
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
		logger.Error("failed to record dance view: " + err.Error())
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
		logger.Error("failed to get dance view count: " + err.Error())
		return 0, users.ErrorInternalServerError
	}
	return count, nil
}

func (u *UserRepository) IsSavedAttemptWithVideo(ctx context.Context, userID uuid.UUID, attemptID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var exists bool
	err := appmetrics.ObserveDBQuery("is_saved_attempt_with_video", func() error {
		return u.db.QueryRow(ctx, IsSavedAttemptWithVideoQuery, attemptID, userID).Scan(&exists)
	})
	if err != nil {
		logger.Error("failed to check saved attempt video flag: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return exists, nil
}

func (u *UserRepository) SavedAttemptExists(ctx context.Context, userID uuid.UUID, attemptID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var exists bool
	err := appmetrics.ObserveDBQuery("saved_attempt_exists", func() error {
		return u.db.QueryRow(ctx, SavedAttemptExistsQuery, attemptID, userID).Scan(&exists)
	})
	if err != nil {
		logger.Error("failed to check saved attempt existence: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return exists, nil
}

func (u *UserRepository) IsAttemptPrivate(ctx context.Context, attemptID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var isPrivate bool
	err := appmetrics.ObserveDBQuery("is_attempt_private", func() error {
		return u.db.QueryRow(ctx, IsAttemptPrivateQuery, attemptID).Scan(&isPrivate)
	})
	if err != nil {
		logger.Error("failed to check attempt privacy: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return isPrivate, nil
}

func (u *UserRepository) UnsaveAttempt(ctx context.Context, userID uuid.UUID, attemptID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("unsave_attempt", func() error {
		_, e := u.db.Exec(ctx, UnsaveAttemptQuery, attemptID, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to unsave attempt: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetSavedAttempts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.SavedAttemptItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_saved_attempts", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetSavedAttemptsQuery, userID, limit, offset)
		return e
	})
	if err != nil {
		logger.Error("failed to query saved attempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.SavedAttemptItem{}
	for rows.Next() {
		var it models.SavedAttemptItem
		if err := rows.Scan(&it.UserDanceID, &it.DanceID, &it.DanceTitle, &it.Score, &it.SavedAt, &it.HasVideo, &it.UserName, &it.IsPrivate); err != nil {
			logger.Error("failed to scan saved attempt: " + err.Error())
			return nil, users.ErrorInternalServerError
		}

		it.ReferenceVideoKey = fmt.Sprintf("results/%s/video.mp4", it.DanceID)
		it.UserAnimationKey = fmt.Sprintf("users/%s/%s/user_animation.glb", userID.String(), it.UserDanceID)
		it.UserSkeletonKey = fmt.Sprintf("users/%s/%s/skeleton.json", userID.String(), it.UserDanceID)
		if it.HasVideo {
			it.UserVideoKey = fmt.Sprintf("users/%s/%s/video.mp4", userID.String(), it.UserDanceID)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetSavedAttempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) GetUserAttempts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.UserAttemptItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_user_attempts", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetUserAttemptsQuery, userID, limit, offset)
		return e
	})
	if err != nil {
		logger.Error("failed to query user attempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.UserAttemptItem{}
	for rows.Next() {
		var it models.UserAttemptItem
		if err := rows.Scan(&it.AttemptID, &it.DanceID, &it.DanceTitle, &it.Score, &it.CreatedAt, &it.IsSaved, &it.IsOpen, &it.UserName, &it.Rank, &it.TotalDancers); err != nil {
			logger.Error("failed to scan user attempt: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetUserAttempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) GetDanceProgress(ctx context.Context, userID uuid.UUID, danceID string) ([]models.DanceProgressEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_dance_progress", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetDanceProgressQuery, userID, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to query dance progress: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.DanceProgressEntry{}
	for rows.Next() {
		var it models.DanceProgressEntry
		if err := rows.Scan(&it.AttemptID, &it.Score, &it.CreatedAt); err != nil {
			logger.Error("failed to scan dance progress entry: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDanceProgress: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) GetUserActivity(ctx context.Context, userID uuid.UUID) ([]models.ActivityEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_user_activity", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetUserActivityQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query user activity: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.ActivityEntry{}
	for rows.Next() {
		var it models.ActivityEntry
		if err := rows.Scan(&it.Day, &it.Count); err != nil {
			logger.Error("failed to scan activity entry: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetUserActivity: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) GetMostImprovedDance(ctx context.Context, userID uuid.UUID) (*models.MostImprovedDance, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var it models.MostImprovedDance
	err := appmetrics.ObserveDBQuery("get_most_improved_dance", func() error {
		return u.db.QueryRow(ctx, GetMostImprovedDanceQuery, userID).
			Scan(&it.DanceID, &it.Title, &it.FirstScore, &it.LastScore, &it.Delta)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.Error("failed to query most improved dance: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return &it, nil
}

func (u *UserRepository) GetReelsAttempts(ctx context.Context, limit, offset int) ([]models.ReelsAttemptItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_reels_attempts", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetReelsAttemptsQuery, limit, offset)
		return e
	})
	if err != nil {
		logger.Error("failed to query reels attempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.ReelsAttemptItem{}
	for rows.Next() {
		var it models.ReelsAttemptItem
		if err := rows.Scan(&it.AttemptID, &it.DanceID, &it.DanceTitle, &it.UserID, &it.UserLogin, &it.UserAvatar, &it.Score); err != nil {
			logger.Error("failed to scan reels attempt: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		it.VideoKey = fmt.Sprintf("users/%s/%s/video.mp4", it.UserID, it.AttemptID)
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetReelsAttempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) GetCreatorAnalytics(ctx context.Context, userID uuid.UUID) (*models.CreatorAnalytics, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var dailyRows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_creator_daily_stats", func() error {
		var e error
		dailyRows, e = u.db.Query(ctx, GetCreatorDailyStatsQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query creator daily stats: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer dailyRows.Close()

	daily := []models.CreatorDailyStats{}
	for dailyRows.Next() {
		var it models.CreatorDailyStats
		if err := dailyRows.Scan(&it.Day, &it.Views, &it.Likes, &it.Attempts); err != nil {
			logger.Error("failed to scan creator daily stats: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		daily = append(daily, it)
	}
	if err := dailyRows.Err(); err != nil {
		logger.Error("rows iteration error in GetCreatorAnalytics (daily): " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	dailyRows.Close()

	var topRows pgx.Rows
	err = appmetrics.ObserveDBQuery("get_creator_top_dances", func() error {
		var e error
		topRows, e = u.db.Query(ctx, GetCreatorTopDancesQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query creator top dances: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer topRows.Close()

	top := []models.CreatorTopDance{}
	for topRows.Next() {
		var it models.CreatorTopDance
		if err := topRows.Scan(&it.DanceID, &it.Title, &it.Attempts, &it.Likes); err != nil {
			logger.Error("failed to scan creator top dance: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		top = append(top, it)
	}
	if err := topRows.Err(); err != nil {
		logger.Error("rows iteration error in GetCreatorAnalytics (top): " + err.Error())
		return nil, users.ErrorInternalServerError
	}

	return &models.CreatorAnalytics{Daily: daily, TopDances: top}, nil
}

func (u *UserRepository) GetAttemptOwner(ctx context.Context, attemptID string) (*uuid.UUID, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var ownerID *uuid.UUID
	err := appmetrics.ObserveDBQuery("get_attempt_owner", func() error {
		return u.db.QueryRow(ctx, GetAttemptOwnerQuery, attemptID).Scan(&ownerID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.Error("failed to get attempt owner: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return ownerID, nil
}

func (u *UserRepository) GetLastAttempt(ctx context.Context, userID uuid.UUID, danceID string) (*models.UserAttemptItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var it models.UserAttemptItem
	err := appmetrics.ObserveDBQuery("get_last_attempt", func() error {
		return u.db.QueryRow(ctx, GetLastAttemptQuery, userID, danceID).
			Scan(&it.AttemptID, &it.DanceID, &it.DanceTitle, &it.Score, &it.CreatedAt, &it.IsSaved)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.Error("failed to query last attempt: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return &it, nil
}

func (u *UserRepository) GetPersonalTop(ctx context.Context, userID uuid.UUID) ([]models.PersonalTopItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_personal_top", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetPersonalTopQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query personal top: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.PersonalTopItem{}
	for rows.Next() {
		var it models.PersonalTopItem
		if err := rows.Scan(&it.DanceID, &it.UserDanceID, &it.DanceTitle, &it.UserName, &it.BestScore, &it.AchievedAt); err != nil {
			logger.Error("failed to scan personal top item: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetPersonalTop: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) LinkDanceUpload(ctx context.Context, userID uuid.UUID, danceID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("link_dance_upload", func() error {
		_, e := u.db.Exec(ctx, LinkDanceUploadQuery, userID, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to link dance upload: " + err.Error())
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
		logger.Error("failed to query dance uploaders: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			logger.Error("failed to scan uploader id: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDanceUploaders: " + err.Error())
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
		logger.Error("failed to query dance author: " + err.Error())
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
		logger.Error("failed to set moderation reason: " + err.Error())
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

func (u *UserRepository) CreateNotification(ctx context.Context, userID uuid.UUID, notifType, danceID, reason string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var reasonArg interface{}
	if reason == "" {
		reasonArg = nil
	} else {
		reasonArg = reason
	}
	err := appmetrics.ObserveDBQuery("create_notification", func() error {
		_, e := u.db.Exec(ctx, CreateNotificationQuery, userID, notifType, danceID, reasonArg)
		return e
	})
	if err != nil {
		logger.Error("failed to create notification: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetNotifications(ctx context.Context, userID uuid.UUID) ([]models.Notification, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_notifications", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetNotificationsQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query notifications: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.Type, &n.DanceID, &n.Reason, &n.IsRead, &n.CreatedAt, &n.FromUserID, &n.FromLogin, &n.RefID, &n.DuelID); err != nil {
			logger.Error("failed to scan notification: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetNotifications: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) MarkNotificationRead(ctx context.Context, id int64, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("mark_notification_read", func() error {
		_, e := u.db.Exec(ctx, MarkNotificationReadQuery, id, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to mark notification read: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("mark_all_notifications_read", func() error {
		_, e := u.db.Exec(ctx, MarkAllNotificationsReadQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to mark all notifications read: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) ClearNotifications(ctx context.Context, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("clear_notifications", func() error {
		_, e := u.db.Exec(ctx, ClearNotificationsQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to clear notifications: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) CreateFriendship(ctx context.Context, senderID, receiverID uuid.UUID) (int64, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var id int64
	err := appmetrics.ObserveDBQuery("create_friendship", func() error {
		return u.db.QueryRow(ctx, CreateFriendshipQuery, senderID, receiverID).Scan(&id)
	})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint") || strings.Contains(msg, "23505") {
			return 0, users.ErrorAlreadyExists
		}
		logger.Error("failed to create friendship: " + msg)
		return 0, users.ErrorInternalServerError
	}
	return id, nil
}

func (u *UserRepository) UpdateFriendshipStatus(ctx context.Context, friendshipID int64, receiverID uuid.UUID, status string) (uuid.UUID, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var senderID uuid.UUID
	err := appmetrics.ObserveDBQuery("update_friendship_status", func() error {
		return u.db.QueryRow(ctx, UpdateFriendshipStatusQuery, friendshipID, receiverID, status).Scan(&senderID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, users.ErrorNotFound
		}
		logger.Error("failed to update friendship status: " + err.Error())
		return uuid.Nil, users.ErrorInternalServerError
	}
	return senderID, nil
}

func (u *UserRepository) GetFriends(ctx context.Context, userID uuid.UUID) ([]models.Friend, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_friends", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetFriendsQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query friends: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := []models.Friend{}
	for rows.Next() {
		var f models.Friend
		if err := rows.Scan(&f.UserID, &f.Login, &f.Avatar, &f.FriendedAt, &f.ActiveDuelID); err != nil {
			logger.Error("failed to scan friend: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		result = append(result, f)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetFriends: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) SearchUsers(ctx context.Context, query string, limit int) ([]models.UserSearchItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("search_users", func() error {
		var e error
		rows, e = u.db.Query(ctx, SearchUsersQuery, query, limit)
		return e
	})
	if err != nil {
		logger.Error("failed to query users search: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := []models.UserSearchItem{}
	for rows.Next() {
		var it models.UserSearchItem
		if err := rows.Scan(&it.ID, &it.Login, &it.Avatar); err != nil {
			logger.Error("failed to scan user search item: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in SearchUsers: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) HasOpenDuel(ctx context.Context, challengerID, opponentID uuid.UUID, danceID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var exists bool
	err := appmetrics.ObserveDBQuery("has_open_duel", func() error {
		return u.db.QueryRow(ctx, HasOpenDuelQuery, challengerID, opponentID, danceID).Scan(&exists)
	})
	if err != nil {
		logger.Error("failed to check open duel: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return exists, nil
}

func (u *UserRepository) GetActiveDuelsForUserDance(ctx context.Context, userID uuid.UUID, danceID string) ([]models.ActiveDuelForDance, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_active_duels_for_user_dance", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetActiveDuelsForUserDanceQuery, userID, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to query active duels for dance: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := []models.ActiveDuelForDance{}
	for rows.Next() {
		var it models.ActiveDuelForDance
		if err := rows.Scan(&it.DuelID, &it.OpponentLogin); err != nil {
			logger.Error("failed to scan active duel: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetActiveDuelsForUserDance: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) GetFriendsCount(ctx context.Context, userID uuid.UUID) (int, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var count int
	err := appmetrics.ObserveDBQuery("get_friends_count", func() error {
		return u.db.QueryRow(ctx, GetFriendsCountQuery, userID).Scan(&count)
	})
	if err != nil {
		logger.Error("failed to count friends: " + err.Error())
		return 0, users.ErrorInternalServerError
	}
	return count, nil
}

func (u *UserRepository) GetFriendshipBetween(ctx context.Context, userID, otherID uuid.UUID) (*models.FriendshipStatus, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var id int64
	var status string
	var senderID uuid.UUID
	err := appmetrics.ObserveDBQuery("get_friendship_between", func() error {
		return u.db.QueryRow(ctx, GetFriendshipBetweenQuery, userID, otherID).Scan(&id, &status, &senderID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &models.FriendshipStatus{Status: "none"}, nil
		}
		logger.Error("failed to get friendship: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return &models.FriendshipStatus{
		Status:       status,
		IsSender:     senderID == userID,
		FriendshipID: id,
	}, nil
}

func (u *UserRepository) DeleteFriendship(ctx context.Context, userID, friendID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("delete_friendship", func() error {
		_, e := u.db.Exec(ctx, DeleteFriendshipQuery, userID, friendID)
		return e
	})
	if err != nil {
		logger.Error("failed to delete friendship: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) CreateFriendNotification(ctx context.Context, toUserID, fromUserID uuid.UUID, notifType string, friendshipID int64) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("create_friend_notification", func() error {
		_, e := u.db.Exec(ctx, CreateFriendNotificationQuery, toUserID, notifType, fromUserID, friendshipID)
		return e
	})
	if err != nil {
		logger.Error("failed to create friend notification: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) FindUserByLoginOrEmail(ctx context.Context, loginOrEmail string) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("find_user_by_login_or_email", func() error {
		return u.db.QueryRow(ctx, BotFindUserQuery, loginOrEmail).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("botFindUser scan failed: " + err.Error())
		return models.User{}, users.ErrorInternalServerError
	}
	return user, nil
}

func (u *UserRepository) UpdateUserTelegramID(ctx context.Context, userID uuid.UUID, telegramID int64) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("update_user_telegram_id", func() error {
		_, e := u.db.Exec(ctx, BotUpdateTelegramIDQuery, telegramID, userID)
		return e
	})
	if err != nil {
		logger.Error("botUpdateTelegramID failed: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetUserTelegramID(ctx context.Context, userID uuid.UUID) (*int64, error) {
	var tid *int64
	err := appmetrics.ObserveDBQuery("get_user_telegram_id", func() error {
		return u.db.QueryRow(ctx, GetUserTelegramIDQuery, userID).Scan(&tid)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, users.ErrorNotFound
		}
		return nil, users.ErrorInternalServerError
	}
	return tid, nil
}

func (u *UserRepository) SetTelegramLinkCode(ctx context.Context, userID uuid.UUID, code string, expiresAt time.Time) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("set_telegram_link_code", func() error {
		_, e := u.db.Exec(ctx,
			`UPDATE user_table SET telegram_link_code = $2, telegram_link_code_expires = $3 WHERE id = $1`,
			userID, code, expiresAt)
		return e
	})
	if err != nil {
		logger.Error("SetTelegramLinkCode failed: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) LinkTelegramByCode(ctx context.Context, code string, telegramID int64) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("link_telegram_by_code", func() error {
		if _, e := u.db.Exec(ctx,
			`UPDATE user_table SET telegram_id = NULL WHERE telegram_id = $1`, telegramID); e != nil {
			return e
		}
		return u.db.QueryRow(ctx,
			`UPDATE user_table
			    SET telegram_id = $2, telegram_link_code = NULL, telegram_link_code_expires = NULL
			  WHERE telegram_link_code = $1 AND telegram_link_code_expires > NOW()
			  RETURNING id, login`,
			code, telegramID).Scan(&user.ID, &user.Login)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("LinkTelegramByCode failed: " + err.Error())
		return models.User{}, users.ErrorInternalServerError
	}
	return user, nil
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

func (u *UserRepository) GetUserByTelegramID(ctx context.Context, telegramID int64) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("get_user_by_telegram_id", func() error {
		return u.db.QueryRow(ctx, BotGetUserByTelegramIDQuery, telegramID).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("GetUserByTelegramID scan failed: " + err.Error())
		return models.User{}, users.ErrorInternalServerError
	}
	return user, nil
}
