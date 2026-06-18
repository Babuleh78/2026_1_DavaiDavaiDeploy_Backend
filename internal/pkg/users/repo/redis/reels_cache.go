package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"

	"DDDance/internal/models"
	uuid "github.com/satori/go.uuid"
)

type ReelsCache struct {
	client *redis.Client
}

func NewReelsCache(addr string) *ReelsCache {
	c := redis.NewClient(&redis.Options{Addr: addr})
	return &ReelsCache{client: c}
}

func (r *ReelsCache) Get(ctx context.Context, userID uuid.UUID) (*models.ReelsFeedResponse, bool) {
	key := fmt.Sprintf("rec:%s", userID.String())
	data, err := r.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	var resp models.ReelsFeedResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, false
	}
	return &resp, true
}

func (r *ReelsCache) Set(ctx context.Context, userID uuid.UUID, resp *models.ReelsFeedResponse, ttl time.Duration) error {
	key := fmt.Sprintf("rec:%s", userID.String())
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	return r.client.Set(ctx, key, data, ttl).Err()
}
