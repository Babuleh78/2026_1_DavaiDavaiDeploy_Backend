package client

import (
	"DDDance/internal/pkg/notifications/delivery/grpc/gen"
	"context"
	"encoding/json"
	"log/slog"
	"time"

	uuid "github.com/satori/go.uuid"
	"google.golang.org/grpc"
)

type NotificationsGRPCClient struct {
	client gen.NotificationsClient
}

func NewNotificationsGRPCClient(conn grpc.ClientConnInterface) *NotificationsGRPCClient {
	return &NotificationsGRPCClient{client: gen.NewNotificationsClient(conn)}
}

func (c *NotificationsGRPCClient) SendDuelNotification(ctx context.Context, toUserID uuid.UUID, fromUserID *uuid.UUID, notifType, duelID string, telegramPayload map[string]any) error {
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	payload := make(map[string]any, len(telegramPayload)+1)
	for k, v := range telegramPayload {
		payload[k] = v
	}
	payload["duel_id"] = duelID

	payloadJSON, mErr := json.Marshal(payload)
	if mErr != nil {
		slog.Warn("notifications client: failed to marshal payload", "error", mErr)
		payloadJSON = []byte("{}")
	}

	fromID := ""
	if fromUserID != nil {
		fromID = fromUserID.String()
	}

	_, err := c.client.Send(callCtx, &gen.NotificationRequest{
		UserId:     toUserID.String(),
		FromUserId: fromID,
		Type:       notifType,
		Payload:    string(payloadJSON),
	})
	if err != nil {
		slog.Warn("notifications gRPC client: Send failed", "type", notifType, "error", err)
		return nil
	}
	return nil
}
