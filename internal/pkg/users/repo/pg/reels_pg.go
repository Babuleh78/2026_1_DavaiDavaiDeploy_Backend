package repo

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v4"
	uuid "github.com/satori/go.uuid"
)

func (u *UserRepository) GetReelsFeed(ctx context.Context, limit, offset int, excludeIDs []string, userID *uuid.UUID) ([]models.ReelItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var userIDArg interface{}
	if userID != nil {
		userIDArg = *userID
	}

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_reels_feed", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetReelsFeedQuery, limit, offset, excludeIDs, userIDArg)
		return e
	})
	if err != nil {
		logger.Error("failed to query reels feed", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.ReelItem
	for rows.Next() {
		var item models.ReelItem
		if err := rows.Scan(
			&item.DanceID,
			&item.Title,
			&item.UploaderID,
			&item.Username,
			&item.AvatarURL,
			&item.VideoURL,
			&item.PreviewURL,
			&item.ViewCount,
			&item.LikeCount,
			&item.AvgScore,
			&item.AttemptCount,
			&item.UserLiked,
		); err != nil {
			logger.Error("failed to scan reel item", "error", err)
			return nil, users.ErrorInternalServerError
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows error in reels feed", "error", err)
		return nil, users.ErrorInternalServerError
	}

	if items == nil {
		items = []models.ReelItem{}
	}
	return items, nil
}

func (u *UserRepository) GetUserReelsHistory(ctx context.Context, userID uuid.UUID) ([]models.UserReelsHistoryItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_user_reels_history", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetUserReelsHistoryQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query user reels history", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.UserReelsHistoryItem
	for rows.Next() {
		var item models.UserReelsHistoryItem
		var viewedAt time.Time
		if err := rows.Scan(&item.DanceID, &item.Score, &viewedAt, &item.Liked); err != nil {
			logger.Error("failed to scan reels history item", "error", err)
			return nil, users.ErrorInternalServerError
		}
		item.ViewedAt = viewedAt.UTC().Format(time.RFC3339)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows error in reels history", "error", err)
		return nil, users.ErrorInternalServerError
	}

	if items == nil {
		items = []models.UserReelsHistoryItem{}
	}
	return items, nil
}

func (u *UserRepository) GetReelsByIDs(ctx context.Context, ids []string, userID *uuid.UUID) ([]models.ReelItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var userIDArg interface{}
	if userID != nil {
		userIDArg = *userID
	}

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_reels_by_ids", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetReelsByIDsQuery, ids, userIDArg)
		return e
	})
	if err != nil {
		logger.Error("failed to query reels by ids", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.ReelItem
	for rows.Next() {
		var item models.ReelItem
		if err := rows.Scan(
			&item.DanceID,
			&item.Title,
			&item.UploaderID,
			&item.Username,
			&item.AvatarURL,
			&item.VideoURL,
			&item.PreviewURL,
			&item.ViewCount,
			&item.LikeCount,
			&item.AvgScore,
			&item.AttemptCount,
			&item.UserLiked,
		); err != nil {
			logger.Error("failed to scan reel item by id", "error", err)
			return nil, users.ErrorInternalServerError
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows error in reels by ids", "error", err)
		return nil, users.ErrorInternalServerError
	}
	if items == nil {
		items = []models.ReelItem{}
	}
	return items, nil
}

func (u *UserRepository) GetCandidateDancesForReels(ctx context.Context) ([]models.RecommenderDanceItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_candidate_dances_for_reels", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetCandidateDancesForReelsQuery)
		return e
	})
	if err != nil {
		logger.Error("failed to query candidate dances for reels", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.RecommenderDanceItem
	for rows.Next() {
		var item models.RecommenderDanceItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.AvgScore, &item.ViewCount, &item.UploaderID, &item.CreatedAt); err != nil {
			logger.Error("failed to scan candidate dance", "error", err)
			return nil, users.ErrorInternalServerError
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows error in candidate dances for reels", "error", err)
		return nil, users.ErrorInternalServerError
	}
	if items == nil {
		items = []models.RecommenderDanceItem{}
	}
	return items, nil
}

func (u *UserRepository) GetReelsFeedCount(ctx context.Context, excludeIDs []string) (int, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var count int
	err := appmetrics.ObserveDBQuery("get_reels_feed_count", func() error {
		return u.db.QueryRow(ctx, GetReelsFeedCountQuery, excludeIDs).Scan(&count)
	})
	if err != nil {
		logger.Error("failed to count reels", "error", err)
		return 0, users.ErrorInternalServerError
	}
	return count, nil
}
