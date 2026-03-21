// @title           Kinopoisk API
// @version         1.0
// @description     API для авторизации пользователей и получения фильмов/жанров/актеров.
// @host            localhost:5458
// @BasePath        /api
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	authHandlers "DDDance/internal/pkg/auth/delivery/http"
	authRepo "DDDance/internal/pkg/auth/repo"
	authUsecase "DDDance/internal/pkg/auth/usecase"
	"DDDance/internal/pkg/middleware/cors"
	logger "DDDance/internal/pkg/middleware/logger"
	userHandlers "DDDance/internal/pkg/users/delivery/http"
	"os"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"

	"github.com/gorilla/mux"

	authGen "DDDance/internal/pkg/auth/delivery/grpc/gen"
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

	// Подключение к auth microservice
	authConn, err := grpc.Dial("auth:5459", grpc.WithInsecure())
	if err != nil {
		log.Printf("unable to connect to auth microservice: %v\n", err)
		return
	}
	defer authConn.Close()

	authClient := authGen.NewAuthClient(authConn)
	authRepo := authRepo.NewAuthRepository(dbpool)
	authUsecase := authUsecase.NewAuthUsecase(authRepo)

	authHandler := authHandlers.NewAuthHandler(authClient, authUsecase)
	userHandler := userHandlers.NewUserHandler(authClient)

	ddLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	mainRouter := mux.NewRouter()

	apiRouter := mainRouter.PathPrefix("/api").Subrouter()
	apiRouter.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "I am not giving any dances!", http.StatusTeapot)
	})

	apiRouter.Use(cors.CorsMiddleware)
	apiRouter.Use(logger.LoggerMiddleware(ddLogger))

	// Auth routes
	authRouter := apiRouter.PathPrefix("/auth").Subrouter()
	authRouter.HandleFunc("/signup", authHandler.SignupUser).Methods(http.MethodPost, http.MethodOptions)
	authRouter.HandleFunc("/signin", authHandler.SignInUser).Methods(http.MethodPost, http.MethodOptions)

	protectedAuthRouter := authRouter.PathPrefix("").Subrouter()
	protectedAuthRouter.Use(authHandler.Middleware)
	protectedAuthRouter.HandleFunc("/check", authHandler.CheckAuth).Methods(http.MethodGet, http.MethodOptions)
	protectedAuthRouter.HandleFunc("/logout", authHandler.LogOutUser).Methods(http.MethodPost, http.MethodOptions)

	userRouter := apiRouter.PathPrefix("/users").Subrouter()

	// Protected user routes
	protectedUserRouter := userRouter.PathPrefix("").Subrouter()
	protectedUserRouter.Use(userHandler.Middleware)
	protectedUserRouter.HandleFunc("/change/password", userHandler.ChangePassword).Methods(http.MethodPut, http.MethodOptions)

	userRouter.HandleFunc("/load", userHandler.LoadDance).Methods(http.MethodGet)
	danceSrv := http.Server{
		Handler: mainRouter,
		Addr:    ":5458",
	}

	go func() {
		log.Println("Starting main server on port 5458!")
		err := danceSrv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Printf("Server start error: %v", err)
			os.Exit(1)
		}
	}()

	quitChannel := make(chan os.Signal, 1)
	signal.Notify(quitChannel, syscall.SIGINT, syscall.SIGTERM)

	<-quitChannel
	log.Printf("Shutting down gracefully...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	err = danceSrv.Shutdown(ctx)
	if err != nil {
		log.Printf("Graceful shutdown failed")
		os.Exit(1)
	}
	log.Printf("Graceful shutdown!!")
}
