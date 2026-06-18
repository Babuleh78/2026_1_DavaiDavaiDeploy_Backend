package repo

import (
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v4"
)

func (u *UserRepository) UpsertSegmentDescription(ctx context.Context, danceID string, segmentIndex int, description string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("upsert_segment_description", func() error {
		_, e := u.db.Exec(ctx, UpsertSegmentDescriptionQuery, danceID, segmentIndex, description)
		return e
	})
	if err != nil {
		logger.Error("failed to upsert segment description", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetSegmentDescription(ctx context.Context, danceID string, segmentIndex int) (string, bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var description string
	err := appmetrics.ObserveDBQuery("get_segment_description", func() error {
		return u.db.QueryRow(ctx, GetSegmentDescriptionQuery, danceID, segmentIndex).Scan(&description)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", false, nil
		}
		logger.Error("failed to get segment description", "error", err)
		return "", false, users.ErrorInternalServerError
	}
	return description, true, nil
}

func (u *UserRepository) GetDanceSegmentDescriptions(ctx context.Context, danceID string) (map[int]string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_dance_segment_descriptions", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetDanceSegmentDescriptionsQuery, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to get dance segment descriptions", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := make(map[int]string)
	for rows.Next() {
		var idx int
		var desc string
		if err := rows.Scan(&idx, &desc); err != nil {
			logger.Error("failed to scan segment description", "error", err)
			return nil, users.ErrorInternalServerError
		}
		result[idx] = desc
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDanceSegmentDescriptions", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}
