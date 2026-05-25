// @title           DDDance API
// @version         1.0
// @description     API для авторизации пользователей
// @host            localhost:5458
// @BasePath        /api
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	grpccreds "google.golang.org/grpc/credentials"

	authHandler "DDDance/internal/pkg/auth/delivery/grpc"
	authRepo "DDDance/internal/pkg/auth/repo"
	authUsecase "DDDance/internal/pkg/auth/usecase"
	"DDDance/internal/pkg/middleware/logger"
	userRepo "DDDance/internal/pkg/users/repo/pg"
	storageRepo "DDDance/internal/pkg/users/repo/s3"
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

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")),
		config.WithEndpointResolverWithOptions(customResolver),
		config.WithHTTPClient(customHTTPClient),
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

	authRepo := authRepo.NewAuthRepository(dbpool)
	authUsecase := authUsecase.NewAuthUsecase(authRepo)
	userRepo := userRepo.NewUserRepository(dbpool)
	s3Repo := storageRepo.NewS3Repository(s3Client, s3Bucket)
	userUsecase := userUsecase.NewUserUsecase(userRepo, s3Repo)

	// инициализация gRPC хендлера
	authHandler := authHandler.NewGrpcAuthHandler(authUsecase, userUsecase)

	ddLogger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	grpcOpts := []grpc.ServerOption{
		grpc.ChainUnaryInterceptor(logger.LoggerInterceptor(ddLogger)),
		grpc.MaxRecvMsgSize(64 * 1024 * 1024),
		grpc.MaxSendMsgSize(64 * 1024 * 1024),
	}

	certFile, keyFile := os.Getenv("GRPC_TLS_CERT"), os.Getenv("GRPC_TLS_KEY")
	if certFile != "" && keyFile != "" {
		creds, err := grpccreds.NewServerTLSFromFile(certFile, keyFile)
		if err != nil {
			log.Fatalf("failed to load gRPC TLS cert: %v", err)
		}
		grpcOpts = append(grpcOpts, grpc.Creds(creds))
		log.Println("gRPC server: TLS enabled")
	} else {
		log.Println("WARNING: gRPC server running WITHOUT TLS (GRPC_TLS_CERT/GRPC_TLS_KEY not set)")
	}
	gRPCServer := grpc.NewServer(grpcOpts...)
	gen.RegisterAuthServer(gRPCServer, authHandler)

	r := mux.NewRouter().PathPrefix("").Subrouter()
	http.Handle("/", r)

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
