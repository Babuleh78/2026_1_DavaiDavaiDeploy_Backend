package config

import "testing"

func TestValidateJWTSecret(t *testing.T) {
	cases := []struct {
		name    string
		secret  string
		wantErr bool
	}{
		{name: "empty rejected", secret: "", wantErr: true},
		{name: "too short rejected", secret: "short", wantErr: true},
		{name: "exactly min length ok", secret: string(make([]byte, minJWTSecretLen)), wantErr: false},
		{name: "long secret ok", secret: string(make([]byte, minJWTSecretLen+10)), wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateJWTSecret(tc.secret)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateJWTSecret(%q) err=%v, wantErr=%v", tc.secret, err, tc.wantErr)
			}
		})
	}
}

func TestSplitNonEmpty(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{name: "empty", in: "", want: nil},
		{name: "whitespace only", in: "   ", want: nil},
		{name: "single", in: "a", want: []string{"a"}},
		{name: "multiple with spaces", in: " a , b ,c ", want: []string{"a", "b", "c"}},
		{name: "drops empty segments", in: "a,,b,", want: []string{"a", "b"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitNonEmpty(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("splitNonEmpty(%q) = %v, want %v", tc.in, got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("splitNonEmpty(%q) = %v, want %v", tc.in, got, tc.want)
				}
			}
		})
	}
}

func TestEnvInt(t *testing.T) {
	t.Run("unset uses fallback", func(t *testing.T) {
		if got := envInt("CONFIG_TEST_INT_UNSET", 42); got != 42 {
			t.Errorf("got %d, want fallback 42", got)
		}
	})
	t.Run("valid value parsed", func(t *testing.T) {
		t.Setenv("CONFIG_TEST_INT", "7")
		if got := envInt("CONFIG_TEST_INT", 42); got != 7 {
			t.Errorf("got %d, want 7", got)
		}
	})
	t.Run("non-positive falls back", func(t *testing.T) {
		t.Setenv("CONFIG_TEST_INT", "0")
		if got := envInt("CONFIG_TEST_INT", 42); got != 42 {
			t.Errorf("got %d, want fallback 42 for non-positive value", got)
		}
	})
	t.Run("garbage falls back", func(t *testing.T) {
		t.Setenv("CONFIG_TEST_INT", "notanumber")
		if got := envInt("CONFIG_TEST_INT", 42); got != 42 {
			t.Errorf("got %d, want fallback 42 for unparseable value", got)
		}
	})
}

// TestLoadReadsSecretsAndCookieFlags covers the centralised reads added so the
// bot/admin secrets and cookie flags come from config rather than scattered
// os.Getenv calls at request time.
func TestLoadReadsSecretsAndCookieFlags(t *testing.T) {
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef") // 32 chars, satisfies validation
	t.Setenv("BOT_SECRET", "bot-secret-value")
	t.Setenv("ADMIN_TOKEN", "admin-token-value")
	t.Setenv("COOKIE_SECURE", "true")
	t.Setenv("COOKIE_SAMESITE", "Strict")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.BotSecret != "bot-secret-value" {
		t.Errorf("BotSecret = %q, want %q", cfg.BotSecret, "bot-secret-value")
	}
	if cfg.AdminToken != "admin-token-value" {
		t.Errorf("AdminToken = %q, want %q", cfg.AdminToken, "admin-token-value")
	}
	if !cfg.CookieSecure {
		t.Error("CookieSecure = false, want true")
	}
	if cfg.CookieSameSite != "Strict" {
		t.Errorf("CookieSameSite = %q, want %q", cfg.CookieSameSite, "Strict")
	}
}

func TestLoadFailsOnShortJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "too-short")
	if _, err := Load(); err == nil {
		t.Error("Load should fail when JWT_SECRET is shorter than the minimum")
	}
}
