package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func passHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// TestRateLimiter_AllowsBurstThenBlocks verifies the in-memory limiter lets the
// burst through, then returns 429 once the bucket is empty within the window.
func TestRateLimiter_AllowsBurstThenBlocks(t *testing.T) {
	// rps low enough that no token refills during the test; burst of 2.
	rl := NewRateLimiter(0.0001, 2)
	h := rl.Middleware(passHandler())

	doReq := func() int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "203.0.113.5:12345"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if c := doReq(); c != http.StatusOK {
		t.Errorf("request 1 = %d, want 200", c)
	}
	if c := doReq(); c != http.StatusOK {
		t.Errorf("request 2 = %d, want 200 (within burst)", c)
	}
	if c := doReq(); c != http.StatusTooManyRequests {
		t.Errorf("request 3 = %d, want 429 (burst exhausted)", c)
	}
}

// TestRateLimiter_PerIPIsolation makes sure one IP's exhausted bucket does not
// affect a different IP.
func TestRateLimiter_PerIPIsolation(t *testing.T) {
	rl := NewRateLimiter(0.0001, 1)
	h := rl.Middleware(passHandler())

	send := func(addr string) int {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = addr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}

	if c := send("198.51.100.1:1000"); c != http.StatusOK {
		t.Errorf("ip1 first = %d, want 200", c)
	}
	if c := send("198.51.100.1:1000"); c != http.StatusTooManyRequests {
		t.Errorf("ip1 second = %d, want 429", c)
	}
	// Different IP starts with a full bucket.
	if c := send("198.51.100.2:1000"); c != http.StatusOK {
		t.Errorf("ip2 first = %d, want 200", c)
	}
}

func TestGetLimiter_StablePerIP(t *testing.T) {
	rl := NewRateLimiter(1, 1)
	a := rl.getLimiter("10.0.0.1")
	b := rl.getLimiter("10.0.0.1")
	if a != b {
		t.Error("getLimiter returned different limiters for the same IP")
	}
	if c := rl.getLimiter("10.0.0.2"); c == a {
		t.Error("getLimiter returned the same limiter for different IPs")
	}
}

func TestClientIP(t *testing.T) {
	cases := []struct {
		name       string
		trust      bool
		remoteAddr string
		xRealIP    string
		xff        string
		want       string
	}{
		{
			name:       "no proxy trust falls back to RemoteAddr host",
			trust:      false,
			remoteAddr: "192.0.2.10:5555",
			xRealIP:    "1.2.3.4",
			want:       "192.0.2.10",
		},
		{
			name:       "trusted X-Real-IP wins",
			trust:      true,
			remoteAddr: "10.0.0.1:5555",
			xRealIP:    "1.2.3.4",
			want:       "1.2.3.4",
		},
		{
			name:       "trusted X-Forwarded-For uses last hop",
			trust:      true,
			remoteAddr: "10.0.0.1:5555",
			xff:        "1.1.1.1, 2.2.2.2, 3.3.3.3",
			want:       "3.3.3.3",
		},
		{
			name:       "trusted but no proxy headers falls back to RemoteAddr",
			trust:      true,
			remoteAddr: "192.0.2.20:9999",
			want:       "192.0.2.20",
		},
		{
			name:       "RemoteAddr without port returned as-is",
			trust:      false,
			remoteAddr: "192.0.2.30",
			want:       "192.0.2.30",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// trustProxyHeaders is a package-level var read by clientIP; override
			// it for the test and restore afterwards.
			orig := trustProxyHeaders
			trustProxyHeaders = tc.trust
			defer func() { trustProxyHeaders = orig }()

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remoteAddr
			if tc.xRealIP != "" {
				req.Header.Set("X-Real-IP", tc.xRealIP)
			}
			if tc.xff != "" {
				req.Header.Set("X-Forwarded-For", tc.xff)
			}

			if got := clientIP(req); got != tc.want {
				t.Errorf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRedisRateLimiter_Bucket(t *testing.T) {
	rl := &RedisRateLimiter{window: 60 * time.Second}
	b1 := rl.bucket()
	b2 := rl.bucket()
	if b1 != b2 {
		t.Errorf("bucket changed within the same window: %d vs %d", b1, b2)
	}
	// The bucket index is the unix time divided by the window length in seconds.
	if want := time.Now().Unix() / 60; b1 != want {
		t.Errorf("bucket = %d, want %d", b1, want)
	}
}
