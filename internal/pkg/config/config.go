// Package config centralises environment-based configuration for the main API
// binary. Instead of scattering os.Getenv calls across packages, startup config
// is read and validated once in Load.
//
// Scope note: this covers the cmd/main service. Startup secrets and flags
// (DB/S3/JWT/bot/admin/cookie) are read here. A few request-time lookups inside
// usecases (e.g. ML_INTERNAL_TOKEN, S3_ADDRESS) are intentionally left where
// they are read; they can be migrated here over time.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const minJWTSecretLen = 32

// Config is the validated configuration for the main API binary.
type Config struct {
	DB    DBConfig
	S3    S3Config
	Redis RedisConfig
	Kafka KafkaConfig

	JWTSecret    string
	MLServiceURL string

	// BotSecret / AdminToken are shared secrets compared on every request to the
	// /bot and /admin routers. Read once here so the comparison sites receive a
	// value rather than calling os.Getenv per request.
	BotSecret  string
	AdminToken string

	// Cookie flags for auth/CSRF cookies. CookieSameSite is "Strict" or anything
	// else (treated as Lax) — kept as a raw string so net/http stays out of config.
	CookieSecure   bool
	CookieSameSite string

	MainAddr    string
	MetricsPort string

	AuthGRPC  GRPCClientConfig
	NotifGRPC GRPCClientConfig
	// AllowInsecureGRPC must be explicitly set to permit connecting to gRPC
	// services without TLS. Without it, a missing CA is a fatal error rather
	// than a silent insecure fallback.
	AllowInsecureGRPC bool

	UserVideoTTLHours             int
	UserVideoCleanupPeriodMinutes int
}

// DBConfig holds Postgres connection settings.
type DBConfig struct {
	Host, Port, User, Password, Name string
	// SSLMode is the libpq sslmode (disable/require/verify-full/...). Empty is
	// treated as "disable" so existing callers keep working.
	SSLMode string
}

// DSN renders the pgx connection string.
func (d DBConfig) DSN() string {
	sslMode := d.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	return fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		d.Host, d.Port, d.User, d.Password, d.Name, sslMode,
	)
}

// DSNFromEnv builds a Postgres DSN from the DB_* environment variables. It is
// the single source of truth for binaries that do not load the full Config
// (auth, notifications and the Kafka workers), so the connection string —
// including TLS — stays consistent across every binary.
//
// DECISION: DB_SSLMODE defaults to "disable" to avoid breaking the local/dev
// stack (its Postgres has no TLS). Prod must set DB_SSLMODE=require (or stricter)
// so DB traffic is encrypted.
func DSNFromEnv() string {
	return DBConfig{
		Host:     os.Getenv("DB_HOST"),
		Port:     os.Getenv("DB_PORT"),
		User:     os.Getenv("DB_USER"),
		Password: os.Getenv("DB_PASS"),
		Name:     os.Getenv("DB_NAME"),
		SSLMode:  envOr("DB_SSLMODE", "disable"),
	}.DSN()
}

// S3Config holds S3-compatible storage settings.
type S3Config struct {
	Endpoint    string
	Bucket      string
	AccessKey   string
	SecretKey   string
	InsecureTLS bool
}

// RedisConfig holds Redis settings. Addr == "" means Redis features are off.
type RedisConfig struct {
	Addr string
}

// Enabled reports whether Redis-backed features should be wired.
func (r RedisConfig) Enabled() bool { return r.Addr != "" }

// KafkaConfig holds Kafka settings. Empty Brokers means Kafka is off.
type KafkaConfig struct {
	Brokers []string
}

// Enabled reports whether Kafka publishing should be wired.
func (k KafkaConfig) Enabled() bool { return len(k.Brokers) > 0 }

// GRPCClientConfig holds a gRPC dial target plus optional TLS CA.
type GRPCClientConfig struct {
	Host   string
	Port   string
	CAFile string
}

// Enabled reports whether a host is configured for this client.
func (g GRPCClientConfig) Enabled() bool { return g.Host != "" }

// Target renders the host:port dial address.
func (g GRPCClientConfig) Target() string { return g.Host + ":" + g.Port }

// Load reads configuration from the environment and validates it. A non-nil
// error means the binary must not start.
func Load() (*Config, error) {
	cfg := &Config{
		DB: DBConfig{
			Host: os.Getenv("DB_HOST"), Port: os.Getenv("DB_PORT"),
			User: os.Getenv("DB_USER"), Password: os.Getenv("DB_PASS"),
			Name:    os.Getenv("DB_NAME"),
			SSLMode: envOr("DB_SSLMODE", "disable"),
		},
		S3: S3Config{
			Endpoint:    os.Getenv("AWS_S3_ENDPOINT"),
			Bucket:      os.Getenv("AWS_S3_BUCKET"),
			AccessKey:   os.Getenv("AWS_ACCESS_KEY_ID"),
			SecretKey:   os.Getenv("AWS_SECRET_ACCESS_KEY"),
			InsecureTLS: os.Getenv("S3_INSECURE_TLS") == "true",
		},
		Redis:          RedisConfig{Addr: os.Getenv("REDIS_ADDR")},
		Kafka:          KafkaConfig{Brokers: splitNonEmpty(os.Getenv("KAFKA_BROKERS"))},
		JWTSecret:      os.Getenv("JWT_SECRET"),
		MLServiceURL:   os.Getenv("ML_SERVICE_URL"),
		BotSecret:      os.Getenv("BOT_SECRET"),
		AdminToken:     os.Getenv("ADMIN_TOKEN"),
		CookieSecure:   os.Getenv("COOKIE_SECURE") == "true",
		CookieSameSite: os.Getenv("COOKIE_SAMESITE"),
		MainAddr:       ":5458",
		MetricsPort:    envOr("METRICS_PORT", "9091"),
		AuthGRPC: GRPCClientConfig{
			Host:   envOr("AUTH_SERVICE_HOST", "auth"),
			Port:   envOr("AUTH_SERVICE_PORT", "5459"),
			CAFile: os.Getenv("GRPC_TLS_CA"),
		},
		NotifGRPC: GRPCClientConfig{
			Host:   os.Getenv("NOTIFICATIONS_HOST"),
			Port:   envOr("NOTIFICATIONS_PORT", "5460"),
			CAFile: os.Getenv("GRPC_TLS_CA"),
		},
		AllowInsecureGRPC:             os.Getenv("GRPC_ALLOW_INSECURE") == "true",
		UserVideoTTLHours:             envInt("USER_VIDEO_TTL_HOURS", 24),
		UserVideoCleanupPeriodMinutes: envInt("USER_VIDEO_CLEANUP_PERIOD_MINUTES", 60),
	}

	if err := ValidateJWTSecret(cfg.JWTSecret); err != nil {
		return nil, err
	}
	return cfg, nil
}

// ValidateJWTSecret enforces a single, consistent rule for the signing secret
// across every binary that issues or verifies tokens.
func ValidateJWTSecret(secret string) error {
	if len(secret) < minJWTSecretLen {
		return fmt.Errorf("JWT_SECRET must be at least %d characters, got %d", minJWTSecretLen, len(secret))
	}
	return nil
}

// MustJWTSecret returns a validated JWT secret or an error, for binaries that do
// not load the full Config (auth service, workers).
func MustJWTSecret() (string, error) {
	secret := os.Getenv("JWT_SECRET")
	if err := ValidateJWTSecret(secret); err != nil {
		return "", err
	}
	return secret, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallback
}

func splitNonEmpty(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := parts[:0]
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
