package auth

import (
	"DDDance/internal/models"
	"context"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	uuid "github.com/satori/go.uuid"
)

type AuthUsecase interface {
	GenerateToken(id uuid.UUID, login string, version int) (string, error)
	ParseToken(token string) (*jwt.Token, error)
	SignUpUser(ctx context.Context, req models.SignUpInput) (models.User, string, error)
	SignInUser(ctx context.Context, req models.SignInInput) (models.User, string, error)
	LogOutUser(ctx context.Context, userID uuid.UUID) error
	ValidateAndGetUser(ctx context.Context, token string) (models.User, error)
	GenerateQRCode(login string) ([]byte, string, error)
	VerifyOTPCode(ctx context.Context, login, secretCode string, userCode string) error
	SignUpVKUser(ctx context.Context, vkid string, login string) (models.User, string, error)
	SignInVKUser(ctx context.Context, vkid string) (models.User, string, error)
	AddNotification(ctx context.Context, userID uuid.UUID) error
}

type AuthRepo interface {
	CheckUserExists(ctx context.Context, login string) (bool, error)
	CreateUser(ctx context.Context, user models.User) error
	CheckUserLogin(ctx context.Context, login string) (models.User, error)
	IncrementUserVersion(ctx context.Context, userID uuid.UUID) error
	GetUserByLogin(ctx context.Context, login string) (models.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error)
	GetUserSecretCode(ctx context.Context, userID uuid.UUID) string
	CreateVKUser(ctx context.Context, user models.User, vkid string) error
	GetVKUser(ctx context.Context, vkid string) (models.User, error)
	AddNotification(ctx context.Context, userID uuid.UUID) error
	GetPasswordUpdates(ctx context.Context, userID uuid.UUID, offset time.Time) (bool, error)
}
