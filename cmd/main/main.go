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
	"strconv"
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
	grpccreds "google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	authGen "DDDance/internal/pkg/auth/delivery/grpc/gen"
	authHandlers "DDDance/internal/pkg/auth/delivery/http"
	authRepo "DDDance/internal/pkg/auth/repo"
	authUsecase "DDDance/internal/pkg/auth/usecase"
	"DDDance/internal/pkg/comparison"
	comparisonHttp "DDDance/internal/pkg/comparison/delivery/http"
	comparisonUsecase "DDDance/internal/pkg/comparison/usecase"
	danceHttp "DDDance/internal/pkg/dance/delivery/http"
	danceUsecase "DDDance/internal/pkg/dance/usecase"
	"DDDance/internal/pkg/kafka"
	_ "DDDance/internal/pkg/metrics"
	notificationsClient "DDDance/internal/pkg/notifications/client"
	profileHttp "DDDance/internal/pkg/profile/delivery/http"
	recommendClient "DDDance/internal/pkg/recommendation/client"
	recommendHttp "DDDance/internal/pkg/recommendation/delivery/http"
	recommendUsecase "DDDance/internal/pkg/recommendation/usecase"
	"strings"

	"DDDance/internal/pkg/events"
	"DDDance/internal/pkg/middleware/cors"
	logger "DDDance/internal/pkg/middleware/logger"
	metricsMiddleware "DDDance/internal/pkg/middleware/metrics"
	"DDDance/internal/pkg/middleware/ratelimit"
	profileUsecase "DDDance/internal/pkg/profile/usecase"
	socialHttp "DDDance/internal/pkg/social/delivery/http"
	socialUsecase "DDDance/internal/pkg/social/usecase"
	"DDDance/internal/pkg/users"
	userHandlers "DDDance/internal/pkg/users/delivery/http"
	userRepo "DDDance/internal/pkg/users/repo/pg"
	redisRepo "DDDance/internal/pkg/users/repo/redis"
	storageRepo "DDDance/internal/pkg/users/repo/s3"
	userUsecase "DDDance/internal/pkg/users/usecase"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func runUserVideoCleanupWorker(ctx context.Context, usersUC comparison.ComparisonUsecase) {
	ttlHours := 24
	if v := os.Getenv("USER_VIDEO_TTL_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ttlHours = n
		}
	}
	periodMinutes := 60
	if v := os.Getenv("USER_VIDEO_CLEANUP_PERIOD_MINUTES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			periodMinutes = n
		}
	}
	ttl := time.Duration(ttlHours) * time.Hour
	period := time.Duration(periodMinutes) * time.Minute

	ticker := time.NewTicker(period)
	defer ticker.Stop()

	doPass := func() {
		passCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		if n, err := usersUC.CleanupExpiredUserVideos(passCtx, ttl); err != nil {
			log.Printf("user-video cleanup pass failed: %v", err)
		} else if n > 0 {
			log.Printf("user-video cleanup deleted %d expired files", n)
		}
	}

	time.Sleep(5 * time.Minute)
	doPass()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			doPass()
		}
	}
}

func runDuelExpireJob(ctx context.Context, usersUC users.UsersUsecase) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			func() {
				jobCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				defer cancel()
				if err := usersUC.RunExpireJob(jobCtx); err != nil {
					log.Printf("duel expire job failed: %v", err)
				}
			}()
		}
	}
}

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

	httpTransport := &http.Transport{
		TLSNextProto: make(map[string]func(string, *tls.Conn) http.RoundTripper),
	}
	if os.Getenv("S3_INSECURE_TLS") == "true" {
		log.Println("WARNING: проверка TLS-сертификата S3 отключена (S3_INSECURE_TLS=true)")
		httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	customHTTPClient := &http.Client{Transport: httpTransport}

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

	if os.Getenv("JWT_SECRET") == "" {
		log.Fatal("JWT_SECRET is not set")
	}

	dbpool, err := initDB(ctx)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v\n", err)
	}
	defer dbpool.Close()

	s3Client, s3Bucket, err := initS3Client(ctx)
	if err != nil {
		log.Fatalf("Unable to connect to S3: %v\n", err)
	}

	authHost := os.Getenv("AUTH_SERVICE_HOST")
	if authHost == "" {
		authHost = "auth"
	}
	authPort := os.Getenv("AUTH_SERVICE_PORT")
	if authPort == "" {
		authPort = "5459"
	}

	var grpcCreds grpccreds.TransportCredentials
	if caFile := os.Getenv("GRPC_TLS_CA"); caFile != "" {
		tlsCreds, err := grpccreds.NewClientTLSFromFile(caFile, "")
		if err != nil {
			log.Fatalf("failed to load gRPC CA cert: %v", err)
		}
		grpcCreds = tlsCreds
		log.Println("gRPC client: TLS enabled")
	} else {
		grpcCreds = insecure.NewCredentials()
		log.Println("WARNING: gRPC client connecting WITHOUT TLS (GRPC_TLS_CA not set)")
	}
	authConn, err := grpc.Dial(
		fmt.Sprintf("%s:%s", authHost, authPort),
		grpc.WithTransportCredentials(grpcCreds),
	)
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

	danceUC := danceUsecase.NewDanceUsecase(userPgRepo, userS3Repo)

	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		viewCache := redisRepo.NewViewCache(redisAddr)
		danceUC.SetViewCache(viewCache)
		danceUC.SetMLLock(redisRepo.NewMLLock(redisAddr))
		usersUC.SetBotNotifier(redisRepo.NewBotNotifier(redisAddr))
		log.Println("Redis view cache, ML lock, and bot notifier enabled:", redisAddr)
	}

	if notifHost := os.Getenv("NOTIFICATIONS_HOST"); notifHost != "" {
		notifPort := os.Getenv("NOTIFICATIONS_PORT")
		if notifPort == "" {
			notifPort = "5460"
		}
		notifConn, notifErr := grpc.Dial(
			fmt.Sprintf("%s:%s", notifHost, notifPort),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		if notifErr != nil {
			log.Printf("WARNING: could not connect to notifications service: %v (falling back to direct DB)", notifErr)
		} else {
			defer notifConn.Close()
			usersUC.SetNotificationSender(notificationsClient.NewNotificationsGRPCClient(notifConn))
			log.Printf("Notifications gRPC client connected: %s:%s", notifHost, notifPort)
		}
	}

	comparisonUC := comparisonUsecase.NewComparisonUsecase(userPgRepo, userS3Repo)
	comparisonUC.SetAchievementTrigger(usersUC)
	comparisonUC.SetDuelSubmitter(usersUC)
	comparisonUC.SetUploadFinalizer(danceUC)
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		comparisonUC.SetTop10Cache(redisRepo.NewTop10Cache(redisAddr))
		comparisonUC.SetBotNotifier(redisRepo.NewBotNotifier(redisAddr))
		comparisonUC.SetLeaderboard(redisRepo.NewLeaderboard(redisAddr))
		comparisonUC.SetSSEPublisher(redisRepo.NewSSEPublisher(redisAddr))
	}

	profileUC := profileUsecase.NewProfileUsecase(userPgRepo, userS3Repo)
	profileUC.SetTokenGenerator(usersUC)
	profileHandler := profileHttp.NewProfileHandler(profileUC)

	socialUC := socialUsecase.NewSocialUsecase(userPgRepo)
	socialUC.SetAchievementTrigger(usersUC)
	danceUC.SetAchievementTrigger(usersUC)

	if kafkaBrokers := os.Getenv("KAFKA_BROKERS"); kafkaBrokers != "" {
		kp := kafka.NewKafkaProducer(strings.Split(kafkaBrokers, ","))
		usersUC.SetKafkaProducer(kp)
		danceUC.SetKafkaProducer(kp)
		comparisonUC.SetKafkaProducer(kp)
		socialUC.SetKafkaProducer(kp)
		log.Printf("Kafka producer wired: brokers=%s", kafkaBrokers)
	}

	mlServiceURL := os.Getenv("ML_SERVICE_URL")
	mlClient := recommendClient.NewMLClient(mlServiceURL)
	recommendUC := recommendUsecase.NewRecommendationUsecase(userPgRepo, mlClient)
	danceUC.SetCacheInvalidator(recommendUC)
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		recommendUC.SetReelsCache(redisRepo.NewReelsCache(redisAddr))
	}
	recommendHandler := recommendHttp.NewRecommendHandler(recommendUC)

	authHandler := authHandlers.NewAuthHandler(authClient, authUsecase)
	userHandler := userHandlers.NewUserHandler(authClient, usersUC)
	userHandler.SetComparisonUsecase(comparisonUC)
	danceHandler := danceHttp.NewDanceHandler(danceUC)
	comparisonHandler := comparisonHttp.NewComparisonHandler(comparisonUC)
	socialHandler := socialHttp.NewSocialHandler(socialUC)

	var eventsHandler *events.Handler
	var botUploadRL, createDuelRL func(http.Handler) http.Handler
	if redisAddr := os.Getenv("REDIS_ADDR"); redisAddr != "" {
		eventsHandler = events.NewHandler(redisAddr)
		botUploadRL = ratelimit.NewRedisRateLimiter(redisAddr, 5, time.Hour, "bot_upload").Middleware
		createDuelRL = ratelimit.NewRedisRateLimiter(redisAddr, 10, time.Minute, "create_duel").Middleware
	}

	ddLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	metricsPort := os.Getenv("METRICS_PORT")
	if metricsPort == "" {
		metricsPort = "9091"
	}
	metricsServer := &http.Server{
		Addr:    ":" + metricsPort,
		Handler: promhttp.Handler(),
	}
	go func() {
		log.Printf("metrics server listening on :%s", metricsPort)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server error: %v", err)
		}
	}()

	mainRouter := mux.NewRouter()
	mainRouter.Use(metricsMiddleware.MetricsMiddleware)
	mainRouter.PathPrefix("/swagger/").Handler(httpSwagger.WrapHandler)

	apiRouter := mainRouter.PathPrefix("/api").Subrouter()
	apiRouter.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "I am not giving any dances!", http.StatusTeapot)
	})

	apiRouter.Use(cors.CorsMiddleware)
	apiRouter.Use(logger.LoggerMiddleware(ddLogger))

	authRouter := apiRouter.PathPrefix("/auth").Subrouter()
	signupRL := ratelimit.NewMiddleware(5.0/60, 5)   // 5 req/min per IP
	signinRL := ratelimit.NewMiddleware(10.0/60, 10) // 10 req/min per IP
	authRouter.Handle("/signup", signupRL(http.HandlerFunc(authHandler.SignupUser))).Methods(http.MethodPost, http.MethodOptions)
	authRouter.Handle("/signin", signinRL(http.HandlerFunc(authHandler.SignInUser))).Methods(http.MethodPost, http.MethodOptions)
	authRouter.HandleFunc("/vk", authHandler.VKAuth).Methods(http.MethodPost, http.MethodOptions)

	protectedAuthRouter := authRouter.PathPrefix("").Subrouter()
	protectedAuthRouter.Use(authHandler.Middleware)
	protectedAuthRouter.HandleFunc("/check", authHandler.CheckAuth).Methods(http.MethodGet, http.MethodOptions)
	protectedAuthRouter.HandleFunc("/logout", authHandler.LogOutUser).Methods(http.MethodPost, http.MethodOptions)

	userRouter := apiRouter.PathPrefix("/users").Subrouter()
	userRouter.Handle("/load", userHandler.OptionalAuthMiddleware(http.HandlerFunc(danceHandler.LoadDance))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/loadByURL", userHandler.OptionalAuthMiddleware(http.HandlerFunc(danceHandler.LoadDanceByURL))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/load/trim", userHandler.OptionalAuthMiddleware(http.HandlerFunc(danceHandler.TrimAndLoadDance))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/dance/compare-upload", userHandler.OptionalAuthMiddleware(http.HandlerFunc(comparisonHandler.CompareDanceWithFile))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/task/{task_id}/status", userHandler.OptionalAuthMiddleware(http.HandlerFunc(comparisonHandler.GetTaskStatus))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.Handle("/dance/rate", userHandler.OptionalAuthMiddleware(http.HandlerFunc(socialHandler.GetRating))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.Handle("/main_page", userHandler.OptionalAuthMiddleware(http.HandlerFunc(danceHandler.GetMainPage))).Methods(http.MethodGet, http.MethodOptions)

	userRouter.Handle("/dance/{id}", userHandler.OptionalAuthMiddleware(http.HandlerFunc(danceHandler.GetDanceByID))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.HandleFunc("/dance/{dance_id}/segment/{segment_idx}", danceHandler.GetSegmentDescription).Methods(http.MethodGet)
	userRouter.Handle("/search", userHandler.OptionalAuthMiddleware(http.HandlerFunc(profileHandler.SearchUsers))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.Handle("/{id}/profile", userHandler.OptionalAuthMiddleware(http.HandlerFunc(profileHandler.GetPublicProfile))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.HandleFunc("/{id}/activity", profileHandler.GetUserActivity).Methods(http.MethodGet, http.MethodOptions)

	protectedUserRouter := userRouter.PathPrefix("").Subrouter()
	protectedUserRouter.Use(userHandler.Middleware)
	protectedUserRouter.HandleFunc("/change/password", userHandler.ChangePassword).Methods(http.MethodPut, http.MethodOptions)
	protectedUserRouter.HandleFunc("/profile", profileHandler.UpdateProfile).Methods(http.MethodPut, http.MethodOptions)
	protectedUserRouter.HandleFunc("/history", profileHandler.GetSearchHistory).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/history/{history_id}", profileHandler.DeleteFromHistory).Methods(http.MethodDelete, http.MethodOptions)
	protectedUserRouter.HandleFunc("/history/{history_id}", profileHandler.UpdateHistoryName).Methods(http.MethodPut, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{id}/like", socialHandler.ToggleLike).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/likes", socialHandler.GetUserLikedDances).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/rate", socialHandler.SaveRating).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{user_dance_id}/save", comparisonHandler.SaveAttempt).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{user_dance_id}/save", comparisonHandler.UnsaveAttempt).Methods(http.MethodDelete, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{user_dance_id}/result", comparisonHandler.GetCompareResult).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/saved-dances", comparisonHandler.GetSavedAttempts).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/attempts", comparisonHandler.GetUserAttempts).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/me/most-improved", profileHandler.GetMostImprovedDance).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/me/feed", socialHandler.GetFriendsFeed).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/me/weak-spots", comparisonHandler.GetWeakSpots).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/me/telegram-code", userHandler.GetTelegramLinkCode).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/creator-stats", profileHandler.GetCreatorAnalytics).Methods(http.MethodGet, http.MethodOptions)
	if eventsHandler != nil {
		userRouter.Handle("/me/events", userHandler.JWTMiddleware(http.HandlerFunc(eventsHandler.ServeSSE))).Methods(http.MethodGet)
	}

	protectedUserRouter.HandleFunc("/uploaded-dances", danceHandler.GetUploadedDances).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{dance_id}/name", danceHandler.SetDanceName).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{dance_id}/publish", danceHandler.PublishDance).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{dance_id}/unpublish", danceHandler.UnpublishDance).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{dance_id}", danceHandler.DeleteDance).Methods(http.MethodDelete, http.MethodOptions)

	protectedUserRouter.HandleFunc("/{id}/friend-request", profileHandler.SendFriendRequest).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/friend-requests/{id}/respond", profileHandler.RespondFriendRequest).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/friends", profileHandler.GetFriends).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/friends/{friend_id}", profileHandler.RemoveFriend).Methods(http.MethodDelete, http.MethodOptions)
	userRouter.Handle("/{id}/friends", userHandler.OptionalAuthMiddleware(http.HandlerFunc(profileHandler.GetPublicFriends))).Methods(http.MethodGet, http.MethodOptions)

	apiRouter.HandleFunc("/dances", danceHandler.GetDanceCatalog).Methods(http.MethodGet, http.MethodOptions)
	apiRouter.HandleFunc("/dances/trending", danceHandler.GetDanceTrending).Methods(http.MethodGet, http.MethodOptions)

	danceRouter := apiRouter.PathPrefix("/dances").Subrouter()
	danceRouter.HandleFunc("/{dance_id}/users/{user_id}/timeline", danceHandler.GetDanceTimeline).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.HandleFunc("/{dance_id}/keyframes", danceHandler.GetDanceKeyframes).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.HandleFunc("/{id}/stats", danceHandler.GetDanceStatsHandler).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.HandleFunc("/{id}/status", danceHandler.GetDanceModerationStatus).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.Handle("/{id}/leaderboard", userHandler.OptionalAuthMiddleware(http.HandlerFunc(comparisonHandler.GetLeaderboard))).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.Handle("/{id}/view", userHandler.OptionalAuthMiddleware(http.HandlerFunc(danceHandler.RecordDanceView))).Methods(http.MethodPost, http.MethodOptions)
	danceRouter.HandleFunc("/{dance_id}/segments/descriptions", danceHandler.GetDanceSegmentDescriptions).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.HandleFunc("/{dance_id}/duration", danceHandler.PatchDanceDuration).Methods(http.MethodPatch, http.MethodOptions)

	protectedDanceRouter := danceRouter.PathPrefix("").Subrouter()
	protectedDanceRouter.Use(userHandler.Middleware)
	protectedDanceRouter.HandleFunc("/{dance_id}/segments/{segment_index}/description", danceHandler.UpdateSegmentDescription).Methods(http.MethodPut, http.MethodOptions)
	protectedDanceRouter.HandleFunc("/{dance_id}/my-progress", comparisonHandler.GetMyDanceProgress).Methods(http.MethodGet, http.MethodOptions)
	protectedDanceRouter.HandleFunc("/{dance_id}/friends-scores", comparisonHandler.GetFriendsDanceScores).Methods(http.MethodGet, http.MethodOptions)

	adminRouter := apiRouter.PathPrefix("/admin").Subrouter()
	adminRouter.Use(userHandlers.AdminTokenMiddleware)
	adminRouter.HandleFunc("/dances/{id}/status", danceHandler.UpdateDanceStatus).Methods(http.MethodPatch, http.MethodOptions)

	notificationsRouter := apiRouter.PathPrefix("/notifications").Subrouter()
	notificationsRouter.Use(userHandler.Middleware)
	notificationsRouter.HandleFunc("", socialHandler.GetNotifications).Methods(http.MethodGet, http.MethodOptions)
	notificationsRouter.HandleFunc("/{id}/read", socialHandler.MarkNotificationRead).Methods(http.MethodPost, http.MethodOptions)
	notificationsRouter.HandleFunc("/read-all", socialHandler.MarkAllNotificationsRead).Methods(http.MethodPost, http.MethodOptions)
	notificationsRouter.HandleFunc("", socialHandler.ClearNotifications).Methods(http.MethodDelete, http.MethodOptions)

	uploadsRouter := apiRouter.PathPrefix("/uploads").Subrouter()
	uploadsRouter.Use(userHandler.Middleware)
	uploadsRouter.HandleFunc("/claim", danceHandler.ClaimUploads).Methods(http.MethodPost, http.MethodOptions)

	apiRouter.HandleFunc("/achievements", userHandler.GetAllAchievements).Methods(http.MethodGet, http.MethodOptions)
	userRouter.HandleFunc("/{user_id}/achievements", userHandler.GetUserAchievements).Methods(http.MethodGet, http.MethodOptions)

	topRouter := apiRouter.PathPrefix("/top").Subrouter()
	topRouter.HandleFunc("/dancers", comparisonHandler.GetTopDancers).Methods(http.MethodGet, http.MethodOptions)
	topRouter.HandleFunc("/dances", danceHandler.GetTopDances).Methods(http.MethodGet, http.MethodOptions)

	apiRouter.HandleFunc("/recommend", recommendHandler.Recommend).Methods(http.MethodPost, http.MethodOptions)
	apiRouter.HandleFunc("/recommend/similar", recommendHandler.GetSimilarDances).Methods(http.MethodGet, http.MethodOptions)
	apiRouter.Handle("/reels", userHandler.OptionalAuthMiddleware(http.HandlerFunc(recommendHandler.GetReelsFeed))).Methods(http.MethodGet, http.MethodOptions)
	apiRouter.HandleFunc("/reels/attempts", comparisonHandler.GetReelsAttempts).Methods(http.MethodGet, http.MethodOptions)

	apiRouter.HandleFunc("/duels/public", userHandler.GetPublicDuels).Methods(http.MethodGet, http.MethodOptions)

	duelsRouter := apiRouter.PathPrefix("/duels").Subrouter()
	duelsRouter.Use(userHandler.Middleware)
	createDuelHandler := http.HandlerFunc(userHandler.CreateDuel)
	if createDuelRL != nil {
		duelsRouter.Handle("", createDuelRL(createDuelHandler)).Methods(http.MethodPost, http.MethodOptions)
	} else {
		duelsRouter.Handle("", createDuelHandler).Methods(http.MethodPost, http.MethodOptions)
	}
	duelsRouter.HandleFunc("", userHandler.GetDuelHistory).Methods(http.MethodGet, http.MethodOptions)
	duelsRouter.HandleFunc("/stats", userHandler.GetDuelStats).Methods(http.MethodGet, http.MethodOptions)
	duelsRouter.HandleFunc("/active", userHandler.GetActiveDuelsForDance).Methods(http.MethodGet, http.MethodOptions)
	duelsRouter.HandleFunc("/submit", userHandler.SubmitAttemptToDuels).Methods(http.MethodPost, http.MethodOptions)
	duelsRouter.HandleFunc("/{duel_id}/accept", userHandler.AcceptDuel).Methods(http.MethodPost, http.MethodOptions)
	duelsRouter.HandleFunc("/{duel_id}/decline", userHandler.DeclineDuel).Methods(http.MethodPost, http.MethodOptions)
	duelsRouter.HandleFunc("/{duel_id}", userHandler.GetDuelByID).Methods(http.MethodGet, http.MethodOptions)

	botRouter := apiRouter.PathPrefix("/bot").Subrouter()
	botRouter.Use(userHandlers.BotSecretMiddleware)
	botRouter.HandleFunc("/link", userHandler.BotLink).Methods(http.MethodPost, http.MethodOptions)
	botUploadHandler := http.HandlerFunc(userHandler.BotUpload)
	if botUploadRL != nil {
		botRouter.Handle("/upload", botUploadRL(botUploadHandler)).Methods(http.MethodPost, http.MethodOptions)
	} else {
		botRouter.Handle("/upload", botUploadHandler).Methods(http.MethodPost, http.MethodOptions)
	}
	botRouter.HandleFunc("/notify", userHandler.BotNotify).Methods(http.MethodPost, http.MethodOptions)
	botRouter.HandleFunc("/duels", userHandler.BotGetDuels).Methods(http.MethodGet, http.MethodOptions)
	botRouter.HandleFunc("/duels/{duel_id}/accept", userHandler.BotAcceptDuel).Methods(http.MethodPost, http.MethodOptions)
	botRouter.HandleFunc("/duels/{duel_id}/decline", userHandler.BotDeclineDuel).Methods(http.MethodPost, http.MethodOptions)
	botRouter.HandleFunc("/task/{task_id}/status", userHandler.BotTaskStatus).Methods(http.MethodGet, http.MethodOptions)
	botRouter.HandleFunc("/attempts", userHandler.BotGetAttempts).Methods(http.MethodGet, http.MethodOptions)
	botRouter.HandleFunc("/stats", userHandler.BotGetStats).Methods(http.MethodGet, http.MethodOptions)
	botRouter.HandleFunc("/achievements", userHandler.BotGetAchievements).Methods(http.MethodGet, http.MethodOptions)

	danceSrv := http.Server{
		Handler:           mainRouter,
		Addr:              ":5458",
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Println("Starting main server on port 5458!")
		err := danceSrv.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Printf("Server start error: %v", err)
			os.Exit(1)
		}
	}()

	cleanupCtx, cleanupCancel := context.WithCancel(context.Background())
	defer cleanupCancel()
	go runUserVideoCleanupWorker(cleanupCtx, comparisonUC)
	go runDuelExpireJob(cleanupCtx, usersUC)

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
