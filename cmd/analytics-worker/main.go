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

	"DDDance/internal/pkg/kafka"
)

type viewEvent struct {
	DanceID  string `json:"dance_id"`
	ViewerID string `json:"viewer_id"`
}

// DECISION: track viewer_id so ON CONFLICT DO NOTHING in dance_views still deduplicates correctly.
type viewAccumulator struct {
	mu     sync.Mutex
	events map[string]map[string]bool // dance_id → set of viewer_ids
}

func newViewAccumulator() *viewAccumulator {
	return &viewAccumulator{events: make(map[string]map[string]bool)}
}

func (a *viewAccumulator) add(danceID, viewerID string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.events[danceID] == nil {
		a.events[danceID] = make(map[string]bool)
	}
	a.events[danceID][viewerID] = true
}

func (a *viewAccumulator) drain() map[string]map[string]bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	old := a.events
	a.events = make(map[string]map[string]bool)
	return old
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
		log.Fatalf("analytics-worker: db connect failed: %v", err)
	}
	defer dbpool.Close()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	consumer := kafka.NewKafkaConsumer(brokers, logger)
	acc := newViewAccumulator()

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		err := consumer.Subscribe(ctx, kafka.TopicDanceViewed, "analytics-worker-views", func(msgCtx context.Context, topic, key string, value []byte) error {
			var ev viewEvent
			if err := json.Unmarshal(value, &ev); err != nil || ev.DanceID == "" || ev.ViewerID == "" {
				return nil
			}
			acc.add(ev.DanceID, ev.ViewerID)
			return nil
		})
		if err != nil {
			logger.Error("analytics-worker kafka consumer exited", "error", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		flushLoop(ctx, dbpool, acc, logger)
	}()

	log.Printf("analytics-worker started, consuming %s", kafka.TopicDanceViewed)

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("analytics-worker shutting down...")
	cancel()
	wg.Wait()
	flushViews(context.Background(), dbpool, acc.drain(), logger)
	log.Println("analytics-worker stopped")
}

func flushLoop(ctx context.Context, db *pgxpool.Pool, acc *viewAccumulator, logger *slog.Logger) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			flushViews(ctx, db, acc.drain(), logger)
		}
	}
}

func flushViews(ctx context.Context, db *pgxpool.Pool, events map[string]map[string]bool, logger *slog.Logger) {
	total := 0
	for _, viewers := range events {
		total += len(viewers)
	}
	if total == 0 {
		return
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		logger.Error("analytics-worker: begin tx failed", "error", err)
		return
	}
	defer tx.Rollback(ctx)

	inserted := 0
	for danceID, viewers := range events {
		for viewerID := range viewers {
			_, err := tx.Exec(ctx,
				`INSERT INTO dance_views (dance_id, viewer_id) VALUES ($1, $2) ON CONFLICT (dance_id, viewer_id) DO NOTHING`,
				danceID, viewerID,
			)
			if err != nil {
				logger.Warn("analytics-worker: insert dance_view failed", "dance_id", danceID, "error", err)
			} else {
				inserted++
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		logger.Error("analytics-worker: commit failed", "error", err)
		return
	}
	logger.Info("analytics-worker: flushed views", "inserted", inserted, "total_events", total)
}

func initDB(ctx context.Context) (*pgxpool.Pool, error) {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASS")
	dbname := os.Getenv("DB_NAME")

	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname,
	)
	config, err := pgxpool.ParseConfig(connStr)
	if err != nil {
		return nil, err
	}
	return pgxpool.ConnectConfig(ctx, config)
}
