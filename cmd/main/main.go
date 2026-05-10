// @title           DDDance API
// @version         1.0
// @description     API для авторизации пользователей и получения разбора танца.
// @host            localhost:5458
// @BasePath        /api
package main

import (
	_ "DDDance/docs"
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
	httpSwagger "github.com/swaggo/http-swagger"
	"google.golang.org/grpc"

	authGen "DDDance/internal/pkg/auth/delivery/grpc/gen"
	authHandlers "DDDance/internal/pkg/auth/delivery/http"
	authRepo "DDDance/internal/pkg/auth/repo"
	authUsecase "DDDance/internal/pkg/auth/usecase"
	"DDDance/internal/pkg/middleware/cors"
	logger "DDDance/internal/pkg/middleware/logger"
	userHandlers "DDDance/internal/pkg/users/delivery/http"
	userRepo "DDDance/internal/pkg/users/repo/pg"
	storageRepo "DDDance/internal/pkg/users/repo/s3"
	userUsecase "DDDance/internal/pkg/users/usecase"
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

func initS3Client(ctx context.Context) (*s3.Client, string, error) {
	endpoint := os.Getenv("AWS_S3_ENDPOINT")
	bucket := os.Getenv("AWS_S3_BUCKET")
	accessKey := os.Getenv("AWS_ACCESS_KEY_ID")
	secretKey := os.Getenv("AWS_SECRET_ACCESS_KEY")

	region := "ru-7"

	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID && endpoint != "" {
			return aws.Endpoint{
				URL:           endpoint,
				SigningRegion: region,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	customHTTPClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		awsconfig.WithEndpointResolverWithOptions(customResolver),
		awsconfig.WithHTTPClient(customHTTPClient),
	)
	if err != nil {
		return nil, "", err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	return client, bucket, nil
}

func main() {
	_ = godotenv.Load()
	ctx := context.Background()

	dbpool, err := initDB(ctx)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer dbpool.Close()

	s3Client, s3Bucket, err := initS3Client(ctx)
	if err != nil {
		log.Printf("Warning: Unable to connect to S3: %v\n", err)
	}

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

	userPgRepo := userRepo.NewUserRepository(dbpool)
	userS3Repo := storageRepo.NewS3Repository(s3Client, s3Bucket)
	usersUC := userUsecase.NewUserUsecase(userPgRepo, userS3Repo)

	authHandler := authHandlers.NewAuthHandler(authClient, authUsecase)
	userHandler := userHandlers.NewUserHandler(authClient, usersUC)

	ddLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	mainRouter := mux.NewRouter()
	mainRouter.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

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
	authRouter.HandleFunc("/vk", authHandler.VKAuth).Methods(http.MethodPost, http.MethodOptions)

	protectedAuthRouter := authRouter.PathPrefix("").Subrouter()
	protectedAuthRouter.Use(authHandler.Middleware)
	protectedAuthRouter.HandleFunc("/check", authHandler.CheckAuth).Methods(http.MethodGet, http.MethodOptions)
	protectedAuthRouter.HandleFunc("/logout", authHandler.LogOutUser).Methods(http.MethodPost, http.MethodOptions)

	userRouter := apiRouter.PathPrefix("/users").Subrouter()
	userRouter.Handle("/load", userHandler.OptionalAuthMiddleware(http.HandlerFunc(userHandler.LoadDance))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/loadByURL", userHandler.OptionalAuthMiddleware(http.HandlerFunc(userHandler.LoadDanceByURL))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/load/trim", userHandler.OptionalAuthMiddleware(http.HandlerFunc(userHandler.TrimAndLoadDance))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/dance/compare-upload", userHandler.OptionalAuthMiddleware(http.HandlerFunc(userHandler.CompareDanceWithFile))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/dance/rate", userHandler.OptionalAuthMiddleware(http.HandlerFunc(userHandler.GetRating))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.HandleFunc("/main_page", userHandler.GetMainPage).Methods(http.MethodGet, http.MethodOptions)

	userRouter.Handle("/dance/{id}", userHandler.OptionalAuthMiddleware(http.HandlerFunc(userHandler.GetDanceByID))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.HandleFunc("/dance/{dance_id}/segment/{segment_idx}", userHandler.GetSegmentDescription).Methods(http.MethodGet)
	

	protectedUserRouter := userRouter.PathPrefix("").Subrouter()
	protectedUserRouter.Use(userHandler.Middleware)
	protectedUserRouter.HandleFunc("/change/password", userHandler.ChangePassword).Methods(http.MethodPut, http.MethodOptions)
	protectedUserRouter.HandleFunc("/history", userHandler.GetSearchHistory).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/history/{history_id}", userHandler.DeleteFromHistory).Methods(http.MethodDelete, http.MethodOptions)
	protectedUserRouter.HandleFunc("/history/{history_id}", userHandler.UpdateHistoryName).Methods(http.MethodPut, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{id}/like", userHandler.ToggleLike).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/likes", userHandler.GetUserLikedDances).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/rate", userHandler.SaveRating).Methods(http.MethodPost, http.MethodOptions)
	

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
