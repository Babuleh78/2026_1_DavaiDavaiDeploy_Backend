package ratelimit

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/users"
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var trustProxyHeaders = os.Getenv("TRUST_PROXY_HEADERS") == "true"

type RedisRateLimiter struct {
	client   *redis.Client
	limit    int64
	window   time.Duration
	endpoint string
	logger   *slog.Logger
}

func NewRedisRateLimiter(addr string, limit int, window time.Duration, endpoint string) *RedisRateLimiter {
	return &RedisRateLimiter{
		client:   redis.NewClient(&redis.Options{Addr: addr}),
		limit:    int64(limit),
		window:   window,
		endpoint: endpoint,
		logger:   slog.Default(),
	}
}

func (rl *RedisRateLimiter) identity(r *http.Request) string {
	if u, ok := r.Context().Value(users.UserKey).(models.User); ok && u.ID.String() != "" {
		return u.ID.String()
	}
	return clientIP(r)
}

func clientIP(r *http.Request) string {
	if trustProxyHeaders {
		if xrip := strings.TrimSpace(r.Header.Get("X-Real-IP")); xrip != "" {
			return xrip
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			last := strings.TrimSpace(parts[len(parts)-1])
			if last != "" {
				return last
			}
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func (rl *RedisRateLimiter) bucket() int64 {
	return time.Now().Unix() / int64(rl.window.Seconds())
}

func (rl *RedisRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := fmt.Sprintf("rate:%s:%s:%d", rl.identity(r), rl.endpoint, rl.bucket())

		count, err := rl.increment(r.Context(), key)
		if err != nil {
			rl.logger.Warn("redis rate limiter unavailable, allowing request",
				"endpoint", rl.endpoint, "error", err)
			next.ServeHTTP(w, r)
			return
		}

		if count > rl.limit {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RedisRateLimiter) increment(ctx context.Context, key string) (int64, error) {
	pipe := rl.client.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, rl.window+time.Second) // +1s buffer so key outlives the window
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incr.Val(), nil
}
