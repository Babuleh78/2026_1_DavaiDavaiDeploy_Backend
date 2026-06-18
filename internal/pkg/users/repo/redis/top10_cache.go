package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

type Top10Cache struct {
	client *redis.Client
}

func NewTop10Cache(addr string) *Top10Cache {
	c := redis.NewClient(&redis.Options{Addr: addr})
	return &Top10Cache{client: c}
}

func (c *Top10Cache) SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	ok, err := c.client.SetNX(ctx, key, 1, ttl).Result()
	if err != nil {
		return false, nil
	}
	return ok, nil
}
