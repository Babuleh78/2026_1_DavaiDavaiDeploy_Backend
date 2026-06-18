package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type ViewCache struct {
	client *redis.Client
}

func NewViewCache(addr string) *ViewCache {
	c := redis.NewClient(&redis.Options{Addr: addr})
	return &ViewCache{client: c}
}

func (vc *ViewCache) SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	ok, err := vc.client.SetNX(ctx, key, 1, ttl).Result()
	if err != nil {
		return true, nil
	}
	return ok, nil
}

func (vc *ViewCache) PFAdd(ctx context.Context, key, element string) {
	vc.client.PFAdd(ctx, key, element)
}

func (vc *ViewCache) PFCount(ctx context.Context, key string) (int64, error) {
	n, err := vc.client.PFCount(ctx, key).Result()
	if err != nil {
		return 0, nil
	}
	return n, nil
}
