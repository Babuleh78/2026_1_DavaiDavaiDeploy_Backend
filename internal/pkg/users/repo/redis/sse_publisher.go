package redis

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

func sseChannelKey(userID string) string {
	return fmt.Sprintf("channel:user:%s", userID)
}

type SSEPublisher struct {
	client *redis.Client
	logger *slog.Logger
}

func NewSSEPublisher(addr string) *SSEPublisher {
	return &SSEPublisher{
		client: redis.NewClient(&redis.Options{Addr: addr}),
		logger: slog.Default(),
	}
}

func (p *SSEPublisher) Publish(ctx context.Context, userID string, payload []byte) error {
	if err := p.client.Publish(ctx, sseChannelKey(userID), payload).Err(); err != nil {
		p.logger.Warn("sse publisher: PUBLISH failed", "user_id", userID, "error", err)
	}
	return nil
}
