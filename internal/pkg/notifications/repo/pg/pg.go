package pg

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/utils/log"
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgtype/pgxtype"
	"github.com/jackc/pgx/v4"
	uuid "github.com/satori/go.uuid"
)

type NotificationsRepository struct {
	db pgxtype.Querier
}

func NewNotificationsRepository(db pgxtype.Querier) *NotificationsRepository {
	return &NotificationsRepository{db: db}
}

func (r *NotificationsRepository) CreateDuelNotification(ctx context.Context, toUserID uuid.UUID, fromUserID *uuid.UUID, notifType, duelID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var fromArg interface{}
	if fromUserID != nil {
		fromArg = *fromUserID
	}
	err := appmetrics.ObserveDBQuery("notif_create_duel_notification", func() error {
		_, e := r.db.Exec(ctx, createDuelNotificationQuery, toUserID, notifType, duelID, fromArg)
		return e
	})
	if err != nil {
		logger.Error("failed to create duel notification", "error", err)
		return err
	}
	return nil
}

func (r *NotificationsRepository) GetNotifications(ctx context.Context, userID uuid.UUID) ([]models.Notification, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	var err error
	err = appmetrics.ObserveDBQuery("notif_get_notifications", func() error {
		rows, err = r.db.Query(ctx, getNotificationsQuery, userID)
		return err
	})
	if err != nil {
		logger.Error("failed to query notifications", "error", err)
		return nil, err
	}
	defer rows.Close()

	var items []models.Notification
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.Type, &n.DanceID, &n.Reason, &n.IsRead, &n.CreatedAt, &n.FromUserID, &n.FromLogin, &n.RefID); err != nil {
			logger.Error("failed to scan notification", "error", err)
			return nil, err
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetNotifications", "error", err)
		return nil, err
	}
	return items, nil
}

func (r *NotificationsRepository) MarkNotificationRead(ctx context.Context, id int64, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("notif_mark_notification_read", func() error {
		_, e := r.db.Exec(ctx, markNotificationReadQuery, id, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to mark notification read", "error", err)
		return err
	}
	return nil
}

func (r *NotificationsRepository) GetUserTelegramID(ctx context.Context, userID uuid.UUID) (*int64, error) {
	var tid *int64
	err := appmetrics.ObserveDBQuery("notif_get_user_telegram_id", func() error {
		return r.db.QueryRow(ctx, getUserTelegramIDQuery, userID).Scan(&tid)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return tid, nil
}
