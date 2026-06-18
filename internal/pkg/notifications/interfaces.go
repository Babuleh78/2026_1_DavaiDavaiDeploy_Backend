package notifications

import (
	"DDDance/internal/models"
	"context"

	uuid "github.com/satori/go.uuid"
)

type NotificationsUsecase interface {
	Send(ctx context.Context, toUserID uuid.UUID, fromUserID *uuid.UUID, notifType, duelID string, telegramPayload map[string]any) error
	SendDuelNotification(ctx context.Context, toUserID uuid.UUID, fromUserID *uuid.UUID, notifType, duelID string) error
	GetUnread(ctx context.Context, userID uuid.UUID) ([]models.Notification, int, error)
	MarkRead(ctx context.Context, id int64, userID uuid.UUID) error
}

type NotificationsRepo interface {
	CreateDuelNotification(ctx context.Context, toUserID uuid.UUID, fromUserID *uuid.UUID, notifType, duelID string) error
	GetNotifications(ctx context.Context, userID uuid.UUID) ([]models.Notification, error)
	MarkNotificationRead(ctx context.Context, id int64, userID uuid.UUID) error
	GetUserTelegramID(ctx context.Context, userID uuid.UUID) (*int64, error)
}

type BotNotifierPort interface {
	Push(ctx context.Context, notification []byte) error
}
