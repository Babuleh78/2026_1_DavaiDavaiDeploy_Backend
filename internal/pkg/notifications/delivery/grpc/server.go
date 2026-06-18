package grpc

import (
	"DDDance/internal/pkg/notifications"
	"DDDance/internal/pkg/notifications/delivery/grpc/gen"
	"context"
	"encoding/json"
	"log/slog"

	uuid "github.com/satori/go.uuid"
)

type NotificationsGRPCServer struct {
	gen.UnimplementedNotificationsServer
	uc notifications.NotificationsUsecase
}

func NewNotificationsGRPCServer(uc notifications.NotificationsUsecase) *NotificationsGRPCServer {
	return &NotificationsGRPCServer{uc: uc}
}

func (s *NotificationsGRPCServer) Send(ctx context.Context, req *gen.NotificationRequest) (*gen.NotificationResponse, error) {
	toUserID, err := uuid.FromString(req.GetUserId())
	if err != nil {
		slog.Warn("notifications gRPC Send: invalid user_id", "user_id", req.GetUserId())
		return &gen.NotificationResponse{Ok: false}, nil
	}

	var fromUserID *uuid.UUID
	if req.GetFromUserId() != "" {
		id, parseErr := uuid.FromString(req.GetFromUserId())
		if parseErr == nil {
			fromUserID = &id
		}
	}

	var payload map[string]any
	if req.GetPayload() != "" {
		_ = json.Unmarshal([]byte(req.GetPayload()), &payload)
	}
	if payload == nil {
		payload = map[string]any{}
	}

	duelID, _ := payload["duel_id"].(string)

	if sendErr := s.uc.Send(ctx, toUserID, fromUserID, req.GetType(), duelID, payload); sendErr != nil {
		slog.Warn("notifications gRPC Send: usecase error", "type", req.GetType(), "error", sendErr)
		return &gen.NotificationResponse{Ok: false}, nil
	}

	return &gen.NotificationResponse{Ok: true}, nil
}

func (s *NotificationsGRPCServer) GetUnread(ctx context.Context, req *gen.UserRequest) (*gen.NotificationsResponse, error) {
	userID, err := uuid.FromString(req.GetUserId())
	if err != nil {
		return &gen.NotificationsResponse{}, nil
	}

	items, unreadCount, err := s.uc.GetUnread(ctx, userID)
	if err != nil {
		slog.Warn("notifications gRPC GetUnread: usecase error", "error", err)
		return &gen.NotificationsResponse{}, nil
	}

	protoItems := make([]*gen.NotificationItem, 0, len(items))
	for _, n := range items {
		protoItems = append(protoItems, &gen.NotificationItem{
			Id:            n.ID,
			Type:          n.Type,
			FromUserId:    n.FromUserID,
			IsRead:        n.IsRead,
			CreatedAtUnix: n.CreatedAt.Unix(),
		})
	}

	return &gen.NotificationsResponse{
		Notifications: protoItems,
		UnreadCount:   int32(unreadCount),
	}, nil
}
