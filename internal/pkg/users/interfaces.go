package users

import (
	"DDDance/internal/models"
	"context"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	uuid "github.com/satori/go.uuid"
)

type StoredUserVideo struct {
	Key          string
	UserID       string
	DanceID      string
	LastModified time.Time
}

type UsersUsecase interface {
	GenerateToken(id uuid.UUID, login string, version int) (string, error)
	ParseToken(token string) (*jwt.Token, error)
	GetUser(ctx context.Context, id uuid.UUID) (models.User, error)
	ValidateAndGetUser(ctx context.Context, token string) (models.User, error)
	ChangePassword(ctx context.Context, id uuid.UUID, oldPassword string, newPassword string) (models.User, string, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, newLogin *string, avatarBuffer []byte, avatarContentType string) (models.User, string, error)
	UploadDance(ctx context.Context, buffer []byte, fileFormat string, uploaderUserID string, uploaderLogin string) (*models.UploadDanceResult, error)
	UploadDanceByURL(ctx context.Context, videoURL string, uploaderUserID string, uploaderLogin string) (*models.UploadDanceResult, error)
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
	CompareDanceFromBuffer(ctx context.Context, buffer []byte, fileFormat string, referenceDanceID string, userID *uuid.UUID) (*models.CompareResult, error)
	GetUserLikedDances(ctx context.Context, userID uuid.UUID) ([]models.DanceLike, error)
	SaveRating(ctx context.Context, userID uuid.UUID, input models.SaveRatingInput) (*models.RatingResponse, error)
	GetAggregatedRating(ctx context.Context, videoID string) (*models.RatingResponse, error)
	GetRating(ctx context.Context, videoID string) (*models.RatingResponse, error)
	GetDanceTimeline(ctx context.Context, danceID string, userID string) ([]byte, error)
	GetDanceKeyframes(ctx context.Context, danceID string) ([]byte, error)
	UpdateDanceStatus(ctx context.Context, id, status string) error
	GetNotifications(ctx context.Context, userID uuid.UUID) (*models.NotificationsResponse, error)
	MarkNotificationRead(ctx context.Context, id int64, userID uuid.UUID) error
	MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error
	ClaimDanceUploads(ctx context.Context, userID uuid.UUID, danceIDs []string) error
	GetDanceCatalog(ctx context.Context, sort, search string, page, limit int) (*models.DanceCatalogResponse, error)
	GetDanceStats(ctx context.Context, danceID string) (*models.DanceStats, error)
	GetDanceTrending(ctx context.Context) (*models.TrendingResponse, error)
	GetLeaderboard(ctx context.Context, danceID string, userID *uuid.UUID) (*models.LeaderboardResponse, error)
	SaveAttempt(ctx context.Context, userID uuid.UUID, attemptID, danceID string, includeVideo bool, fallbackScore *float64, userName string, isPrivate bool) error
	UnsaveAttempt(ctx context.Context, userID uuid.UUID, attemptID string) error
	GetSavedAttempts(ctx context.Context, userID uuid.UUID) ([]models.SavedAttemptItem, error)
	GetUserAttempts(ctx context.Context, userID uuid.UUID) ([]models.UserAttemptItem, error)
	GetPublicProfile(ctx context.Context, profileUserID uuid.UUID, viewerUserID *uuid.UUID) (*models.PublicProfileResponse, error)
	SendFriendRequest(ctx context.Context, senderID, receiverID uuid.UUID) error
	RespondFriendRequest(ctx context.Context, userID uuid.UUID, friendshipID int64, accept bool) error
	GetFriends(ctx context.Context, userID uuid.UUID) ([]models.Friend, error)
	RemoveFriend(ctx context.Context, userID, friendID uuid.UUID) error
	GetUploadedDances(ctx context.Context, userID uuid.UUID) ([]models.UploadedDance, error)
	SetDanceName(ctx context.Context, userID uuid.UUID, danceID, title, difficulty string) error
	GetDanceModerationStatus(ctx context.Context, danceID string) (string, string, error)
	PublishDance(ctx context.Context, userID uuid.UUID, danceID string) error
	UnpublishDance(ctx context.Context, userID uuid.UUID, danceID string) error
	DeleteDance(ctx context.Context, userID uuid.UUID, danceID string) error
	CleanupExpiredUserVideos(ctx context.Context, ttl time.Duration) (int, error)
	RecordDanceView(ctx context.Context, danceID string, viewerID string) error
	GetCompareResult(ctx context.Context, userID uuid.UUID, userDanceID string) (*models.CompareResult, error)
	GetTaskStatus(ctx context.Context, taskID, taskType, danceID, userDanceID, videoKey, uploaderUserID string, userID *uuid.UUID) (*models.TaskStatusResponse, error)
}

type UsersRepo interface {
	GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error)
	GetUserByLogin(ctx context.Context, login string) (models.User, error)
	UpdateUserPassword(ctx context.Context, version int, userID uuid.UUID, passwordHash []byte) error
	UpdateUserProfile(ctx context.Context, userID uuid.UUID, login *string, avatar *string, bumpVersion bool) error
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
	SaveRating(ctx context.Context, userID uuid.UUID, input models.SaveRatingInput) error
	GetAggregatedRating(ctx context.Context, videoID string) (*models.RatingResponse, error)
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
	GetNotifications(ctx context.Context, userID uuid.UUID) ([]models.Notification, error)
	MarkNotificationRead(ctx context.Context, id int64, userID uuid.UUID) error
	MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error
	RecordDanceAttempt(ctx context.Context, danceID string, userID *uuid.UUID, attemptID string, score float64) error
	CreateCompareTask(ctx context.Context, taskID, danceID, userDanceID, videoKey string) error
	GetCompareTask(ctx context.Context, taskID string) (models.CompareTask, error)
	MarkCompareTaskFinalized(ctx context.Context, taskID string) (bool, error)
	GetDanceCatalog(ctx context.Context, sort, search string, page, limit int) ([]models.DanceCatalogItem, error)
	GetDanceCatalogCount(ctx context.Context, sort, search string) (int, error)
	GetDanceStats(ctx context.Context, danceID string) (*models.DanceStats, error)
	GetDancesEnrichedInfo(ctx context.Context, ids []string) (map[string]models.DanceEnrichedInfo, error)
	GetPublishedDanceIDs(ctx context.Context, ids []string) (map[string]struct{}, error)
	GetDanceTrending(ctx context.Context) ([]models.DanceCatalogItem, error)
	GetDanceLeaderboard(ctx context.Context, danceID string) ([]models.LeaderboardEntry, error)
	GetUserDanceRank(ctx context.Context, danceID string, userID uuid.UUID) (*models.LeaderboardEntry, error)
	SaveAttempt(ctx context.Context, userID uuid.UUID, attemptID, danceID string, score float64, hasVideo bool, userName string, isPrivate bool) error
	UnsaveAttempt(ctx context.Context, userID uuid.UUID, attemptID string) error
	IsSavedAttemptWithVideo(ctx context.Context, userID uuid.UUID, attemptID string) (bool, error)
	GetSavedAttempts(ctx context.Context, userID uuid.UUID) ([]models.SavedAttemptItem, error)
	GetPersonalTop(ctx context.Context, userID uuid.UUID) ([]models.PersonalTopItem, error)
	GetUserAttempts(ctx context.Context, userID uuid.UUID) ([]models.UserAttemptItem, error)
	GetLastAttempt(ctx context.Context, userID uuid.UUID, danceID string) (*models.UserAttemptItem, error)
	GetAttemptOwner(ctx context.Context, attemptID string) (*uuid.UUID, error)
	GetUploadedDancesByUser(ctx context.Context, userID uuid.UUID) ([]models.UploadedDance, error)
	UpdateDanceTitle(ctx context.Context, danceID string, title string) error
	UpdateDanceDifficulty(ctx context.Context, danceID, difficulty string, difficultyScore int) error
	DeleteDanceAndRelated(ctx context.Context, danceID string) error
	GetEffectiveDanceDifficulty(ctx context.Context, danceID string) (string, bool, error)
	RecordDanceView(ctx context.Context, danceID string, viewerID string) error
	CreateFriendship(ctx context.Context, senderID, receiverID uuid.UUID) (int64, error)
	UpdateFriendshipStatus(ctx context.Context, friendshipID int64, receiverID uuid.UUID, status string) (uuid.UUID, error)
	GetFriends(ctx context.Context, userID uuid.UUID) ([]models.Friend, error)
	GetFriendsCount(ctx context.Context, userID uuid.UUID) (int, error)
	GetFriendshipBetween(ctx context.Context, userID, otherID uuid.UUID) (*models.FriendshipStatus, error)
	DeleteFriendship(ctx context.Context, userID, friendID uuid.UUID) error
	CreateFriendNotification(ctx context.Context, toUserID, fromUserID uuid.UUID, notifType string, friendshipID int64) error
}

type StorageRepo interface {
	UploadDance(ctx context.Context, buffer []byte, fileFormat string, danceExtension string) (string, error)
	UploadAvatar(ctx context.Context, buffer []byte, contentType string, userID string) (string, error)
	ListUserVideos(ctx context.Context) ([]StoredUserVideo, error)
	ListDances(ctx context.Context, maxKeys int) ([]string, error)
	DownloadFile(ctx context.Context, s3Key string) ([]byte, error)
	UploadFileRaw(ctx context.Context, localPath string, s3Key string) error
	UploadBytes(ctx context.Context, key string, data []byte, contentType string) error
	DeleteFile(ctx context.Context, s3Key string) error
	DeleteByPrefix(ctx context.Context, prefix string) error
	FileExists(ctx context.Context, s3Key string) bool
}
