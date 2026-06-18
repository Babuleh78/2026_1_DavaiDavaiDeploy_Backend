package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/notifications"
	"DDDance/internal/pkg/utils/log"
	"context"
	"encoding/json"
	"log/slog"

	uuid "github.com/satori/go.uuid"
)

type NotificationsUsecase struct {
	repo     notifications.NotificationsRepo
	botNotif notifications.BotNotifierPort
}

func NewNotificationsUsecase(repo notifications.NotificationsRepo) *NotificationsUsecase {
	return &NotificationsUsecase{repo: repo}
}

func (uc *NotificationsUsecase) SetBotNotifier(bn notifications.BotNotifierPort) {
	uc.botNotif = bn
}

func (uc *NotificationsUsecase) Send(ctx context.Context, toUserID uuid.UUID, fromUserID *uuid.UUID, notifType, duelID string, telegramPayload map[string]any) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if storeErr := uc.repo.CreateDuelNotification(ctx, toUserID, fromUserID, notifType, duelID); storeErr != nil {
		logger.Warn("failed to store duel notification in DB", "type", notifType, "error", storeErr)
	}

	if uc.botNotif == nil {
		return nil
	}

	tid, err := uc.repo.GetUserTelegramID(ctx, toUserID)
	if err != nil || tid == nil {
		return nil
	}

	msg := map[string]any{
		"telegram_id": *tid,
		"type":        notifType,
		"payload":     telegramPayload,
	}
	data, marshalErr := json.Marshal(msg)
	if marshalErr != nil {
		return nil
	}
	if pushErr := uc.botNotif.Push(ctx, data); pushErr != nil {
		logger.Warn("bot notifier push failed", "type", notifType, "error", pushErr)
	}
	return nil
}

func (uc *NotificationsUsecase) SendDuelNotification(ctx context.Context, toUserID uuid.UUID, fromUserID *uuid.UUID, notifType, duelID string) error {
	return uc.Send(ctx, toUserID, fromUserID, notifType, duelID, map[string]any{"duel_id": duelID})
}

func (uc *NotificationsUsecase) GetUnread(ctx context.Context, userID uuid.UUID) ([]models.Notification, int, error) {
	items, err := uc.repo.GetNotifications(ctx, userID)
	if err != nil {
		return nil, 0, err
	}
	unread := 0
	for _, n := range items {
		if !n.IsRead {
			unread++
		}
	}
	return items, unread, nil
}

func (uc *NotificationsUsecase) MarkRead(ctx context.Context, id int64, userID uuid.UUID) error {
	return uc.repo.MarkNotificationRead(ctx, id, userID)
}
