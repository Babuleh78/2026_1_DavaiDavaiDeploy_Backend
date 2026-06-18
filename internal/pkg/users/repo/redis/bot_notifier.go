package redis

import (
	"context"
	"log/slog"

	"github.com/redis/go-redis/v9"
)

const botNotificationsKey = "bot:notifications"

type BotNotifier struct {
	client *redis.Client
	logger *slog.Logger
}

func NewBotNotifier(addr string) *BotNotifier {
	c := redis.NewClient(&redis.Options{Addr: addr})
	return &BotNotifier{client: c, logger: slog.Default()}
}

func (n *BotNotifier) Push(ctx context.Context, notification []byte) error {
	err := n.client.RPush(ctx, botNotificationsKey, notification).Err()
	if err != nil {
		n.logger.Error("bot notifier: RPUSH failed", "error", err)
		return nil
	}
	return nil
}
