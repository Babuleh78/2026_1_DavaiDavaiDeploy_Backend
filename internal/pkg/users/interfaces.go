package users

import (
	"DDDance/internal/models"
	"context"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	uuid "github.com/satori/go.uuid"
)

type UsersUsecase interface {
	GenerateToken(id uuid.UUID, login string, version int) (string, error)
	ParseToken(token string) (*jwt.Token, error)
	GetUser(ctx context.Context, id uuid.UUID) (models.User, error)
	ValidateAndGetUser(ctx context.Context, token string) (models.User, error)
	ChangePassword(ctx context.Context, id uuid.UUID, oldPassword string, newPassword string) (models.User, string, error)
	CheckAndUnlockAchievements(ctx context.Context, userID uuid.UUID) ([]models.Achievement, error)
	GetAllAchievements(ctx context.Context) ([]models.Achievement, error)
	GetUserAchievements(ctx context.Context, userID uuid.UUID) (*models.AchievementsWithMeta, error)
	CreateDuel(ctx context.Context, challengerID uuid.UUID, opponentID uuid.UUID, mode string, danceID string) (*models.DuelWithUsers, error)
	AcceptDuel(ctx context.Context, userID uuid.UUID, duelID uuid.UUID) (*models.DuelWithUsers, error)
	DeclineDuel(ctx context.Context, userID uuid.UUID, duelID uuid.UUID) error
	SubmitDuelAttempt(ctx context.Context, userID uuid.UUID, duelID uuid.UUID, attemptID uuid.UUID, publicConsent bool) error
	SubmitAttemptToActiveDuels(ctx context.Context, userID uuid.UUID, danceID string, attemptID uuid.UUID, publicConsent bool) error
	GetActiveDuelsForUserDance(ctx context.Context, userID uuid.UUID, danceID string) ([]models.ActiveDuelForDance, error)
	GetDuelHistory(ctx context.Context, userID uuid.UUID, limit, offset int) (*models.DuelHistoryResponse, error)
	GetPublicDuels(ctx context.Context, limit, offset int) (*models.DuelHistoryResponse, error)
	GetDuelByID(ctx context.Context, userID uuid.UUID, duelID uuid.UUID) (*models.DuelWithUsers, error)
	GetDuelStats(ctx context.Context, userID uuid.UUID) (*models.DuelStats, error)
	RunExpireJob(ctx context.Context) error
	GenerateTelegramLinkCode(ctx context.Context, userID uuid.UUID) (string, error)
	LinkTelegramAccount(ctx context.Context, code string, telegramID int64) (*models.BotAuthResponse, error)
	BotGetUserByTelegramID(ctx context.Context, telegramID int64) (*models.User, error)
	BotPushNotification(ctx context.Context, telegramID int64, notifType string, payload map[string]any) error
	GetUserBotStats(ctx context.Context, userID uuid.UUID) (*models.BotUserStats, error)
}

type UsersRepo interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error)
	GetUserByLogin(ctx context.Context, login string) (models.User, error)
	UpdateUserPassword(ctx context.Context, version int, userID uuid.UUID, passwordHash []byte) error
	GetAttemptOwner(ctx context.Context, attemptID string) (*uuid.UUID, error)
	GetAllAchievements(ctx context.Context) ([]models.Achievement, error)
	GetUserAchievements(ctx context.Context, userID uuid.UUID) ([]models.UserAchievement, error)
	UnlockAchievement(ctx context.Context, userID uuid.UUID, achievementID int) (bool, error)
	GetUserStatsForAchievements(ctx context.Context, userID uuid.UUID) (*models.UserAchievementStats, error)
	GetAvgAchievementUnlockCount(ctx context.Context) (float64, error)
	CreateDuel(ctx context.Context, mode string, challengerID, opponentID uuid.UUID, danceID string, expiresAt time.Time, token string) (*models.DuelWithUsers, error)
	GetDuelByID(ctx context.Context, duelID uuid.UUID) (*models.DuelWithUsers, error)
	GetActiveDuelsForUserDance(ctx context.Context, userID uuid.UUID, danceID string) ([]models.ActiveDuelForDance, error)
	HasOpenDuel(ctx context.Context, challengerID, opponentID uuid.UUID, danceID string) (bool, error)
	EnsureSavedAttemptForDuel(ctx context.Context, userID uuid.UUID, attemptID, danceID string, score float64, isPrivate bool) error
	GetDuelsByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.DuelWithUsers, error)
	GetPublicDuels(ctx context.Context, limit, offset int) ([]models.DuelWithUsers, error)
	GetDuelStats(ctx context.Context, userID uuid.UUID) (*models.DuelStats, error)
	UpdateDuelStatus(ctx context.Context, duelID uuid.UUID, status string, challengerAttemptID, opponentAttemptID *uuid.UUID, challengerScore, opponentScore *float64, winnerID *uuid.UUID, completedAt *time.Time, isSubmitChallenger *bool, publicConsent bool) error
	ExpireDuels(ctx context.Context) ([]uuid.UUID, error)
	GetRandomDance(ctx context.Context) (string, error)
	GetDistinctChallengersCount(ctx context.Context, opponentID uuid.UUID) (int, error)
	CreateDuelNotification(ctx context.Context, toUserID uuid.UUID, fromUserID *uuid.UUID, notifType, duelID string) error
	GetBatchDuelParticipants(ctx context.Context, duelIDs []uuid.UUID) ([]models.DuelParticipants, error)
	GetDanceStatus(ctx context.Context, id string) (string, error)
	GetDanceCreatedAt(ctx context.Context, danceID string) (time.Time, error)
	FindUserByLoginOrEmail(ctx context.Context, loginOrEmail string) (models.User, error)
	UpdateUserTelegramID(ctx context.Context, userID uuid.UUID, telegramID int64) error
	GetUserByTelegramID(ctx context.Context, telegramID int64) (models.User, error)
	GetUserTelegramID(ctx context.Context, userID uuid.UUID) (*int64, error)
	SetTelegramLinkCode(ctx context.Context, userID uuid.UUID, code string, expiresAt time.Time) error
	LinkTelegramByCode(ctx context.Context, code string, telegramID int64) (models.User, error)
	GetUserGlobalRank(ctx context.Context, userID uuid.UUID) (int, error)
}

type StorageRepo interface {
	UploadDance(ctx context.Context, buffer []byte, fileFormat string, danceExtension string) (string, error)
	UploadAvatar(ctx context.Context, buffer []byte, contentType string, userID string) (string, error)
	ListDances(ctx context.Context, maxKeys int) ([]string, error)
	DownloadFile(ctx context.Context, s3Key string) ([]byte, error)
	UploadFileRaw(ctx context.Context, localPath string, s3Key string) error
	UploadBytes(ctx context.Context, key string, data []byte, contentType string) error
	DeleteFile(ctx context.Context, s3Key string) error
	DeleteByPrefix(ctx context.Context, prefix string) error
	FileExists(ctx context.Context, s3Key string) bool
}

type BotNotifier interface {
	Push(ctx context.Context, notification []byte) error
}

type NotificationSender interface {
	SendDuelNotification(ctx context.Context, toUserID uuid.UUID, fromUserID *uuid.UUID, notifType, duelID string, telegramPayload map[string]any) error
}

type KafkaPublisher interface {
	PublishAsync(ctx context.Context, topic, key string, value []byte, errFn func(error))
}

type SSEPublisher interface {
	Publish(ctx context.Context, userID string, payload []byte) error
}
