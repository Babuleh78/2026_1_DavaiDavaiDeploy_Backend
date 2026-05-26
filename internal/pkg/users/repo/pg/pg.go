package repo

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"

	"github.com/jackc/pgtype/pgxtype"
	"github.com/jackc/pgx/v4"
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
	err := u.db.QueryRow(
		ctx,
		GetUserByIDQuery,
		id,
	).Scan(
		&user.ID, &user.Version, &user.Login,
		&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
	)
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
	err := u.db.QueryRow(
		ctx,
		GetUserByLoginQuery,
		login,
	).Scan(
		&user.ID, &user.Version, &user.Login,
		&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
	)
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
	_, err := u.db.Exec(
		ctx,
		UpdateUserPasswordQuery,
		passwordHash, version, userID,
	)
	if err != nil {
		logger.Error("failed to update password: " + err.Error())
		return users.ErrorInternalServerError
	}

	logger.Info("succesfully updated password of user from db")
	return err
}

func (u *UserRepository) UpdateUserProfile(ctx context.Context, userID uuid.UUID, login *string, avatar *string, bumpVersion bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, UpdateUserProfileQuery, login, avatar, bumpVersion, userID)
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
	_, err := u.db.Exec(ctx, AddToHistoryQuery, userID, danceID, sourceURL)
	if err != nil {
		logger.Error("failed to add to history: " + err.Error())
		return users.ErrorInternalServerError
	}
	logger.Info("successfully added to search history")
	return nil
}

func (u *UserRepository) GetHistory(ctx context.Context, userID uuid.UUID) ([]models.SearchHistoryItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	rows, err := u.db.Query(ctx, GetHistoryQuery, userID)
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
	return items, nil
}

func (u *UserRepository) DeleteFromHistory(ctx context.Context, historyID uuid.UUID, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	result, err := u.db.Exec(ctx, DeleteFromHistoryQuery, historyID, userID)
	if err != nil {
		logger.Error("failed to delete from history: " + err.Error())
		return users.ErrorInternalServerError
	}
	if result.RowsAffected() == 0 {
		return users.ErrorNotFound
	}
	logger.Info("successfully deleted from search history")
	return nil
}

func (u *UserRepository) UpdateHistoryName(ctx context.Context, historyID uuid.UUID, userID uuid.UUID, name string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	result, err := u.db.Exec(ctx, UpdateHistoryNameQuery, name, historyID, userID)
	if err != nil {
		logger.Error("failed to update history name: " + err.Error())
		return users.ErrorInternalServerError
	}
	if result.RowsAffected() == 0 {
		return users.ErrorNotFound
	}
	logger.Info("successfully updated history item name")
	return nil
}

func (u *UserRepository) ToggleLike(ctx context.Context, userID uuid.UUID, danceID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var inserted bool
	err := u.db.QueryRow(ctx, ToggleLikeQuery, userID, danceID).Scan(&inserted)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			_, err = u.db.Exec(ctx, DeleteLikeQuery, userID, danceID)
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
	err := u.db.QueryRow(ctx, GetLikesCountQuery, danceID).Scan(&count)
	if err != nil {
		logger.Error("failed to get likes count: " + err.Error())
		return 0, users.ErrorInternalServerError
	}
	return count, nil
}

func (u *UserRepository) IsLikedByUser(ctx context.Context, userID uuid.UUID, danceID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var exists bool
	err := u.db.QueryRow(ctx, IsLikedByUserQuery, userID, danceID).Scan(&exists)
	if err != nil {
		logger.Error("failed to check like: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return exists, nil
}

func (u *UserRepository) GetTopLikedDances(ctx context.Context, limit int) ([]models.DanceLikeStat, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	rows, err := u.db.Query(ctx, GetTopLikedDancesQuery, limit)
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
	return stats, nil
}

func (u *UserRepository) GetUserLikedDances(ctx context.Context, userID uuid.UUID) ([]models.DanceLike, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	rows, err := u.db.Query(ctx, GetUserLikedDancesQuery, userID)
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
	return likes, nil
}

func (u *UserRepository) CleanHistory(ctx context.Context, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, CleanHistoryQuery, userID)
	if err != nil {
		logger.Error("failed to clean history: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) SaveRating(ctx context.Context, userID uuid.UUID, input models.SaveRatingInput) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, SaveRatingQuery,
		input.VideoID, userID, input.Physical, input.Speed, input.Coordination, input.Repeatability)
	if err != nil {
		logger.Error("failed to save rating: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetAggregatedRating(ctx context.Context, videoID string) (*models.RatingResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	row := u.db.QueryRow(ctx, GetAggregatedRatingQuery, videoID)

	var r models.RatingResponse
	r.VideoID = videoID
	if err := row.Scan(&r.AvgPhysical, &r.AvgSpeed, &r.AvgCoordination, &r.AvgRepeatability, &r.AvgScore, &r.TotalRatings); err != nil {
		logger.Error("failed to get rating: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return &r, nil
}

func (u *UserRepository) CreateDance(ctx context.Context, id, title, status, difficulty, videoPath string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, CreateDanceQuery, id, title, status, difficulty, videoPath)
	if err != nil {
		logger.Error("failed to create dance: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetDanceVideoPath(ctx context.Context, id string) (string, error) {
	var videoPath string
	err := u.db.QueryRow(ctx, GetDanceVideoPathQuery, id).Scan(&videoPath)
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
	_, err := u.db.Exec(ctx, UpdateDanceStatusQuery, status, id)
	if err != nil {
		logger.Error("failed to update dance status: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetDanceStatus(ctx context.Context, id string) (string, error) {
	var status string
	err := u.db.QueryRow(ctx, GetDanceStatusQuery, id).Scan(&status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", users.ErrorNotFound
		}
		return "", users.ErrorInternalServerError
	}
	return status, nil
}

func (u *UserRepository) GetPublishedDanceIDs(ctx context.Context, ids []string) (map[string]struct{}, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	result := make(map[string]struct{})
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := u.db.Query(ctx, GetPublishedDanceIDsQuery, ids)
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
	return result, nil
}

func (u *UserRepository) UpdateDanceDifficulty(ctx context.Context, danceID, difficulty string, difficultyScore int) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, UpdateDanceDifficultyQuery, danceID, difficulty, difficultyScore)
	if err != nil {
		logger.Error("failed to update dance difficulty: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) RecordDanceAttempt(ctx context.Context, danceID string, userID *uuid.UUID, attemptID string, score float64) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, RecordDanceAttemptQuery, danceID, userID, attemptID, score)
	if err != nil {
		logger.Error("failed to record dance attempt: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) CreateCompareTask(ctx context.Context, taskID, danceID, userDanceID, videoKey string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, CreateCompareTaskQuery, taskID, danceID, userDanceID, videoKey)
	if err != nil {
		logger.Error("failed to create compare task: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetCompareTask(ctx context.Context, taskID string) (models.CompareTask, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	task := models.CompareTask{TaskID: taskID}
	err := u.db.QueryRow(ctx, GetCompareTaskQuery, taskID).Scan(
		&task.DanceID, &task.UserDanceID, &task.VideoKey,
	)
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
	tag, err := u.db.Exec(ctx, MarkCompareTaskFinalizedQuery, taskID)
	if err != nil {
		logger.Error("failed to mark compare task finalized: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return tag.RowsAffected() == 1, nil
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
        GROUP BY d.id, d.title, d.difficulty_score, d.created_at
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

	rows, err := u.db.Query(ctx, query, args...)
	if err != nil {
		logger.Error("failed to get dance catalog: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.DanceCatalogItem
	for rows.Next() {
		var item models.DanceCatalogItem
		var hasAuthor bool
		if err := rows.Scan(&item.ID, &item.Title, &item.CreatedAt, &item.AttemptCount, &item.AvgScore, &item.ViewCount, &item.LikeCount, &item.DifficultyByUsers, &item.DifficultyScore, &hasAuthor); err != nil {
			logger.Error("failed to scan catalog item: " + err.Error())
			return nil, users.ErrorInternalServerError
		}

		if hasAuthor || item.DifficultyByUsers {
			item.Difficulty = difficultyLabelFromScore(item.DifficultyScore)
		}
		items = append(items, item)
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
			err = u.db.QueryRow(ctx,
				"SELECT COUNT(*)::int FROM dances WHERE status = 'published' AND title ILIKE $1",
				"%"+search+"%",
			).Scan(&count)
		} else {
			err = u.db.QueryRow(ctx, GetDanceCatalogCountQuery).Scan(&count)
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
	if err := u.db.QueryRow(ctx, query, args...).Scan(&count); err != nil {
		logger.Error("failed to get filtered catalog count: " + err.Error())
		return 0, users.ErrorInternalServerError
	}
	return count, nil
}

func (u *UserRepository) GetDancesEnrichedInfo(ctx context.Context, ids []string) (map[string]models.DanceEnrichedInfo, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	rows, err := u.db.Query(ctx, GetDancesEnrichedInfoQuery, ids)
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
	return result, nil
}

func (u *UserRepository) GetDanceTrending(ctx context.Context) ([]models.DanceCatalogItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	rows, err := u.db.Query(ctx, GetDanceTrendingQuery)
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
	if items == nil {
		items = []models.DanceCatalogItem{}
	}
	return items, nil
}

func (u *UserRepository) GetDanceStats(ctx context.Context, danceID string) (*models.DanceStats, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var s models.DanceStats
	err := u.db.QueryRow(ctx, GetDanceStatsQuery, danceID).Scan(
		&s.AttemptCount, &s.AvgScore, &s.TopScore, &s.TopUser, &s.ViewCount,
	)
	if err != nil {
		logger.Error("failed to get dance stats: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return &s, nil
}

func (u *UserRepository) GetDanceLeaderboard(ctx context.Context, danceID string) ([]models.LeaderboardEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	rows, err := u.db.Query(ctx, GetLeaderboardQuery, danceID)
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
	if entries == nil {
		entries = []models.LeaderboardEntry{}
	}
	return entries, nil
}

func (u *UserRepository) GetUserDanceRank(ctx context.Context, danceID string, userID uuid.UUID) (*models.LeaderboardEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var e models.LeaderboardEntry
	err := u.db.QueryRow(ctx, GetUserDanceRankQuery, danceID, userID).Scan(&e.Rank, &e.Login, &e.Score, &e.Avatar)
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

func (u *UserRepository) SaveAttempt(ctx context.Context, userID uuid.UUID, attemptID, danceID string, score float64, hasVideo bool, userName string, isPrivate bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, SaveAttemptQuery, attemptID, userID, danceID, score, hasVideo, userName, isPrivate)
	if err != nil {
		logger.Error("failed to save attempt: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetUploadedDancesByUser(ctx context.Context, userID uuid.UUID) ([]models.UploadedDance, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	query := fmt.Sprintf(GetUploadedDancesByUserQuery, minRatingsForCrowdDifficulty, minRatingsForCrowdDifficulty)
	rows, err := u.db.Query(ctx, query, userID)
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
	for _, q := range queries {
		if _, err := u.db.Exec(ctx, q, danceID); err != nil {
			logger.Error("delete step failed", "query", q, "dance_id", danceID, "error", err)
			return users.ErrorInternalServerError
		}
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
	err := u.db.QueryRow(ctx, query, danceID).Scan(&score, &ratingCount, &avgDiff, &hasAuthor)
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
	_, err := u.db.Exec(ctx, UpdateDanceTitleQuery, danceID, title)
	if err != nil {
		logger.Error("failed to update dance title: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) RecordDanceView(ctx context.Context, danceID string, viewerID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	if danceID == "" || viewerID == "" {
		return users.ErrorBadRequest
	}
	_, err := u.db.Exec(ctx, RecordDanceViewQuery, danceID, viewerID)
	if err != nil {
		logger.Error("failed to record dance view: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) IsSavedAttemptWithVideo(ctx context.Context, userID uuid.UUID, attemptID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var exists bool
	err := u.db.QueryRow(ctx, IsSavedAttemptWithVideoQuery, attemptID, userID).Scan(&exists)
	if err != nil {
		logger.Error("failed to check saved attempt video flag: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return exists, nil
}

func (u *UserRepository) UnsaveAttempt(ctx context.Context, userID uuid.UUID, attemptID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, UnsaveAttemptQuery, attemptID, userID)
	if err != nil {
		logger.Error("failed to unsave attempt: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetSavedAttempts(ctx context.Context, userID uuid.UUID) ([]models.SavedAttemptItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	rows, err := u.db.Query(ctx, GetSavedAttemptsQuery, userID)
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
	return items, nil
}

func (u *UserRepository) GetUserAttempts(ctx context.Context, userID uuid.UUID) ([]models.UserAttemptItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	rows, err := u.db.Query(ctx, GetUserAttemptsQuery, userID)
	if err != nil {
		logger.Error("failed to query user attempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.UserAttemptItem{}
	for rows.Next() {
		var it models.UserAttemptItem
		if err := rows.Scan(&it.AttemptID, &it.DanceID, &it.DanceTitle, &it.Score, &it.CreatedAt, &it.IsSaved); err != nil {
			logger.Error("failed to scan user attempt: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, it)
	}
	return items, nil
}

func (u *UserRepository) GetAttemptOwner(ctx context.Context, attemptID string) (*uuid.UUID, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var ownerID *uuid.UUID
	err := u.db.QueryRow(ctx, GetAttemptOwnerQuery, attemptID).Scan(&ownerID)
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
	err := u.db.QueryRow(ctx, GetLastAttemptQuery, userID, danceID).
		Scan(&it.AttemptID, &it.DanceID, &it.DanceTitle, &it.Score, &it.CreatedAt, &it.IsSaved)
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
	rows, err := u.db.Query(ctx, GetPersonalTopQuery, userID)
	if err != nil {
		logger.Error("failed to query personal top: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.PersonalTopItem{}
	for rows.Next() {
		var it models.PersonalTopItem
		if err := rows.Scan(&it.DanceID, &it.UserDanceID, &it.DanceTitle, &it.BestScore, &it.AchievedAt); err != nil {
			logger.Error("failed to scan personal top item: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, it)
	}
	return items, nil
}

func (u *UserRepository) LinkDanceUpload(ctx context.Context, userID uuid.UUID, danceID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, LinkDanceUploadQuery, userID, danceID)
	if err != nil {
		logger.Error("failed to link dance upload: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetDanceUploaders(ctx context.Context, danceID string) ([]uuid.UUID, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	rows, err := u.db.Query(ctx, GetDanceUploadersQuery, danceID)
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
	return ids, nil
}

func (u *UserRepository) GetDanceAuthor(ctx context.Context, danceID string) (*models.DanceAuthor, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var author models.DanceAuthor
	err := u.db.QueryRow(ctx, GetDanceAuthorQuery, danceID).Scan(&author.ID, &author.Login, &author.Avatar)
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
	_, err := u.db.Exec(ctx, SetDanceModerationReasonQuery, reason, danceID)
	if err != nil {
		logger.Error("failed to set moderation reason: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetDanceModerationReason(ctx context.Context, danceID string) (string, error) {
	var reason string
	err := u.db.QueryRow(ctx, GetDanceModerationReasonQuery, danceID).Scan(&reason)
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
	_, err := u.db.Exec(ctx, CreateNotificationQuery, userID, notifType, danceID, reasonArg)
	if err != nil {
		logger.Error("failed to create notification: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetNotifications(ctx context.Context, userID uuid.UUID) ([]models.Notification, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	rows, err := u.db.Query(ctx, GetNotificationsQuery, userID)
	if err != nil {
		logger.Error("failed to query notifications: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.Type, &n.DanceID, &n.Reason, &n.IsRead, &n.CreatedAt, &n.FromUserID, &n.FromLogin, &n.RefID); err != nil {
			logger.Error("failed to scan notification: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, n)
	}
	return items, nil
}

func (u *UserRepository) MarkNotificationRead(ctx context.Context, id int64, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, MarkNotificationReadQuery, id, userID)
	if err != nil {
		logger.Error("failed to mark notification read: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, MarkAllNotificationsReadQuery, userID)
	if err != nil {
		logger.Error("failed to mark all notifications read: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) ClearNotifications(ctx context.Context, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, ClearNotificationsQuery, userID)
	if err != nil {
		logger.Error("failed to clear notifications: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) CreateFriendship(ctx context.Context, senderID, receiverID uuid.UUID) (int64, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var id int64
	err := u.db.QueryRow(ctx, CreateFriendshipQuery, senderID, receiverID).Scan(&id)
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
	err := u.db.QueryRow(ctx, UpdateFriendshipStatusQuery, friendshipID, receiverID, status).Scan(&senderID)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return uuid.Nil, users.ErrorNotFound
		}
		logger.Error("failed to update friendship status: " + err.Error())
		return uuid.Nil, users.ErrorInternalServerError
	}
	return senderID, nil
}

func (u *UserRepository) GetFriends(ctx context.Context, userID uuid.UUID) ([]models.Friend, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	rows, err := u.db.Query(ctx, GetFriendsQuery, userID)
	if err != nil {
		logger.Error("failed to query friends: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := []models.Friend{}
	for rows.Next() {
		var f models.Friend
		if err := rows.Scan(&f.UserID, &f.Login, &f.Avatar, &f.FriendedAt); err != nil {
			logger.Error("failed to scan friend: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		result = append(result, f)
	}
	return result, nil
}

func (u *UserRepository) GetFriendsCount(ctx context.Context, userID uuid.UUID) (int, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var count int
	err := u.db.QueryRow(ctx, GetFriendsCountQuery, userID).Scan(&count)
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
	err := u.db.QueryRow(ctx, GetFriendshipBetweenQuery, userID, otherID).Scan(&id, &status, &senderID)
	if err != nil {
		if err.Error() == "no rows in result set" {
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
	_, err := u.db.Exec(ctx, DeleteFriendshipQuery, userID, friendID)
	if err != nil {
		logger.Error("failed to delete friendship: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) CreateFriendNotification(ctx context.Context, toUserID, fromUserID uuid.UUID, notifType string, friendshipID int64) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(ctx, CreateFriendNotificationQuery, toUserID, notifType, fromUserID, friendshipID)
	if err != nil {
		logger.Error("failed to create friend notification: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}
