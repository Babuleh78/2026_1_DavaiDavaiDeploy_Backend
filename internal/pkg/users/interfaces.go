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
	GetDanceByID(ctx context.Context, danceID string, userID *uuid.UUID) (*models.UploadDanceResult, error)
	GetSegmentDescription(ctx context.Context, danceID string, segmentIdx int) (*models.SegmentDescriptionResult, error)
	GetMainPage(ctx context.Context) ([]models.VideoItem, error)
	AddToHistory(ctx context.Context, userID uuid.UUID, danceID string, sourceURL string) error
	GetHistory(ctx context.Context, userID uuid.UUID) ([]models.SearchHistoryItem, error)
	DeleteFromHistory(ctx context.Context, historyID uuid.UUID, userID uuid.UUID) error
	UpdateHistoryName(ctx context.Context, historyID uuid.UUID, userID uuid.UUID, name string) error
	ToggleLike(ctx context.Context, userID uuid.UUID, danceID string) (*models.LikeResponse, error)
	GetTopLikedDances(ctx context.Context, limit int) ([]models.DanceLikeStat, error)
	CompareDance(ctx context.Context, videoKey string, danceID string, segmentIdx int) (*models.DanceCompareResponse, error)
	GetUserLikedDances(ctx context.Context, userID uuid.UUID) ([]models.DanceLike, error)
}


type UsersRepo interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error)
	GetUserByLogin(ctx context.Context, login string) (models.User, error)
	UpdateUserPassword(ctx context.Context, version int, userID uuid.UUID, passwordHash []byte) error
	AddToHistory(ctx context.Context, userID uuid.UUID, danceID string, sourceURL string) error
	GetHistory(ctx context.Context, userID uuid.UUID) ([]models.SearchHistoryItem, error)
	DeleteFromHistory(ctx context.Context, historyID uuid.UUID, userID uuid.UUID) error
	UpdateHistoryName(ctx context.Context, historyID uuid.UUID, userID uuid.UUID, name string) error
	ToggleLike(ctx context.Context, userID uuid.UUID, danceID string) (liked bool, err error)
	GetLikesCount(ctx context.Context, danceID string) (int64, error)
	IsLikedByUser(ctx context.Context, userID uuid.UUID, danceID string) (bool, error)
	GetTopLikedDances(ctx context.Context, limit int) ([]models.DanceLikeStat, error)
	GetUserLikedDances(ctx context.Context, userID uuid.UUID) ([]models.DanceLike, error)
	CleanHistory(ctx context.Context, userID uuid.UUID) error
}

type StorageRepo interface {
	UploadDance(ctx context.Context, buffer []byte, fileFormat string, danceExtension string) (string, error)
	ListDances(ctx context.Context, maxKeys int) ([]string, error)
	DownloadFile(ctx context.Context, s3Key string) ([]byte, error)
}
