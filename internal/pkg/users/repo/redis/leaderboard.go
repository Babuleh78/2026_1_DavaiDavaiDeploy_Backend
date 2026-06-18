package redis

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Leaderboard struct {
	client *redis.Client
}

func NewLeaderboard(addr string) *Leaderboard {
	return &Leaderboard{client: redis.NewClient(&redis.Options{Addr: addr})}
}

func (l *Leaderboard) ZAdd(ctx context.Context, scope string, score float64, userID string) error {
	return l.client.ZAddArgs(ctx, fmt.Sprintf("leaderboard:%s", scope), redis.ZAddArgs{
		GT:      true,
		Members: []redis.Z{{Score: score, Member: userID}},
	}).Err()
}

func (l *Leaderboard) ZTopMembers(ctx context.Context, scope string, count int64) ([]string, error) {
	return l.client.ZRevRange(ctx, fmt.Sprintf("leaderboard:%s", scope), 0, count-1).Result()
}

func (l *Leaderboard) ZRevRank(ctx context.Context, scope, userID string) (int64, error) {
	return l.client.ZRevRank(ctx, fmt.Sprintf("leaderboard:%s", scope), userID).Result()
}

func (l *Leaderboard) ZCard(ctx context.Context, scope string) (int64, error) {
	return l.client.ZCard(ctx, fmt.Sprintf("leaderboard:%s", scope)).Result()
}
