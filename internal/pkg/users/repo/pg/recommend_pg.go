package repo

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"log/slog"

	"github.com/jackc/pgx/v4"
	uuid "github.com/satori/go.uuid"
)

func (u *UserRepository) GetDancesForRecommender(ctx context.Context) ([]models.RecommenderDanceItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_dances_for_recommender", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetDancesForRecommenderQuery)
		return e
	})
	if err != nil {
		logger.Error("failed to query dances for recommender", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.RecommenderDanceItem
	for rows.Next() {
		var item models.RecommenderDanceItem
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.AvgScore, &item.ViewCount); err != nil {
			logger.Error("failed to scan recommender dance row", "error", err)
			return nil, users.ErrorInternalServerError
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDancesForRecommender", "error", err)
		return nil, users.ErrorInternalServerError
	}
	if items == nil {
		items = []models.RecommenderDanceItem{}
	}
	return items, nil
}

func (u *UserRepository) GetFriendIDs(ctx context.Context, userID uuid.UUID) ([]string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	const q = `
		SELECT CASE WHEN sender_id = $1 THEN receiver_id ELSE sender_id END::text
		FROM friendships
		WHERE (sender_id = $1 OR receiver_id = $1) AND status = 'accepted'`
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_friend_ids", func() error {
		var e error
		rows, e = u.db.Query(ctx, q, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query friend ids", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			logger.Error("failed to scan friend id", "error", err)
			return nil, users.ErrorInternalServerError
		}
		ids = append(ids, id)
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}
