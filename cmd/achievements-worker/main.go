package main

import (
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
	uuid "github.com/satori/go.uuid"

	"DDDance/internal/pkg/config"
	"DDDance/internal/pkg/kafka"
	userRepo "DDDance/internal/pkg/users/repo/pg"
	redisRepo "DDDance/internal/pkg/users/repo/redis"
	userUsecase "DDDance/internal/pkg/users/usecase"
)

func main() {
	_ = godotenv.Load()

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		log.Fatal("KAFKA_BROKERS is not set")
	}
	brokers := strings.Split(kafkaBrokers, ",")

	jwtSecret, err := config.MustJWTSecret()
	if err != nil {
		log.Fatalf("achievements-worker: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbpool, err := initDB(ctx)
	if err != nil {
		log.Fatalf("achievements-worker: db connect failed: %v", err)
	}
	defer dbpool.Close()

	pgRepo := userRepo.NewUserRepository(dbpool)
	uc := userUsecase.NewUserUsecase(pgRepo, nil, jwtSecret)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	consumer := kafka.NewKafkaConsumer(brokers, logger)
	producer := kafka.NewKafkaProducer(brokers)

	var ssePub *redisRepo.SSEPublisher
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		ssePub = redisRepo.NewSSEPublisher(redisAddr)
		log.Println("achievements-worker: SSE publisher enabled:", redisAddr)
	}

	unlockAndNotify := func(ctx context.Context, userID uuid.UUID) {
		newly, err := uc.CheckAndUnlockAchievements(ctx, userID)
		if err != nil {
			logger.Warn("CheckAndUnlockAchievements failed", "user_id", userID, "error", err)
			return
		}
		if ssePub == nil || len(newly) == 0 {
			return
		}
		for _, a := range newly {
			payload, mErr := json.Marshal(map[string]interface{}{
				"type":        "achievement_unlocked",
				"id":          a.ID,
				"title":       a.Title,
				"description": a.Description,
				"icon_key":    a.IconKey,
			})
			if mErr != nil {
				continue
			}
			_ = ssePub.Publish(ctx, userID.String(), payload)
		}
	}

	var wg sync.WaitGroup

	startConsumer := func(topic, groupID string, handler kafka.MessageHandler) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := consumer.Subscribe(ctx, topic, groupID, handler); err != nil {
				logger.Error("kafka consumer exited", "topic", topic, "error", err)
			}
		}()
	}

	startConsumer(kafka.TopicAttemptSaved, "achievements-worker-attempt", func(ctx context.Context, topic, key string, value []byte) error {
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
		unlockAndNotify(ctx, userID)
		return nil
	})

	startConsumer(kafka.TopicDanceUploaded, "achievements-worker-dance", func(ctx context.Context, topic, key string, value []byte) error {
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
		unlockAndNotify(ctx, userID)
		return nil
	})

	startConsumer(kafka.TopicLikeToggled, "achievements-worker-like", func(ctx context.Context, topic, key string, value []byte) error {
		var msg struct {
			UserID string `json:"user_id"`
			Liked  bool   `json:"liked"`
		}
		if err := json.Unmarshal(value, &msg); err != nil || msg.UserID == "" || !msg.Liked {
			return nil
		}
		userID, err := uuid.FromString(msg.UserID)
		if err != nil {
			return nil
		}
		unlockAndNotify(ctx, userID)
		return nil
	})

	startConsumer(kafka.TopicDuelCompleted, "achievements-worker-duel", func(ctx context.Context, topic, key string, value []byte) error {
		var msg struct {
			ChallengerID string `json:"challenger_id"`
			OpponentID   string `json:"opponent_id"`
		}
		if err := json.Unmarshal(value, &msg); err != nil {
			return nil
		}
		for _, idStr := range []string{msg.ChallengerID, msg.OpponentID} {
			if idStr == "" {
				continue
			}
			uid, err := uuid.FromString(idStr)
			if err != nil {
				continue
			}
			unlockAndNotify(ctx, uid)
		}
		return nil
	})

	startConsumer(kafka.TopicAttemptImproved, "achievements-worker-improved", func(ctx context.Context, topic, key string, value []byte) error {
		var msg struct {
			UserID     string  `json:"user_id"`
			DanceTitle string  `json:"dance_title"`
			NewScore   float64 `json:"new_score"`
			Delta      float64 `json:"delta"`
		}
		if err := json.Unmarshal(value, &msg); err != nil || msg.UserID == "" {
			return nil
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"to_user_id": msg.UserID,
			"type":       "personal_record",
			"telegram_payload": map[string]interface{}{
				"dance_title": msg.DanceTitle,
				"new_score":   msg.NewScore,
				"delta":       msg.Delta,
			},
		})
		producer.PublishAsync(ctx, kafka.TopicNotificationSend, msg.UserID, payload, func(pubErr error) {
			logger.Warn("achievements-worker: publish notification.send failed", "user_id", msg.UserID, "error", pubErr)
		})
		return nil
	})

	log.Printf("achievements-worker started, consuming %s, %s, %s, %s, %s",
		kafka.TopicAttemptSaved, kafka.TopicDanceUploaded, kafka.TopicLikeToggled, kafka.TopicDuelCompleted, kafka.TopicAttemptImproved)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("achievements-worker shutting down...")
	cancel()
	wg.Wait()
	log.Println("achievements-worker stopped")
}

func initDB(ctx context.Context) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(config.DSNFromEnv())
	if err != nil {
		return nil, err
	}
	return pgxpool.ConnectConfig(ctx, poolCfg)
}
