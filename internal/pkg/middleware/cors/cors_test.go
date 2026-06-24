package cors

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoadAllowedOrigins_Defaults(t *testing.T) {
	t.Setenv("CORS_ALLOWED_ORIGINS", "")
	set := loadAllowedOrigins()

	for _, want := range defaultAllowedOrigins {
		if _, ok := set[want]; !ok {
			t.Errorf("default origin %q missing from allowed set", want)
		}
	}
}

func TestLoadAllowedOrigins_FromEnv(t *testing.T) {
	// Whitespace around entries and a trailing empty segment must be tolerated.
	t.Setenv("CORS_ALLOWED_ORIGINS", " https://a.example , https://b.example ,")
	set := loadAllowedOrigins()

	if len(set) != 2 {
		t.Fatalf("len(set) = %d, want 2 (empty segment ignored)", len(set))
	}
	for _, want := range []string{"https://a.example", "https://b.example"} {
		if _, ok := set[want]; !ok {
			t.Errorf("origin %q missing from allowed set", want)
		}
	}
	// Env override must fully replace the defaults, not merge with them.
	if _, ok := set["https://dddance.ru"]; ok {
		t.Error("default origin leaked into env-configured set")
	}
}

func TestCorsMiddleware_AllowedOrigin(t *testing.T) {
	h := CorsMiddleware(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://dddance.ru")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://dddance.ru" {
		t.Errorf("Allow-Origin = %q, want https://dddance.ru", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("Allow-Credentials = %q, want true", got)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (request passed through)", rec.Code)
	}
}

func TestCorsMiddleware_DisallowedOrigin(t *testing.T) {
	h := CorsMiddleware(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Allow-Origin = %q, want empty for disallowed origin", got)
	}
	// Disallowed CORS origin still passes to the handler (CORS is browser-enforced).
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestCorsMiddleware_PreflightShortCircuits(t *testing.T) {
	called := false
	h := CorsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://dddance.ru")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if called {
		t.Error("next handler was called for OPTIONS preflight; want short-circuit")
	}
	if rec.Code != http.StatusOK {
		t.Errorf("preflight status = %d, want 200", rec.Code)
	}
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
