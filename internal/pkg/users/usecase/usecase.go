package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"os"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/argon2"
)

func HashPass(plainPassword string) ([]byte, error) {
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	hashedPass := argon2.IDKey([]byte(plainPassword), []byte(salt), 1, 64*1024, 4, 32)
	return append(salt, hashedPass...), nil
}

func CheckPass(passHash []byte, plainPassword string) bool {
	salt := make([]byte, 8)
	copy(salt, passHash[:8])
	userHash := argon2.IDKey([]byte(plainPassword), salt, 1, 64*1024, 4, 32)
	userHashedPassword := append(salt, userHash...)
	return subtle.ConstantTimeCompare(userHashedPassword, passHash) == 1
}

type UserUsecase struct {
	secret        string
	userRepo      users.UsersRepo
	storageRepo   users.StorageRepo
	botNotifier   users.BotNotifier
	notifSender   users.NotificationSender
	kafkaProducer users.KafkaPublisher
	ssePublisher  users.SSEPublisher
}

func NewUserUsecase(userRepo users.UsersRepo, storageRepo users.StorageRepo) *UserUsecase {
	jwtSecret := os.Getenv("JWT_SECRET")
	if len(jwtSecret) < 32 {
		fmt.Fprintf(os.Stderr, "FATAL: JWT_SECRET must be at least 32 characters, got %d\n", len(jwtSecret))
		os.Exit(1)
	}
	return &UserUsecase{
		secret:      jwtSecret,
		userRepo:    userRepo,
		storageRepo: storageRepo,
	}
}

func (uc *UserUsecase) SetBotNotifier(bn users.BotNotifier) {
	uc.botNotifier = bn
}

func (uc *UserUsecase) SetNotificationSender(ns users.NotificationSender) {
	uc.notifSender = ns
}

func (uc *UserUsecase) SetKafkaProducer(kp users.KafkaPublisher) {
	uc.kafkaProducer = kp
}

func (uc *UserUsecase) SetSSEPublisher(sp users.SSEPublisher) {
	uc.ssePublisher = sp
}

func (uc *UserUsecase) GenerateToken(id uuid.UUID, login string, version int) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":      id,
		"login":   login,
		"version": version,
		"exp":     time.Now().Add(time.Hour * 12).Unix(),
	})
	return token.SignedString([]byte(uc.secret))
}

func (uc *UserUsecase) ParseToken(token string) (*jwt.Token, error) {
	return jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(uc.secret), nil
	})
}

func (uc *UserUsecase) ValidateAndGetUser(ctx context.Context, token string) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if token == "" {
		logger.Error("no token")
		return models.User{}, users.ErrorUnauthorized
	}

	parsedToken, err := uc.ParseToken(token)
	if err != nil || !parsedToken.Valid {
		return models.User{}, users.ErrorUnauthorized
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		logger.Error("invalid claims")
		return models.User{}, users.ErrorUnauthorized
	}

	exp, ok := claims["exp"].(float64)
	if !ok || int64(exp) < time.Now().Unix() {
		logger.Error("invalid exp claim")
		return models.User{}, users.ErrorUnauthorized
	}

	login, ok := claims["login"].(string)
	if !ok || login == "" {
		logger.Error("invalid login claim")
		return models.User{}, users.ErrorUnauthorized
	}

	user, err := uc.userRepo.GetUserByLogin(ctx, login)
	if err != nil {
		return models.User{}, users.ErrorUnauthorized
	}

	version, ok := claims["version"].(float64)
	if !ok {
		logger.Error("invalid version claim")
		return models.User{}, users.ErrorUnauthorized
	}

	if int(version) != user.Version {
		logger.Error("token version mismatch")
		return models.User{}, users.ErrorUnauthorized
	}

	return user, nil
}

func (uc *UserUsecase) GetUser(ctx context.Context, id uuid.UUID) (models.User, error) {
	user, err := uc.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (uc *UserUsecase) ChangePassword(ctx context.Context, id uuid.UUID, oldPassword string, newPassword string) (models.User, string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	neededUser, err := uc.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return models.User{}, "", err
	}

	if !CheckPass(neededUser.PasswordHash, oldPassword) {
		logger.Error("wrong old password")
		return models.User{}, "", users.ErrorBadRequest
	}

	msg, passwordIsValid := users.Validation(neededUser.Login, newPassword)
	if !passwordIsValid {
		logger.Error(msg)
		return models.User{}, "", users.ErrorBadRequest
	}

	if newPassword == oldPassword {
		logger.Error("passwords are equal")
		return models.User{}, "", users.ErrorBadRequest
	}

	neededUser.Version += 1

	passwordHash, err := HashPass(newPassword)
	if err != nil {
		logger.Error("cannot hash password")
		return models.User{}, "", users.ErrorInternalServerError
	}

	err = uc.userRepo.UpdateUserPassword(ctx, neededUser.Version, neededUser.ID, passwordHash)
	if err != nil {
		return models.User{}, "", err
	}

	neededUser.PasswordHash = passwordHash
	neededUser.UpdatedAt = time.Now().UTC()

	token, err := uc.GenerateToken(neededUser.ID, neededUser.Login, neededUser.Version)
	if err != nil {
		return models.User{}, "", err
	}

	return neededUser, token, nil
}
