package repo

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"github.com/jackc/pgx/v4"
	uuid "github.com/satori/go.uuid"
	"log/slog"
)

func (u *UserRepository) AddToHistory(ctx context.Context, userID uuid.UUID, danceID string, sourceURL string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("add_to_history", func() error {
		_, e := u.db.Exec(ctx, AddToHistoryQuery, userID, danceID, sourceURL)
		return e
	})
	if err != nil {
		logger.Error("failed to add to history", "error", err)
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
		logger.Error("failed to get history", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var items []models.SearchHistoryItem
	for rows.Next() {
		var item models.SearchHistoryItem
		if err := rows.Scan(&item.ID, &item.UserID, &item.DanceID, &item.Name, &item.SourceURL, &item.CreatedAt, &item.DanceTitle, &item.Score); err != nil {
			logger.Error("failed to scan history item", "error", err)
			return nil, users.ErrorInternalServerError
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetHistory", "error", err)
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
		logger.Error("failed to delete from history", "error", err)
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
		logger.Error("failed to update history name", "error", err)
		return users.ErrorInternalServerError
	}
	if rowsAffected == 0 {
		return users.ErrorNotFound
	}
	logger.Info("successfully updated history item name")
	return nil
}

func (u *UserRepository) CleanHistory(ctx context.Context, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("clean_history", func() error {
		_, e := u.db.Exec(ctx, CleanHistoryQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to clean history", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}
