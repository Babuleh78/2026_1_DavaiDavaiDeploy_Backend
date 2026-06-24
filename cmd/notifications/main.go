package main

import (
	"DDDance/internal/pkg/config"
	"DDDance/internal/pkg/kafka"
	notificationsGRPC "DDDance/internal/pkg/notifications/delivery/grpc"
	"DDDance/internal/pkg/notifications/delivery/grpc/gen"
	notificationsRepo "DDDance/internal/pkg/notifications/repo/pg"
	notificationsRedis "DDDance/internal/pkg/notifications/repo/redis"
	notificationsUsecase "DDDance/internal/pkg/notifications/usecase"
	"context"
	"encoding/json"
	"log"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
	uuid "github.com/satori/go.uuid"

	"google.golang.org/grpc"
)

func main() {
	_ = godotenv.Load()

	addr := ":5460"
	if v := os.Getenv("NOTIFICATIONS_ADDR"); v != "" {
		addr = v
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dbpool, err := pgxpool.Connect(ctx, config.DSNFromEnv())
	if err != nil {
		log.Fatalf("notifications: unable to connect to database: %v", err)
	}
	defer dbpool.Close()

	pgRepo := notificationsRepo.NewNotificationsRepository(dbpool)
	uc := notificationsUsecase.NewNotificationsUsecase(pgRepo)

	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		uc.SetBotNotifier(notificationsRedis.NewBotNotifier(redisAddr))
		slog.Info("notifications: bot notifier enabled", slog.String("redis", redisAddr))
	}

	if kafkaBrokers := os.Getenv("KAFKA_BROKERS"); kafkaBrokers != "" {
		brokers := strings.Split(kafkaBrokers, ",")
		consumer := kafka.NewKafkaConsumer(brokers, slog.Default())
		go func() {
			err := consumer.Subscribe(ctx, kafka.TopicNotificationSend, "notifications-service",
				func(msgCtx context.Context, topic, key string, value []byte) error {
					var msg struct {
						ToUserID        string         `json:"to_user_id"`
						FromUserID      string         `json:"from_user_id"`
						Type            string         `json:"type"`
						DuelID          string         `json:"duel_id"`
						TelegramPayload map[string]any `json:"telegram_payload"`
					}
					if jsonErr := json.Unmarshal(value, &msg); jsonErr != nil || msg.ToUserID == "" {
						return nil
					}
					toID, parseErr := uuid.FromString(msg.ToUserID)
					if parseErr != nil {
						return nil
					}
					var fromPtr *uuid.UUID
					if msg.FromUserID != "" {
						fromID, fErr := uuid.FromString(msg.FromUserID)
						if fErr == nil {
							fromPtr = &fromID
						}
					}
					if sendErr := uc.Send(msgCtx, toID, fromPtr, msg.Type, msg.DuelID, msg.TelegramPayload); sendErr != nil {
						slog.Warn("notifications kafka: Send failed", "type", msg.Type, "error", sendErr)
					}
					return nil
				},
			)
			if err != nil {
				slog.Error("notifications kafka consumer exited", "error", err)
			}
		}()
		slog.Info("notifications: Kafka consumer started", slog.String("brokers", kafkaBrokers))
	}

	grpcServer := grpc.NewServer()
	gen.RegisterNotificationsServer(grpcServer, notificationsGRPC.NewNotificationsGRPCServer(uc))

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("notifications: listen failed", slog.String("addr", addr), slog.String("err", err.Error()))
		os.Exit(1)
	}

	slog.Info("notifications gRPC server starting", slog.String("addr", addr))

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if serveErr := grpcServer.Serve(lis); serveErr != nil {
			slog.Error("notifications: serve error", slog.String("err", serveErr.Error()))
		}
	}()

	<-quit
	slog.Info("notifications: shutting down")
	cancel()
	grpcServer.GracefulStop()
}
