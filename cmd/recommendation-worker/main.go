package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
	uuid "github.com/satori/go.uuid"

	recommendClient "DDDance/internal/pkg/recommendation/client"
	recommendUsecase "DDDance/internal/pkg/recommendation/usecase"
	userRepo "DDDance/internal/pkg/users/repo/pg"
	redisRepo "DDDance/internal/pkg/users/repo/redis"

	"DDDance/internal/pkg/kafka"
)

func main() {
	_ = godotenv.Load()

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		log.Fatal("KAFKA_BROKERS is not set")
	}
	brokers := strings.Split(kafkaBrokers, ",")

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbpool, err := initDB(ctx)
	if err != nil {
		log.Fatalf("recommendation-worker: db connect failed: %v", err)
	}
	defer dbpool.Close()

	pgRepo := userRepo.NewUserRepository(dbpool)
	mlClient := recommendClient.NewMLClient(os.Getenv("ML_SERVICE_URL"))
	uc := recommendUsecase.NewRecommendationUsecase(pgRepo, mlClient)
	uc.SetReelsCache(redisRepo.NewReelsCache(redisAddr))

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	consumer := kafka.NewKafkaConsumer(brokers, logger)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := consumer.Subscribe(ctx, kafka.TopicRecommendationWarmup, "recommendation-worker", func(msgCtx context.Context, topic, key string, value []byte) error {
			var msg struct {
				UserID string `json:"user_id"`
			}
			if err := json.Unmarshal(value, &msg); err != nil || msg.UserID == "" {
				return nil
			}
			userID, err := uuid.FromString(msg.UserID)
			if err != nil {
				return nil
			}
			warmCtx, warmCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer warmCancel()
			if _, err := uc.GetReelsFeed(warmCtx, 10, 0, nil, &userID, nil); err != nil {
				logger.Warn("recommendation-worker: warmup failed", "user_id", msg.UserID, "error", err)
			} else {
				logger.Info("recommendation-worker: cache warmed", "user_id", msg.UserID)
			}
			return nil
		})
		if err != nil {
			logger.Error("recommendation-worker kafka consumer exited", "error", err)
		}
	}()

	log.Printf("recommendation-worker started, consuming %s", kafka.TopicRecommendationWarmup)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("recommendation-worker shutting down...")
	cancel()
	wg.Wait()
	log.Println("recommendation-worker stopped")
}

func initDB(ctx context.Context) (*pgxpool.Pool, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		os.Getenv("DB_HOST"), os.Getenv("DB_PORT"),
		os.Getenv("DB_USER"), os.Getenv("DB_PASS"), os.Getenv("DB_NAME"),
	)
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	return pgxpool.ConnectConfig(ctx, config)
}
