package ratelimit

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type ipEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type RateLimiter struct {
	ips   sync.Map
	rps   rate.Limit
	burst int
}

func NewRateLimiter(rps float64, burst int) *RateLimiter {
	rl := &RateLimiter{
		rps:   rate.Limit(rps),
		burst: burst,
	}
	go rl.cleanupLoop()
	return rl
}

func NewMiddleware(rps float64, burst int) func(http.Handler) http.Handler {
	return NewRateLimiter(rps, burst).Middleware
}

func (rl *RateLimiter) getLimiter(ip string) *rate.Limiter {
	if val, ok := rl.ips.Load(ip); ok {
		e := val.(*ipEntry)
		e.lastSeen = time.Now()
		return e.limiter
	}
	limiter := rate.NewLimiter(rl.rps, rl.burst)
	rl.ips.Store(ip, &ipEntry{limiter: limiter, lastSeen: time.Now()})
	return limiter
}

func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-10 * time.Minute)
		rl.ips.Range(func(key, val any) bool {
			if val.(*ipEntry).lastSeen.Before(cutoff) {
				rl.ips.Delete(key)
			}
			return true
		})
	}
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !rl.getLimiter(ip).Allow() {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
