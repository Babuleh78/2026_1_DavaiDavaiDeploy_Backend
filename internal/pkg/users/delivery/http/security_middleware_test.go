package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// okHandler returns 200 so tests can tell "passed the guard" from "blocked".
func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestBotSecretMiddleware(t *testing.T) {
	const secret = "super-secret-bot-token"

	cases := []struct {
		name       string
		configured string
		header     string
		setHeader  bool
		wantStatus int
	}{
		{name: "correct secret passes", configured: secret, header: secret, setHeader: true, wantStatus: http.StatusOK},
		{name: "wrong secret rejected", configured: secret, header: "nope", setHeader: true, wantStatus: http.StatusUnauthorized},
		{name: "missing header rejected", configured: secret, setHeader: false, wantStatus: http.StatusUnauthorized},
		{name: "empty header rejected", configured: secret, header: "", setHeader: true, wantStatus: http.StatusUnauthorized},
		// A blank configured secret must never authenticate, even if the caller
		// also sends a blank header — guards against a misconfigured deployment.
		{name: "unconfigured secret rejects empty header", configured: "", header: "", setHeader: true, wantStatus: http.StatusUnauthorized},
		{name: "prefix of secret rejected", configured: secret, header: secret[:len(secret)-1], setHeader: true, wantStatus: http.StatusUnauthorized},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mw := BotSecretMiddleware(tc.configured)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/bot/upload", nil)
			if tc.setHeader {
				req.Header.Set("X-Bot-Secret", tc.header)
			}

			mw(okHandler()).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestAdminTokenMiddleware(t *testing.T) {
	const token = "admin-bearer-token-value"

	cases := []struct {
		name       string
		configured string
		authHeader string
		setHeader  bool
		wantStatus int
	}{
		{name: "correct token passes", configured: token, authHeader: "Bearer " + token, setHeader: true, wantStatus: http.StatusOK},
		{name: "wrong token rejected", configured: token, authHeader: "Bearer wrong", setHeader: true, wantStatus: http.StatusUnauthorized},
		{name: "missing bearer prefix rejected", configured: token, authHeader: token, setHeader: true, wantStatus: http.StatusUnauthorized},
		{name: "missing header rejected", configured: token, setHeader: false, wantStatus: http.StatusUnauthorized},
		// No admin token configured must fail closed with 503, not authenticate.
		{name: "unconfigured token unavailable", configured: "", authHeader: "Bearer ", setHeader: true, wantStatus: http.StatusServiceUnavailable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mw := AdminTokenMiddleware(tc.configured)
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/admin/dances/x/status", nil)
			if tc.setHeader {
				req.Header.Set("Authorization", tc.authHeader)
			}

			mw(okHandler()).ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}
