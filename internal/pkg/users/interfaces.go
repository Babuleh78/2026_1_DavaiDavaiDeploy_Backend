package users

import (
	"DDDance/internal/models"
	"context"

	jwt "github.com/golang-jwt/jwt/v5"
	uuid "github.com/satori/go.uuid"
)

type UsersUsecase interface {
	GenerateToken(id uuid.UUID, login string, version int) (string, error)
	ParseToken(token string) (*jwt.Token, error)
	GetUser(ctx context.Context, id uuid.UUID) (models.User, error)
	ValidateAndGetUser(ctx context.Context, token string) (models.User, error)
	ChangePassword(ctx context.Context, id uuid.UUID, oldPassword string, newPassword string) (models.User, string, error)
}

type UsersRepo interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error)
	GetUserByLogin(ctx context.Context, login string) (models.User, error)
	UpdateUserPassword(ctx context.Context, version int, userID uuid.UUID, passwordHash []byte) error
}
