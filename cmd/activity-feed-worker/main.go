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
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
	uuid "github.com/satori/go.uuid"

	"DDDance/internal/pkg/config"
	"DDDance/internal/pkg/kafka"
)

type friendCountCache struct {
	mu    sync.Mutex
	items map[string]friendCountEntry
}

type friendCountEntry struct {
	count     int
	expiresAt time.Time
}

func (c *friendCountCache) get(key string) (int, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.items[key]
	if !ok || time.Now().After(e.expiresAt) {
		return 0, false
	}
	return e.count, true
}

func (c *friendCountCache) set(key string, count int, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = friendCountEntry{count: count, expiresAt: time.Now().Add(ttl)}
}

const insertActivityFeedQuery = `
INSERT INTO activity_feed (actor_id, action_type, metadata, source_event_id)
VALUES ($1, $2, $3, $4)
ON CONFLICT (source_event_id) DO NOTHING`

const duelFeedQuery = `
SELECT d.dance_id, d.is_public,
       d.challenger_score, d.opponent_score,
       d.winner_id::text, d.challenger_attempt_id::text, d.opponent_attempt_id::text,
       cu.login AS challenger_login, ou.login AS opponent_login
FROM duels d
JOIN user_table cu ON cu.id = d.challenger_id
JOIN user_table ou ON ou.id = d.opponent_id
WHERE d.id = $1`

func insertFeedEvent(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger, actorID uuid.UUID, actionType, sourceEventID string, meta map[string]interface{}) {
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		logger.Warn("activity-feed: failed to marshal metadata", "error", err)
		return
	}
	if _, err := db.Exec(ctx, insertActivityFeedQuery, actorID, actionType, metaJSON, sourceEventID); err != nil {
		logger.Warn("activity-feed: failed to insert event", "action_type", actionType, "actor_id", actorID, "error", err)
	}
}

const friendCountCacheTTL = 5 * time.Minute

func logFriendCount(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger, cache *friendCountCache, actorID uuid.UUID, actionType string) {
	key := actorID.String()
	count, ok := cache.get(key)
	if !ok {
		const q = `
			SELECT COUNT(*)
			FROM friendships
			WHERE (sender_id = $1 OR receiver_id = $1) AND status = 'accepted'`
		if err := db.QueryRow(ctx, q, actorID).Scan(&count); err != nil {
			return
		}
		cache.set(key, count, friendCountCacheTTL)
	}
	logger.Info("activity-feed: event written", "action_type", actionType, "actor_id", actorID, "friend_count", count)
}

func duelResult(winnerID *string, actorID string) string {
	if winnerID == nil || *winnerID == "" {
		return "draw"
	}
	if *winnerID == actorID {
		return "win"
	}
	return "loss"
}

func buildDuelMeta(duelID string, isPublic bool, danceID, attemptID *string, oppLogin, result string, myScore, oppScore *float64) map[string]interface{} {
	meta := map[string]interface{}{
		"duel_id":        duelID,
		"is_public":      isPublic,
		"opponent_login": oppLogin,
		"result":         result,
	}
	if danceID != nil {
		meta["dance_id"] = *danceID
	}
	if attemptID != nil {
		meta["attempt_id"] = *attemptID
	}
	if myScore != nil {
		meta["my_score"] = *myScore
	}
	if oppScore != nil {
		meta["opp_score"] = *oppScore
	}
	return meta
}

func insertDuelFeed(ctx context.Context, db *pgxpool.Pool, logger *slog.Logger, cache *friendCountCache, challengerID, opponentID, duelID string) {
	var danceID, winnerID, chAttempt, opAttempt *string
	var isPublic bool
	var chScore, opScore *float64
	var chLogin, opLogin string

	err := db.QueryRow(ctx, duelFeedQuery, duelID).Scan(
		&danceID, &isPublic, &chScore, &opScore,
		&winnerID, &chAttempt, &opAttempt, &chLogin, &opLogin,
	)
	if err != nil {
		logger.Warn("activity-feed: failed to load duel for feed", "duel_id", duelID, "error", err)
		return
	}

	if chUUID, err := uuid.FromString(challengerID); err == nil {
		meta := buildDuelMeta(duelID, isPublic, danceID, chAttempt, opLogin, duelResult(winnerID, challengerID), chScore, opScore)
		insertFeedEvent(ctx, db, logger, chUUID, "duel_completed", "duel_completed:"+duelID+":"+challengerID, meta)
		logFriendCount(ctx, db, logger, cache, chUUID, "duel_completed")
	}

	if opUUID, err := uuid.FromString(opponentID); err == nil {
		meta := buildDuelMeta(duelID, isPublic, danceID, opAttempt, chLogin, duelResult(winnerID, opponentID), opScore, chScore)
		insertFeedEvent(ctx, db, logger, opUUID, "duel_completed", "duel_completed:"+duelID+":"+opponentID, meta)
		logFriendCount(ctx, db, logger, cache, opUUID, "duel_completed")
	}
}

func main() {
	_ = godotenv.Load()

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		log.Fatal("KAFKA_BROKERS is not set")
	}
	brokers := strings.Split(kafkaBrokers, ",")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbpool, err := initDB(ctx)
	if err != nil {
		log.Fatalf("activity-feed-worker: db connect failed: %v", err)
	}
	defer dbpool.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	consumer := kafka.NewKafkaConsumer(brokers, logger)
	fcCache := &friendCountCache{items: make(map[string]friendCountEntry)}

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

	startConsumer(kafka.TopicAttemptSaved, "activity-feed-worker-attempt", func(ctx context.Context, topic, key string, value []byte) error {
		var msg struct {
			UserID    string  `json:"user_id"`
			DanceID   string  `json:"dance_id"`
			AttemptID string  `json:"attempt_id"`
			Score     float64 `json:"score"`
			IsPrivate bool    `json:"is_private"`
		}
		if err := json.Unmarshal(value, &msg); err != nil || msg.UserID == "" {
			return nil
		}
		if msg.IsPrivate {
			return nil
		}
		actorID, err := uuid.FromString(msg.UserID)
		if err != nil {
			return nil
		}
		meta := map[string]interface{}{
			"dance_id":   msg.DanceID,
			"attempt_id": msg.AttemptID,
			"score":      msg.Score,
		}
		insertFeedEvent(ctx, dbpool, logger, actorID, "attempt_saved", "attempt_saved:"+msg.AttemptID, meta)
		logFriendCount(ctx, dbpool, logger, fcCache, actorID, "attempt_saved")
		return nil
	})

	startConsumer(kafka.TopicDanceUploaded, "activity-feed-worker-dance", func(ctx context.Context, topic, key string, value []byte) error {
		var msg struct {
			UserID  string `json:"user_id"`
			DanceID string `json:"dance_id"`
		}
		if err := json.Unmarshal(value, &msg); err != nil || msg.UserID == "" {
			return nil
		}
		actorID, err := uuid.FromString(msg.UserID)
		if err != nil {
			return nil
		}
		meta := map[string]interface{}{
			"dance_id": msg.DanceID,
		}
		insertFeedEvent(ctx, dbpool, logger, actorID, "dance_uploaded", "dance_uploaded:"+msg.DanceID, meta)
		logFriendCount(ctx, dbpool, logger, fcCache, actorID, "dance_uploaded")
		return nil
	})

	startConsumer(kafka.TopicLikeToggled, "activity-feed-worker-like", func(ctx context.Context, topic, key string, value []byte) error {
		var msg struct {
			UserID  string `json:"user_id"`
			LikerID string `json:"liker_id"`
			DanceID string `json:"dance_id"`
			Liked   bool   `json:"liked"`
		}
		if err := json.Unmarshal(value, &msg); err != nil || !msg.Liked {
			return nil
		}
		actorIDStr := msg.LikerID
		if actorIDStr == "" {
			actorIDStr = msg.UserID
		}
		actorID, err := uuid.FromString(actorIDStr)
		if err != nil {
			return nil
		}
		meta := map[string]interface{}{
			"dance_id": msg.DanceID,
		}
		insertFeedEvent(ctx, dbpool, logger, actorID, "like", "like:"+actorIDStr+":"+msg.DanceID, meta)
		logFriendCount(ctx, dbpool, logger, fcCache, actorID, "like")
		return nil
	})

	startConsumer(kafka.TopicDuelCompleted, "activity-feed-worker-duel", func(ctx context.Context, topic, key string, value []byte) error {
		var msg struct {
			ChallengerID string `json:"challenger_id"`
			OpponentID   string `json:"opponent_id"`
			DuelID       string `json:"duel_id"`
		}
		if err := json.Unmarshal(value, &msg); err != nil || msg.DuelID == "" {
			return nil
		}
		if msg.ChallengerID == "" || msg.OpponentID == "" {
			return nil
		}
		insertDuelFeed(ctx, dbpool, logger, fcCache, msg.ChallengerID, msg.OpponentID, msg.DuelID)
		return nil
	})

	log.Printf("activity-feed-worker started, consuming %s, %s, %s, %s",
		kafka.TopicAttemptSaved, kafka.TopicDanceUploaded, kafka.TopicLikeToggled, kafka.TopicDuelCompleted)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("activity-feed-worker shutting down...")
	cancel()
	wg.Wait()
	log.Println("activity-feed-worker stopped")
}

func initDB(ctx context.Context) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(config.DSNFromEnv())
	if err != nil {
		return nil, err
	}
	return pgxpool.ConnectConfig(ctx, poolCfg)
}
