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
	UploadDance(ctx context.Context, buffer []byte, fileFormat string) (*models.UploadDanceResult, error)
	UploadDanceByURL(ctx context.Context, videoURL string) (*models.UploadDanceResult, error)
	GetDanceByID(ctx context.Context, danceID string) (*models.UploadDanceResult, error)
	GetSegmentDescription(ctx context.Context, danceID string, segmentIdx int) (*models.SegmentDescriptionResult, error)
	GetMainPage(ctx context.Context) ([]models.VideoItem, error)
}


type UsersRepo interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error)
	GetUserByLogin(ctx context.Context, login string) (models.User, error)
	UpdateUserPassword(ctx context.Context, version int, userID uuid.UUID, passwordHash []byte) error
}

type StorageRepo interface {
	UploadDance(ctx context.Context, buffer []byte, fileFormat string, danceExtension string) (string, error)
	ListDances(ctx context.Context, maxKeys int) ([]string, error)
	DownloadFile(ctx context.Context, s3Key string) ([]byte, error)
}
