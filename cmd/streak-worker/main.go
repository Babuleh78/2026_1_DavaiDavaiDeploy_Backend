package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"

	"DDDance/internal/pkg/kafka"
)

var streakMilestones = map[int64]bool{3: true, 7: true, 30: true}

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

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	defer rdb.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	producer := kafka.NewKafkaProducer(brokers)
	consumer := kafka.NewKafkaConsumer(brokers, logger)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := consumer.Subscribe(ctx, kafka.TopicAttemptSaved, "streak-worker", func(msgCtx context.Context, topic, key string, value []byte) error {
			return handleAttemptSaved(msgCtx, rdb, producer, logger, value)
		})
		if err != nil {
			logger.Error("streak-worker kafka consumer exited", "error", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		streakReminderCron(ctx, rdb, producer, logger)
	}()

	log.Printf("streak-worker started, consuming %s", kafka.TopicAttemptSaved)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("streak-worker shutting down...")
	cancel()
	wg.Wait()
	log.Println("streak-worker stopped")
}

func handleAttemptSaved(ctx context.Context, rdb *redis.Client, producer *kafka.KafkaProducer, logger *slog.Logger, value []byte) error {
	var msg struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(value, &msg); err != nil || msg.UserID == "" {
		return nil
	}

	today := utcToday()
	streakKey := fmt.Sprintf("streak:%s", msg.UserID)
	lastKey := fmt.Sprintf("streak_last:%s", msg.UserID)

	lastDate, getErr := rdb.Get(ctx, lastKey).Result()
	if getErr != nil && getErr != redis.Nil {
		logger.Warn("streak-worker: get last date failed", "user_id", msg.UserID, "error", getErr)
		return nil
	}

	var newStreak int64
	switch {
	case lastDate == today:
		return nil
	case lastDate == utcYesterday():
		var incrErr error
		newStreak, incrErr = rdb.Incr(ctx, streakKey).Result()
		if incrErr != nil {
			logger.Warn("streak-worker: incr streak failed", "user_id", msg.UserID, "error", incrErr)
			return nil
		}
	default:
		rdb.Set(ctx, streakKey, 1, 48*time.Hour)
		newStreak = 1
	}

	rdb.Set(ctx, lastKey, today, 48*time.Hour)
	rdb.Expire(ctx, streakKey, 48*time.Hour)
	rdb.Expire(ctx, lastKey, 48*time.Hour)

	if streakMilestones[newStreak] {
		publishStreakNotif(ctx, producer, logger, msg.UserID, "streak_milestone", newStreak)
	}
	return nil
}

func streakReminderCron(ctx context.Context, rdb *redis.Client, producer *kafka.KafkaProducer, logger *slog.Logger) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runStreakReminders(ctx, rdb, producer, logger)
		}
	}
}

func runStreakReminders(ctx context.Context, rdb *redis.Client, producer *kafka.KafkaProducer, logger *slog.Logger) {
	today := utcToday()
	yesterday := utcYesterday()

	iter := rdb.Scan(ctx, 0, "streak:*", 100).Iterator()
	for iter.Next(ctx) {
		streakKey := iter.Val()
		userID := strings.TrimPrefix(streakKey, "streak:")
		lastKey := fmt.Sprintf("streak_last:%s", userID)

		lastDate, getErr := rdb.Get(ctx, lastKey).Result()
		if getErr != nil && getErr != redis.Nil {
			logger.Warn("streak-worker: get last date failed", "user_id", userID, "error", getErr)
			continue
		}
		if lastDate != yesterday {
			continue
		}

		notifKey := fmt.Sprintf("streak_notified:%s:%s", userID, today)
		ok, setErr := rdb.SetNX(ctx, notifKey, "1", 24*time.Hour).Result()
		if setErr != nil {
			logger.Warn("streak-worker: SetNX notify flag failed", "user_id", userID, "error", setErr)
			continue
		}
		if !ok {
			continue
		}

		streakVal, getErr := rdb.Get(ctx, streakKey).Result()
		if getErr != nil && getErr != redis.Nil {
			logger.Warn("streak-worker: get streak value failed", "user_id", userID, "error", getErr)
			continue
		}
		streak, parseErr := strconv.ParseInt(streakVal, 10, 64)
		if parseErr != nil || streak <= 0 {
			continue
		}

		publishStreakNotif(ctx, producer, logger, userID, "streak_reminder", streak)
	}
	if err := iter.Err(); err != nil {
		logger.Warn("streak-worker: scan error", "error", err)
	}
}

func publishStreakNotif(ctx context.Context, producer *kafka.KafkaProducer, logger *slog.Logger, userID, notifType string, streak int64) {
	payload, mErr := json.Marshal(map[string]interface{}{
		"to_user_id": userID,
		"type":       notifType,
		"telegram_payload": map[string]interface{}{
			"streak": streak,
		},
	})
	if mErr != nil {
		logger.Warn("streak-worker: marshal notification payload failed", "user_id", userID, "error", mErr)
		return
	}
	producer.PublishAsync(ctx, kafka.TopicNotificationSend, userID, payload, func(err error) {
		logger.Warn("streak-worker: publish notification.send failed", "user_id", userID, "type", notifType, "error", err)
	})
}

func utcToday() string {
	return time.Now().UTC().Format("2006-01-02")
}

func utcYesterday() string {
	return time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
}
