package comparison

import (
	"DDDance/internal/models"
	"context"
	"time"

	uuid "github.com/satori/go.uuid"
)

type Top10Cache interface {
	SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error)
}

type Leaderboard interface {
	ZAdd(ctx context.Context, scope string, score float64, userID string) error
	ZTopMembers(ctx context.Context, scope string, count int64) ([]string, error)
	ZRevRank(ctx context.Context, scope, userID string) (int64, error)
	ZCard(ctx context.Context, scope string) (int64, error)
}

type ComparisonUsecase interface {
	SaveAttempt(ctx context.Context, userID uuid.UUID, attemptID, danceID string, includeVideo bool, fallbackScore *float64, userName string, isPrivate bool, duelID *uuid.UUID, publicConsent bool, submitToDuels bool, timingScore, amplitudeScore, poseScore float64) error
	UnsaveAttempt(ctx context.Context, userID uuid.UUID, attemptID string) error
	GetSavedAttempts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.SavedAttemptItem, error)
	GetUserAttempts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.UserAttemptItem, error)
	GetCompareResult(ctx context.Context, userID uuid.UUID, userDanceID string) (*models.CompareResult, error)
	GetLeaderboard(ctx context.Context, danceID string, userID *uuid.UUID) (*models.LeaderboardResponse, error)
	GetTopDancers(ctx context.Context) ([]models.TopDancerEntry, error)
	CompareDanceFromBuffer(ctx context.Context, buffer []byte, fileFormat string, referenceDanceID string, userID *uuid.UUID) (*models.CompareResult, error)
	GetTaskStatus(ctx context.Context, taskID, taskType, danceID, userDanceID, videoKey, uploaderUserID string, userID *uuid.UUID) (*models.TaskStatusResponse, error)
	CleanupExpiredUserVideos(ctx context.Context, ttl time.Duration) (int, error)
	GetMyDanceProgress(ctx context.Context, userID uuid.UUID, danceID string) ([]models.DanceProgressEntry, error)
	GetReelsAttempts(ctx context.Context, limit, offset int) ([]models.ReelsAttemptItem, error)
	GetFriendsDanceScores(ctx context.Context, userID uuid.UUID, danceID string) ([]models.FriendScore, error)
	GetUserWeakSpots(ctx context.Context, userID uuid.UUID) (*models.WeakSpots, error)
}

type ComparisonRepo interface {
	GetUserDanceRank(ctx context.Context, danceID string, userID uuid.UUID) (*models.LeaderboardEntry, error)
	GetUserGlobalRank(ctx context.Context, userID uuid.UUID) (int, error)
	GetFriendsDanceScores(ctx context.Context, userID uuid.UUID, danceID string) ([]models.FriendScore, error)
	RecordDanceAttempt(ctx context.Context, danceID string, userID *uuid.UUID, attemptID string, score float64) error
	SaveAttempt(ctx context.Context, userID uuid.UUID, attemptID, danceID string, score float64, hasVideo bool, userName string, isPrivate bool, timingScore, amplitudeScore, poseScore float64) error
	GetUserWeakSpots(ctx context.Context, userID uuid.UUID) (*models.WeakSpots, error)
	UnsaveAttempt(ctx context.Context, userID uuid.UUID, attemptID string) error
	IsSavedAttemptWithVideo(ctx context.Context, userID uuid.UUID, attemptID string) (bool, error)
	IsAttemptPrivate(ctx context.Context, attemptID string) (bool, error)
	SavedAttemptExists(ctx context.Context, userID uuid.UUID, attemptID string) (bool, error)
	GetSavedAttempts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.SavedAttemptItem, error)
	GetUserAttempts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.UserAttemptItem, error)
	GetAttemptOwner(ctx context.Context, attemptID string) (*uuid.UUID, error)
	GetDanceStats(ctx context.Context, danceID string) (*models.DanceStats, error)
	GetDanceLeaderboard(ctx context.Context, danceID string) ([]models.LeaderboardEntry, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error)
	GetTopDancers(ctx context.Context) ([]models.TopDancerEntry, error)
	CreateCompareTask(ctx context.Context, taskID, danceID, userDanceID, videoKey string) error
	GetCompareTask(ctx context.Context, taskID string) (models.CompareTask, error)
	MarkCompareTaskFinalized(ctx context.Context, taskID string) (bool, error)
	GetDanceProgress(ctx context.Context, userID uuid.UUID, danceID string) ([]models.DanceProgressEntry, error)
	GetReelsAttempts(ctx context.Context, limit, offset int) ([]models.ReelsAttemptItem, error)
	GetUserTelegramID(ctx context.Context, userID uuid.UUID) (*int64, error)
	GetDanceTitleByID(ctx context.Context, danceID string) (string, error)
	GetTopDancersByIDs(ctx context.Context, userIDs []string) ([]models.TopDancerEntry, error)
}

type ComparisonStorageRepo interface {
	DownloadFile(ctx context.Context, s3Key string) ([]byte, error)
	FileExists(ctx context.Context, s3Key string) bool
	DeleteFile(ctx context.Context, s3Key string) error
	UploadFileRaw(ctx context.Context, localPath string, s3Key string) error
	ListUserVideos(ctx context.Context) ([]models.StoredUserVideo, error)
}

type AchievementTrigger interface {
	CheckAndUnlockAchievements(ctx context.Context, userID uuid.UUID) ([]models.Achievement, error)
	TriggerNightDancerAchievement(ctx context.Context, userID uuid.UUID)
	TriggerSpeedLearnerAchievement(ctx context.Context, userID uuid.UUID, danceID string)
	TriggerTopDancerAchievement(ctx context.Context, userID uuid.UUID)
}

type DuelSubmitter interface {
	SubmitDuelAttempt(ctx context.Context, userID uuid.UUID, duelID uuid.UUID, attemptID uuid.UUID, publicConsent bool) error
	SubmitAttemptToActiveDuels(ctx context.Context, userID uuid.UUID, danceID string, attemptID uuid.UUID, publicConsent bool) error
}

type UploadFinalizer interface {
	FinalizeUploadTask(ctx context.Context, danceID string, result *models.ProcessingResult, uploaderUserID string) error
}

type KafkaPublisher interface {
	PublishAsync(ctx context.Context, topic, key string, value []byte, errFn func(error))
}

type BotNotifier interface {
	Push(ctx context.Context, notification []byte) error
}

type SSEPublisher interface {
	Publish(ctx context.Context, userID string, payload []byte) error
}
