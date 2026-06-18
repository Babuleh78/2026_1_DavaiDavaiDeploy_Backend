package repo

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"log/slog"

	"github.com/jackc/pgx/v4"
)

func (u *UserRepository) GetTopDancers(ctx context.Context) ([]models.TopDancerEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_top_dancers", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetTopDancersQuery)
		return e
	})
	if err != nil {
		logger.Error("failed to query top dancers", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var entries []models.TopDancerEntry
	for rows.Next() {
		var e models.TopDancerEntry
		if err := rows.Scan(&e.UserID, &e.Username, &e.Avatar, &e.AvgScore, &e.AttemptCount, &e.BestScore); err != nil {
			logger.Error("failed to scan top dancer row", "error", err)
			return nil, users.ErrorInternalServerError
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetTopDancers", "error", err)
		return nil, users.ErrorInternalServerError
	}
	if entries == nil {
		entries = []models.TopDancerEntry{}
	}
	return entries, nil
}

func (u *UserRepository) GetTopDancersByIDs(ctx context.Context, userIDs []string) ([]models.TopDancerEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_top_dancers_by_ids", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetTopDancersByIDsQuery, userIDs)
		return e
	})
	if err != nil {
		logger.Error("failed to query top dancers by ids", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var entries []models.TopDancerEntry
	for rows.Next() {
		var e models.TopDancerEntry
		if err := rows.Scan(&e.UserID, &e.Username, &e.Avatar, &e.AvgScore, &e.AttemptCount, &e.BestScore); err != nil {
			logger.Error("failed to scan top dancer row", "error", err)
			return nil, users.ErrorInternalServerError
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetTopDancersByIDs", "error", err)
		return nil, users.ErrorInternalServerError
	}
	if entries == nil {
		entries = []models.TopDancerEntry{}
	}
	return entries, nil
}

func (u *UserRepository) GetTopDances(ctx context.Context) ([]models.DanceCatalogItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_top_dances", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetTopDancesQuery)
		return e
	})
	if err != nil {
		logger.Error("failed to query top dances", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.DanceCatalogItem
	for rows.Next() {
		var item models.DanceCatalogItem
		if err := rows.Scan(&item.ID, &item.Title, &item.AttemptCount, &item.AvgScore, &item.ViewCount, &item.LikeCount); err != nil {
			logger.Error("failed to scan top dance row", "error", err)
			return nil, users.ErrorInternalServerError
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetTopDances", "error", err)
		return nil, users.ErrorInternalServerError
	}
	if items == nil {
		items = []models.DanceCatalogItem{}
	}
	return items, nil
}
