package dance

import (
	"DDDance/internal/models"
	"context"
	"time"

	uuid "github.com/satori/go.uuid"
)

type DanceUsecase interface {
	UploadDance(ctx context.Context, buffer []byte, fileFormat string, uploaderUserID string, uploaderLogin string) (*models.UploadDanceResult, error)
	UploadDanceByURL(ctx context.Context, videoURL string, uploaderUserID string, uploaderLogin string) (*models.UploadDanceResult, error)
	FinalizeUploadTask(ctx context.Context, danceID string, result *models.ProcessingResult, uploaderUserID string) error
	GetDanceByID(ctx context.Context, danceID string, userID *uuid.UUID) (*models.UploadDanceResult, error)
	GetSegmentDescription(ctx context.Context, danceID string, segmentIdx int) (*models.SegmentDescriptionResult, error)
	GetMainPage(ctx context.Context) ([]models.VideoItem, error)
	AddToHistory(ctx context.Context, userID uuid.UUID, danceID string, sourceURL string) error
	GetDanceCatalog(ctx context.Context, sort, search string, page, limit int) (*models.DanceCatalogResponse, error)
	GetDanceTrending(ctx context.Context) (*models.TrendingResponse, error)
	GetDanceStats(ctx context.Context, danceID string) (*models.DanceStats, error)
	GetDanceTimeline(ctx context.Context, danceID string, userID string) ([]byte, error)
	GetDanceKeyframes(ctx context.Context, danceID string) ([]byte, error)
	UpdateDanceStatus(ctx context.Context, id, status string) error
	RecordDanceView(ctx context.Context, danceID string, viewerID string) (int64, error)
	ClaimDanceUploads(ctx context.Context, userID uuid.UUID, danceIDs []string) error
	GetUploadedDances(ctx context.Context, userID uuid.UUID) ([]models.UploadedDance, error)
	SetDanceName(ctx context.Context, userID uuid.UUID, danceID, title, difficulty string) error
	GetDanceModerationStatus(ctx context.Context, danceID string) (string, string, error)
	PublishDance(ctx context.Context, userID uuid.UUID, danceID string) error
	UnpublishDance(ctx context.Context, userID uuid.UUID, danceID string) error
	DeleteDance(ctx context.Context, userID uuid.UUID, danceID string) error
	GetTopDances(ctx context.Context) (*models.TrendingResponse, error)
	UpdateSegmentDescription(ctx context.Context, userID uuid.UUID, danceID string, segmentIndex int, description string) error
	GetDanceChoreographerDescriptions(ctx context.Context, danceID string) (map[int]string, error)
	UpdateDanceDuration(ctx context.Context, danceID string, durationSec int) error
}

type DanceRepo interface {
	CreateDance(ctx context.Context, id, title, status, difficulty, videoPath string) error
	UpdateDanceStatus(ctx context.Context, id, status string) error
	GetDanceStatus(ctx context.Context, id string) (string, error)
	GetDanceVideoPath(ctx context.Context, id string) (string, error)
	SetDanceModerationReason(ctx context.Context, danceID, reason string) error
	GetDanceModerationReason(ctx context.Context, danceID string) (string, error)
	LinkDanceUpload(ctx context.Context, userID uuid.UUID, danceID string) error
	GetDanceUploaders(ctx context.Context, danceID string) ([]uuid.UUID, error)
	GetDanceAuthor(ctx context.Context, danceID string) (*models.DanceAuthor, error)
	CreateNotification(ctx context.Context, userID uuid.UUID, notifType, danceID, reason string) error
	GetTopLikedDances(ctx context.Context, limit int) ([]models.DanceLikeStat, error)
	GetLikesCount(ctx context.Context, danceID string) (int64, error)
	IsLikedByUser(ctx context.Context, userID uuid.UUID, danceID string) (bool, error)
	AddToHistory(ctx context.Context, userID uuid.UUID, danceID string, sourceURL string) error
	CleanHistory(ctx context.Context, userID uuid.UUID) error
	GetLastAttempt(ctx context.Context, userID uuid.UUID, danceID string) (*models.UserAttemptItem, error)
	GetPublishedDanceIDs(ctx context.Context, ids []string) (map[string]struct{}, error)
	GetDancesEnrichedInfo(ctx context.Context, ids []string) (map[string]models.DanceEnrichedInfo, error)
	GetDanceCatalog(ctx context.Context, sort, search string, page, limit int) ([]models.DanceCatalogItem, error)
	GetDanceCatalogCount(ctx context.Context, sort, search string) (int, error)
	GetDanceStats(ctx context.Context, danceID string) (*models.DanceStats, error)
	GetDanceTrending(ctx context.Context) ([]models.DanceCatalogItem, error)
	RecordDanceView(ctx context.Context, danceID string, viewerID string) error
	GetDanceViewCount(ctx context.Context, danceID string) (int64, error)
	GetUploadedDancesByUser(ctx context.Context, userID uuid.UUID) ([]models.UploadedDance, error)
	UpdateDanceTitle(ctx context.Context, danceID string, title string) error
	UpdateDanceDifficulty(ctx context.Context, danceID, difficulty string, difficultyScore int) error
	UpdateDanceDuration(ctx context.Context, danceID string, durationSec int) error
	DeleteDanceAndRelated(ctx context.Context, danceID string) error
	GetEffectiveDanceDifficulty(ctx context.Context, danceID string) (string, bool, error)
	GetDancesForRecommender(ctx context.Context) ([]models.RecommenderDanceItem, error)
	GetReelsFeed(ctx context.Context, limit, offset int, excludeIDs []string, userID *uuid.UUID) ([]models.ReelItem, error)
	GetReelsFeedCount(ctx context.Context, excludeIDs []string) (int, error)
	GetUserReelsHistory(ctx context.Context, userID uuid.UUID) ([]models.UserReelsHistoryItem, error)
	GetCandidateDancesForReels(ctx context.Context) ([]models.RecommenderDanceItem, error)
	GetReelsByIDs(ctx context.Context, ids []string, userID *uuid.UUID) ([]models.ReelItem, error)
	UpsertSegmentDescription(ctx context.Context, danceID string, segmentIndex int, description string) error
	GetDanceSegmentDescriptions(ctx context.Context, danceID string) (map[int]string, error)
	GetTopDances(ctx context.Context) ([]models.DanceCatalogItem, error)
}

type DanceStorageRepo interface {
	UploadDance(ctx context.Context, buffer []byte, fileFormat string, danceExtension string) (string, error)
	ListDances(ctx context.Context, maxKeys int) ([]string, error)
	DownloadFile(ctx context.Context, s3Key string) ([]byte, error)
	DeleteFile(ctx context.Context, s3Key string) error
	DeleteByPrefix(ctx context.Context, prefix string) error
	FileExists(ctx context.Context, s3Key string) bool
}

type ViewCache interface {
	SetNX(ctx context.Context, key string, ttl time.Duration) (bool, error)
	PFAdd(ctx context.Context, key, element string)
	PFCount(ctx context.Context, key string) (int64, error)
}

type KafkaPublisher interface {
	PublishAsync(ctx context.Context, topic, key string, value []byte, errFn func(error))
}

type AchievementTrigger interface {
	CheckAndUnlockAchievements(ctx context.Context, userID uuid.UUID) ([]models.Achievement, error)
}

type CacheInvalidator interface {
	InvalidateCache()
}

type MLLock interface {
	TryLock(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	GetValue(ctx context.Context, key string) (string, error)
	Unlock(ctx context.Context, key string) error
}
