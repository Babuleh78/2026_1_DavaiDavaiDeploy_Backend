package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type MLLock struct {
	client *redis.Client
}

func NewMLLock(addr string) *MLLock {
	return &MLLock{client: redis.NewClient(&redis.Options{Addr: addr})}
}

func (l *MLLock) TryLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	ok, err := l.client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return true, nil // Redis unavailable — allow
	}
	return ok, nil
}

func (l *MLLock) GetValue(ctx context.Context, key string) (string, error) {
	val, err := l.client.Get(ctx, key).Result()
	if err == redis.Nil || err != nil {
		return "", nil
	}
	return val, nil
}

func (l *MLLock) Unlock(ctx context.Context, key string) error {
	return l.client.Del(ctx, key).Err()
}
