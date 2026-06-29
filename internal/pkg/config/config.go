package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const minJWTSecretLen = 32

type Config struct {
	DB    DBConfig
	S3    S3Config
	Redis RedisConfig
	Kafka KafkaConfig

	JWTSecret    string
	MLServiceURL string

	S3Address       string
	MLInternalToken string

	BotSecret  string
	AdminToken string

	CookieSecure   bool
	CookieSameSite string

	MainAddr    string
	MetricsPort string

	AuthGRPC          GRPCClientConfig
	NotifGRPC         GRPCClientConfig
	AllowInsecureGRPC bool

	UserVideoTTLHours             int
	UserVideoCleanupPeriodMinutes int
}

type DBConfig struct {
	Host, Port, User, Password, Name string
	SSLMode                          string
}

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

type S3Config struct {
	Endpoint    string
	Bucket      string
	AccessKey   string
	SecretKey   string
	InsecureTLS bool
}

type RedisConfig struct {
	Addr string
}

func (r RedisConfig) Enabled() bool { return r.Addr != "" }

type KafkaConfig struct {
	Brokers []string
}

func (k KafkaConfig) Enabled() bool { return len(k.Brokers) > 0 }

type GRPCClientConfig struct {
	Host   string
	Port   string
	CAFile string
}

func (g GRPCClientConfig) Enabled() bool { return g.Host != "" }

func (g GRPCClientConfig) Target() string { return g.Host + ":" + g.Port }

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
		Redis:           RedisConfig{Addr: os.Getenv("REDIS_ADDR")},
		Kafka:           KafkaConfig{Brokers: splitNonEmpty(os.Getenv("KAFKA_BROKERS"))},
		JWTSecret:       os.Getenv("JWT_SECRET"),
		MLServiceURL:    os.Getenv("ML_SERVICE_URL"),
		S3Address:       os.Getenv("S3_ADDRESS"),
		MLInternalToken: os.Getenv("ML_INTERNAL_TOKEN"),
		BotSecret:       os.Getenv("BOT_SECRET"),
		AdminToken:      os.Getenv("ADMIN_TOKEN"),
		CookieSecure:    os.Getenv("COOKIE_SECURE") == "true",
		CookieSameSite:  os.Getenv("COOKIE_SAMESITE"),
		MainAddr:        ":5458",
		MetricsPort:     envOr("METRICS_PORT", "9091"),
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

func ValidateJWTSecret(secret string) error {
	if len(secret) < minJWTSecretLen {
		return fmt.Errorf("JWT_SECRET must be at least %d characters, got %d", minJWTSecretLen, len(secret))
	}
	return nil
}

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
