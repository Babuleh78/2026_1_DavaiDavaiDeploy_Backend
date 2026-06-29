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
	"github.com/prometheus/client_golang/prometheus/promhttp"
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
	"DDDance/internal/pkg/config"
	danceHttp "DDDance/internal/pkg/dance/delivery/http"
	danceUsecase "DDDance/internal/pkg/dance/usecase"
	"DDDance/internal/pkg/events"
	"DDDance/internal/pkg/kafka"
	_ "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/middleware/cors"
	logger "DDDance/internal/pkg/middleware/logger"
	metricsMiddleware "DDDance/internal/pkg/middleware/metrics"
	"DDDance/internal/pkg/middleware/ratelimit"
	notificationsClient "DDDance/internal/pkg/notifications/client"
	profileHttp "DDDance/internal/pkg/profile/delivery/http"
	profileUsecase "DDDance/internal/pkg/profile/usecase"
	recommendClient "DDDance/internal/pkg/recommendation/client"
	recommendHttp "DDDance/internal/pkg/recommendation/delivery/http"
	recommendUsecase "DDDance/internal/pkg/recommendation/usecase"
	socialHttp "DDDance/internal/pkg/social/delivery/http"
	socialUsecase "DDDance/internal/pkg/social/usecase"
	"DDDance/internal/pkg/users"
	userHandlers "DDDance/internal/pkg/users/delivery/http"
	userRepo "DDDance/internal/pkg/users/repo/pg"
	redisRepo "DDDance/internal/pkg/users/repo/redis"
	storageRepo "DDDance/internal/pkg/users/repo/s3"
	userUsecase "DDDance/internal/pkg/users/usecase"
)

func runUserVideoCleanupWorker(ctx context.Context, usersUC comparison.ComparisonUsecase, ttl time.Duration, period time.Duration) {
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

	select {
	case <-ctx.Done():
		return
	case <-time.After(5 * time.Minute):
	}
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

func initDB(ctx context.Context, dbCfg config.DBConfig) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(dbCfg.DSN())
	if err != nil {
		return nil, err
	}
	return pgxpool.ConnectConfig(ctx, poolCfg)
}

func initS3Client(ctx context.Context, s3Cfg config.S3Config) (*s3.Client, string, error) {
	region := "ru-7"

	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID && s3Cfg.Endpoint != "" {
			return aws.Endpoint{
				URL:           s3Cfg.Endpoint,
				SigningRegion: region,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	httpTransport := &http.Transport{
		TLSNextProto: make(map[string]func(string, *tls.Conn) http.RoundTripper),
	}
	if s3Cfg.InsecureTLS {
		log.Println("WARNING: проверка TLS-сертификата S3 отключена (S3_INSECURE_TLS=true)")
		httpTransport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	customHTTPClient := &http.Client{Transport: httpTransport}

	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(s3Cfg.AccessKey, s3Cfg.SecretKey, "")),
		awsconfig.WithEndpointResolverWithOptions(customResolver),
		awsconfig.WithHTTPClient(customHTTPClient),
	)
	if err != nil {
		return nil, "", err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	return client, s3Cfg.Bucket, nil
}

func dialGRPC(target config.GRPCClientConfig, allowInsecure bool, name string) (*grpc.ClientConn, error) {
	var creds grpccreds.TransportCredentials
	switch {
	case target.CAFile != "":
		tlsCreds, err := grpccreds.NewClientTLSFromFile(target.CAFile, "")
		if err != nil {
			return nil, fmt.Errorf("%s: load CA cert: %w", name, err)
		}
		creds = tlsCreds
		log.Printf("gRPC client %s: TLS enabled", name)
	case allowInsecure:
		creds = insecure.NewCredentials()
		log.Printf("WARNING: gRPC client %s connecting WITHOUT TLS (GRPC_ALLOW_INSECURE=true)", name)
	default:
		return nil, fmt.Errorf("%s: no GRPC_TLS_CA set and GRPC_ALLOW_INSECURE is not true — refusing insecure connection", name)
	}
	return grpc.Dial(target.Target(), grpc.WithTransportCredentials(creds))
}

type appHandlers struct {
	auth       *authHandlers.AuthHandler
	user       *userHandlers.UserHandler
	dance      *danceHttp.DanceHandler
	comparison *comparisonHttp.ComparisonHandler
	social     *socialHttp.SocialHandler
	profile    *profileHttp.ProfileHandler
	recommend  *recommendHttp.RecommendHandler
	events     *events.Handler
}

func setupRouter(h appHandlers, ddLogger *slog.Logger, botUploadRL, createDuelRL, botSecretMW, adminMW func(http.Handler) http.Handler) *mux.Router {
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
	authRouter.Handle("/signup", signupRL(http.HandlerFunc(h.auth.SignupUser))).Methods(http.MethodPost, http.MethodOptions)
	authRouter.Handle("/signin", signinRL(http.HandlerFunc(h.auth.SignInUser))).Methods(http.MethodPost, http.MethodOptions)
	authRouter.HandleFunc("/vk", h.auth.VKAuth).Methods(http.MethodPost, http.MethodOptions)

	protectedAuthRouter := authRouter.PathPrefix("").Subrouter()
	protectedAuthRouter.Use(h.auth.Middleware)
	protectedAuthRouter.HandleFunc("/check", h.auth.CheckAuth).Methods(http.MethodGet, http.MethodOptions)
	protectedAuthRouter.HandleFunc("/logout", h.auth.LogOutUser).Methods(http.MethodPost, http.MethodOptions)

	userRouter := apiRouter.PathPrefix("/users").Subrouter()
	userRouter.Handle("/load", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.dance.LoadDance))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/loadByURL", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.dance.LoadDanceByURL))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/load/trim", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.dance.TrimAndLoadDance))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/dance/compare-upload", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.comparison.CompareDanceWithFile))).Methods(http.MethodPost, http.MethodOptions)
	userRouter.Handle("/task/{task_id}/status", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.comparison.GetTaskStatus))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.Handle("/dance/rate", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.social.GetRating))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.Handle("/main_page", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.dance.GetMainPage))).Methods(http.MethodGet, http.MethodOptions)

	userRouter.Handle("/dance/{id}", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.dance.GetDanceByID))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.HandleFunc("/dance/{dance_id}/segment/{segment_idx}", h.dance.GetSegmentDescription).Methods(http.MethodGet)
	userRouter.Handle("/search", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.profile.SearchUsers))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.Handle("/{id}/profile", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.profile.GetPublicProfile))).Methods(http.MethodGet, http.MethodOptions)
	userRouter.HandleFunc("/{id}/activity", h.profile.GetUserActivity).Methods(http.MethodGet, http.MethodOptions)

	protectedUserRouter := userRouter.PathPrefix("").Subrouter()
	protectedUserRouter.Use(h.user.Middleware)
	protectedUserRouter.HandleFunc("/change/password", h.user.ChangePassword).Methods(http.MethodPut, http.MethodOptions)
	protectedUserRouter.HandleFunc("/profile", h.profile.UpdateProfile).Methods(http.MethodPut, http.MethodOptions)
	protectedUserRouter.HandleFunc("/history", h.profile.GetSearchHistory).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/history/{history_id}", h.profile.DeleteFromHistory).Methods(http.MethodDelete, http.MethodOptions)
	protectedUserRouter.HandleFunc("/history/{history_id}", h.profile.UpdateHistoryName).Methods(http.MethodPut, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{id}/like", h.social.ToggleLike).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/likes", h.social.GetUserLikedDances).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/rate", h.social.SaveRating).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{user_dance_id}/save", h.comparison.SaveAttempt).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{user_dance_id}/save", h.comparison.UnsaveAttempt).Methods(http.MethodDelete, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{user_dance_id}/result", h.comparison.GetCompareResult).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/saved-dances", h.comparison.GetSavedAttempts).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/attempts", h.comparison.GetUserAttempts).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/me/most-improved", h.profile.GetMostImprovedDance).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/me/feed", h.social.GetFriendsFeed).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/me/weak-spots", h.comparison.GetWeakSpots).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/me/telegram-code", h.user.GetTelegramLinkCode).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/creator-stats", h.profile.GetCreatorAnalytics).Methods(http.MethodGet, http.MethodOptions)
	if h.events != nil {
		userRouter.Handle("/me/events", h.user.JWTMiddleware(http.HandlerFunc(h.events.ServeSSE))).Methods(http.MethodGet)
	}

	protectedUserRouter.HandleFunc("/uploaded-dances", h.dance.GetUploadedDances).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{dance_id}/name", h.dance.SetDanceName).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{dance_id}/publish", h.dance.PublishDance).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{dance_id}/unpublish", h.dance.UnpublishDance).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/dance/{dance_id}", h.dance.DeleteDance).Methods(http.MethodDelete, http.MethodOptions)

	protectedUserRouter.HandleFunc("/{id}/friend-request", h.profile.SendFriendRequest).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/friend-requests/{id}/respond", h.profile.RespondFriendRequest).Methods(http.MethodPost, http.MethodOptions)
	protectedUserRouter.HandleFunc("/friends", h.profile.GetFriends).Methods(http.MethodGet, http.MethodOptions)
	protectedUserRouter.HandleFunc("/friends/{friend_id}", h.profile.RemoveFriend).Methods(http.MethodDelete, http.MethodOptions)
	userRouter.Handle("/{id}/friends", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.profile.GetPublicFriends))).Methods(http.MethodGet, http.MethodOptions)

	apiRouter.HandleFunc("/dances", h.dance.GetDanceCatalog).Methods(http.MethodGet, http.MethodOptions)
	apiRouter.HandleFunc("/dances/trending", h.dance.GetDanceTrending).Methods(http.MethodGet, http.MethodOptions)

	danceRouter := apiRouter.PathPrefix("/dances").Subrouter()
	danceRouter.HandleFunc("/{dance_id}/users/{user_id}/timeline", h.dance.GetDanceTimeline).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.HandleFunc("/{dance_id}/keyframes", h.dance.GetDanceKeyframes).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.HandleFunc("/{id}/stats", h.dance.GetDanceStatsHandler).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.HandleFunc("/{id}/status", h.dance.GetDanceModerationStatus).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.Handle("/{id}/leaderboard", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.comparison.GetLeaderboard))).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.Handle("/{id}/view", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.dance.RecordDanceView))).Methods(http.MethodPost, http.MethodOptions)
	danceRouter.HandleFunc("/{dance_id}/segments/descriptions", h.dance.GetDanceSegmentDescriptions).Methods(http.MethodGet, http.MethodOptions)
	danceRouter.HandleFunc("/{dance_id}/duration", h.dance.PatchDanceDuration).Methods(http.MethodPatch, http.MethodOptions)

	protectedDanceRouter := danceRouter.PathPrefix("").Subrouter()
	protectedDanceRouter.Use(h.user.Middleware)
	protectedDanceRouter.HandleFunc("/{dance_id}/segments/{segment_index}/description", h.dance.UpdateSegmentDescription).Methods(http.MethodPut, http.MethodOptions)
	protectedDanceRouter.HandleFunc("/{dance_id}/my-progress", h.comparison.GetMyDanceProgress).Methods(http.MethodGet, http.MethodOptions)
	protectedDanceRouter.HandleFunc("/{dance_id}/friends-scores", h.comparison.GetFriendsDanceScores).Methods(http.MethodGet, http.MethodOptions)

	adminRouter := apiRouter.PathPrefix("/admin").Subrouter()
	adminRouter.Use(adminMW)
	adminRouter.HandleFunc("/dances/{id}/status", h.dance.UpdateDanceStatus).Methods(http.MethodPatch, http.MethodOptions)

	notificationsRouter := apiRouter.PathPrefix("/notifications").Subrouter()
	notificationsRouter.Use(h.user.Middleware)
	notificationsRouter.HandleFunc("", h.social.GetNotifications).Methods(http.MethodGet, http.MethodOptions)
	notificationsRouter.HandleFunc("/{id}/read", h.social.MarkNotificationRead).Methods(http.MethodPost, http.MethodOptions)
	notificationsRouter.HandleFunc("/read-all", h.social.MarkAllNotificationsRead).Methods(http.MethodPost, http.MethodOptions)
	notificationsRouter.HandleFunc("", h.social.ClearNotifications).Methods(http.MethodDelete, http.MethodOptions)

	uploadsRouter := apiRouter.PathPrefix("/uploads").Subrouter()
	uploadsRouter.Use(h.user.Middleware)
	uploadsRouter.HandleFunc("/claim", h.dance.ClaimUploads).Methods(http.MethodPost, http.MethodOptions)

	apiRouter.HandleFunc("/achievements", h.user.GetAllAchievements).Methods(http.MethodGet, http.MethodOptions)
	userRouter.HandleFunc("/{user_id}/achievements", h.user.GetUserAchievements).Methods(http.MethodGet, http.MethodOptions)

	topRouter := apiRouter.PathPrefix("/top").Subrouter()
	topRouter.HandleFunc("/dancers", h.comparison.GetTopDancers).Methods(http.MethodGet, http.MethodOptions)
	topRouter.HandleFunc("/dances", h.dance.GetTopDances).Methods(http.MethodGet, http.MethodOptions)

	apiRouter.HandleFunc("/recommend", h.recommend.Recommend).Methods(http.MethodPost, http.MethodOptions)
	apiRouter.HandleFunc("/recommend/similar", h.recommend.GetSimilarDances).Methods(http.MethodGet, http.MethodOptions)
	apiRouter.Handle("/reels", h.user.OptionalAuthMiddleware(http.HandlerFunc(h.recommend.GetReelsFeed))).Methods(http.MethodGet, http.MethodOptions)
	apiRouter.HandleFunc("/reels/attempts", h.comparison.GetReelsAttempts).Methods(http.MethodGet, http.MethodOptions)

	apiRouter.HandleFunc("/duels/public", h.user.GetPublicDuels).Methods(http.MethodGet, http.MethodOptions)

	duelsRouter := apiRouter.PathPrefix("/duels").Subrouter()
	duelsRouter.Use(h.user.Middleware)
	createDuelHandler := http.HandlerFunc(h.user.CreateDuel)
	if createDuelRL != nil {
		duelsRouter.Handle("", createDuelRL(createDuelHandler)).Methods(http.MethodPost, http.MethodOptions)
	} else {
		duelsRouter.Handle("", createDuelHandler).Methods(http.MethodPost, http.MethodOptions)
	}
	duelsRouter.HandleFunc("", h.user.GetDuelHistory).Methods(http.MethodGet, http.MethodOptions)
	duelsRouter.HandleFunc("/stats", h.user.GetDuelStats).Methods(http.MethodGet, http.MethodOptions)
	duelsRouter.HandleFunc("/active", h.user.GetActiveDuelsForDance).Methods(http.MethodGet, http.MethodOptions)
	duelsRouter.HandleFunc("/submit", h.user.SubmitAttemptToDuels).Methods(http.MethodPost, http.MethodOptions)
	duelsRouter.HandleFunc("/{duel_id}/accept", h.user.AcceptDuel).Methods(http.MethodPost, http.MethodOptions)
	duelsRouter.HandleFunc("/{duel_id}/decline", h.user.DeclineDuel).Methods(http.MethodPost, http.MethodOptions)
	duelsRouter.HandleFunc("/{duel_id}", h.user.GetDuelByID).Methods(http.MethodGet, http.MethodOptions)

	botRouter := apiRouter.PathPrefix("/bot").Subrouter()
	botRouter.Use(botSecretMW)
	botRouter.HandleFunc("/link", h.user.BotLink).Methods(http.MethodPost, http.MethodOptions)
	botUploadHandler := http.HandlerFunc(h.user.BotUpload)
	if botUploadRL != nil {
		botRouter.Handle("/upload", botUploadRL(botUploadHandler)).Methods(http.MethodPost, http.MethodOptions)
	} else {
		botRouter.Handle("/upload", botUploadHandler).Methods(http.MethodPost, http.MethodOptions)
	}
	botUploadDanceHandler := http.HandlerFunc(h.user.BotUploadDance)
	if botUploadRL != nil {
		botRouter.Handle("/upload_dance", botUploadRL(botUploadDanceHandler)).Methods(http.MethodPost, http.MethodOptions)
	} else {
		botRouter.Handle("/upload_dance", botUploadDanceHandler).Methods(http.MethodPost, http.MethodOptions)
	}
	botRouter.HandleFunc("/upload_dance/{task_id}/status", h.user.BotUploadDanceStatus).Methods(http.MethodGet, http.MethodOptions)
	botRouter.HandleFunc("/notify", h.user.BotNotify).Methods(http.MethodPost, http.MethodOptions)
	botRouter.HandleFunc("/duels", h.user.BotGetDuels).Methods(http.MethodGet, http.MethodOptions)
	botRouter.HandleFunc("/duels/{duel_id}/accept", h.user.BotAcceptDuel).Methods(http.MethodPost, http.MethodOptions)
	botRouter.HandleFunc("/duels/{duel_id}/decline", h.user.BotDeclineDuel).Methods(http.MethodPost, http.MethodOptions)
	botRouter.HandleFunc("/task/{task_id}/status", h.user.BotTaskStatus).Methods(http.MethodGet, http.MethodOptions)
	botRouter.HandleFunc("/attempts", h.user.BotGetAttempts).Methods(http.MethodGet, http.MethodOptions)
	botRouter.HandleFunc("/stats", h.user.BotGetStats).Methods(http.MethodGet, http.MethodOptions)
	botRouter.HandleFunc("/achievements", h.user.BotGetAchievements).Methods(http.MethodGet, http.MethodOptions)

	return mainRouter
}

func startMetricsServer(port string) *http.Server {
	srv := &http.Server{
		Addr:    ":" + port,
		Handler: promhttp.Handler(),
	}
	go func() {
		log.Printf("metrics server listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server error: %v", err)
		}
	}()
	return srv
}

func main() {
	_ = godotenv.Load()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx := context.Background()

	dbpool, err := initDB(ctx, cfg.DB)
	if err != nil {
		log.Fatalf("Unable to connect to database: %v", err)
	}
	defer dbpool.Close()

	s3Client, s3Bucket, err := initS3Client(ctx, cfg.S3)
	if err != nil {
		log.Fatalf("Unable to connect to S3: %v", err)
	}

	authConn, err := dialGRPC(cfg.AuthGRPC, cfg.AllowInsecureGRPC, "auth")
	if err != nil {
		log.Fatalf("auth microservice: %v", err)
	}
	defer authConn.Close()
	authClient := authGen.NewAuthClient(authConn)

	// --- repositories ---
	userPgRepo := userRepo.NewUserRepository(dbpool)
	userS3Repo := storageRepo.NewS3Repository(s3Client, s3Bucket)

	authUC := authUsecase.NewAuthUsecase(authRepo.NewAuthRepository(dbpool), cfg.JWTSecret)
	usersUC := userUsecase.NewUserUsecase(userPgRepo, userS3Repo, cfg.JWTSecret)

	mlClient := recommendClient.NewMLClient(cfg.MLServiceURL, cfg.MLInternalToken)
	recommendUC := recommendUsecase.NewRecommendationUsecase(userPgRepo, mlClient, cfg.S3Address)
	danceUC := danceUsecase.NewDanceUsecase(userPgRepo, userS3Repo, usersUC, recommendUC, cfg.S3Address, cfg.MLInternalToken)
	comparisonUC := comparisonUsecase.NewComparisonUsecase(userPgRepo, userS3Repo, usersUC, usersUC, danceUC)
	comparisonUC.SetMLInternalToken(cfg.MLInternalToken)
	socialUC := socialUsecase.NewSocialUsecase(userPgRepo, usersUC)
	profileUC := profileUsecase.NewProfileUsecase(userPgRepo, userS3Repo, usersUC)

	if cfg.Redis.Enabled() {
		addr := cfg.Redis.Addr
		danceUC.SetViewCache(redisRepo.NewViewCache(addr))
		danceUC.SetMLLock(redisRepo.NewMLLock(addr))
		usersUC.SetBotNotifier(redisRepo.NewBotNotifier(addr))
		usersUC.SetSSEPublisher(redisRepo.NewSSEPublisher(addr))
		comparisonUC.SetTop10Cache(redisRepo.NewTop10Cache(addr))
		comparisonUC.SetBotNotifier(redisRepo.NewBotNotifier(addr))
		comparisonUC.SetLeaderboard(redisRepo.NewLeaderboard(addr))
		comparisonUC.SetSSEPublisher(redisRepo.NewSSEPublisher(addr))
		recommendUC.SetReelsCache(redisRepo.NewReelsCache(addr))
		log.Println("Redis features enabled:", addr)
	}

	if cfg.NotifGRPC.Enabled() {
		notifConn, notifErr := dialGRPC(cfg.NotifGRPC, cfg.AllowInsecureGRPC, "notifications")
		if notifErr != nil {
			log.Printf("WARNING: notifications service unavailable: %v (falling back to direct DB)", notifErr)
		} else {
			defer notifConn.Close()
			usersUC.SetNotificationSender(notificationsClient.NewNotificationsGRPCClient(notifConn))
			log.Printf("Notifications gRPC client connected: %s", cfg.NotifGRPC.Target())
		}
	}

	if cfg.Kafka.Enabled() {
		kp := kafka.NewKafkaProducer(cfg.Kafka.Brokers)
		usersUC.SetKafkaProducer(kp)
		danceUC.SetKafkaProducer(kp)
		comparisonUC.SetKafkaProducer(kp)
		socialUC.SetKafkaProducer(kp)
		log.Printf("Kafka producer wired: brokers=%v", cfg.Kafka.Brokers)
	}

	// --- handlers ---
	handlers := appHandlers{
		auth:       authHandlers.NewAuthHandler(authClient, authUC),
		user:       userHandlers.NewUserHandler(authClient, usersUC, comparisonUC, danceUC, cfg.CookieSecure, cfg.CookieSameSite),
		dance:      danceHttp.NewDanceHandler(danceUC, cfg.CookieSecure, cfg.CookieSameSite, cfg.MLInternalToken),
		comparison: comparisonHttp.NewComparisonHandler(comparisonUC),
		social:     socialHttp.NewSocialHandler(socialUC),
		profile:    profileHttp.NewProfileHandler(profileUC),
		recommend:  recommendHttp.NewRecommendHandler(recommendUC),
	}

	var botUploadRL, createDuelRL func(http.Handler) http.Handler
	if cfg.Redis.Enabled() {
		handlers.events = events.NewHandler(cfg.Redis.Addr)
		botUploadRL = ratelimit.NewRedisRateLimiter(cfg.Redis.Addr, 5, time.Hour, "bot_upload").Middleware
		createDuelRL = ratelimit.NewRedisRateLimiter(cfg.Redis.Addr, 10, time.Minute, "create_duel").Middleware
	}

	ddLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	metricsServer := startMetricsServer(cfg.MetricsPort)

	botSecretMW := userHandlers.BotSecretMiddleware(cfg.BotSecret)
	adminMW := userHandlers.AdminTokenMiddleware(cfg.AdminToken)

	mainRouter := setupRouter(handlers, ddLogger, botUploadRL, createDuelRL, botSecretMW, adminMW)

	danceSrv := http.Server{
		Handler:           mainRouter,
		Addr:              cfg.MainAddr,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      15 * time.Minute,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("Starting main server on %s!", cfg.MainAddr)
		if err := danceSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Server start error: %v", err)
			os.Exit(1)
		}
	}()

	jobsCtx, jobsCancel := context.WithCancel(context.Background())
	defer jobsCancel()
	go runUserVideoCleanupWorker(jobsCtx, comparisonUC,
		time.Duration(cfg.UserVideoTTLHours)*time.Hour,
		time.Duration(cfg.UserVideoCleanupPeriodMinutes)*time.Minute)
	go runDuelExpireJob(jobsCtx, usersUC)

	quitChannel := make(chan os.Signal, 1)
	signal.Notify(quitChannel, syscall.SIGINT, syscall.SIGTERM)

	<-quitChannel
	log.Printf("Shutting down gracefully...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := danceSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Graceful shutdown failed: %v", err)
		os.Exit(1)
	}
	if err := metricsServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("metrics server shutdown failed: %v", err)
	}
	if handlers.events != nil {
		if err := handlers.events.Close(); err != nil {
			log.Printf("events handler close failed: %v", err)
		}
	}
	log.Printf("Graceful shutdown complete")
}
