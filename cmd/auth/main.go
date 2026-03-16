// @title           DDDance API
// @version         1.0
// @description     API для авторизации пользователей
// @host            localhost:5458
// @BasePath        /api
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"

	authHandler "DDDance/internal/pkg/auth/delivery/grpc"
	authRepo "DDDance/internal/pkg/auth/repo"
	authUsecase "DDDance/internal/pkg/auth/usecase"
	"DDDance/internal/pkg/middleware/logger"
	userRepo "DDDance/internal/pkg/users/repo"
	userUsecase "DDDance/internal/pkg/users/usecase"

	"DDDance/internal/pkg/auth/delivery/grpc/gen"
)

func initDB(ctx context.Context) (*pgxpool.Pool, error) {
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASS")
	dbname := os.Getenv("DB_NAME")

	postgresString := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname,
	)

	config, err := pgxpool.ParseConfig(postgresString)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	return pool, nil
}

func main() {
	_ = godotenv.Load()
	ctx := context.Background()

	dbpool, err := initDB(ctx)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer dbpool.Close()

	authRepo := authRepo.NewAuthRepository(dbpool)
	authUsecase := authUsecase.NewAuthUsecase(authRepo)
	userRepo := userRepo.NewUserRepository(dbpool)
	userUsecase := userUsecase.NewUserUsecase(userRepo)

	// инициализация gRPC хендлера
	authHandler := authHandler.NewGrpcAuthHandler(authUsecase, userUsecase)

	ddLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	gRPCServer := grpc.NewServer(grpc.ChainUnaryInterceptor(logger.LoggerInterceptor(ddLogger)))
	gen.RegisterAuthServer(gRPCServer, authHandler)

	r := mux.NewRouter().PathPrefix("").Subrouter()
	http.Handle("/", r)
	httpSrv := http.Server{Handler: r, Addr: ":5460"}
	//запуск мониторинга
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil {
			fmt.Println(err)
		}
	}()

	go func() {
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", 5459))
		if err != nil {
			fmt.Println(err)
		}
		if err := gRPCServer.Serve(listener); err != nil {
			fmt.Println(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGTERM, syscall.SIGINT)
	<-stop

	log.Println("Shutting down auth gRPC server...")
	gRPCServer.GracefulStop()
	log.Println("Auth server exited")
}
