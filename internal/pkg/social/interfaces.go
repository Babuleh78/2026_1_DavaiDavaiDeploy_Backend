package social

import (
	"DDDance/internal/models"
	"context"
	"time"

	uuid "github.com/satori/go.uuid"
)

type SocialUsecase interface {
	ToggleLike(ctx context.Context, userID uuid.UUID, danceID string) (*models.LikeResponse, error)
	GetUserLikedDances(ctx context.Context, userID uuid.UUID) ([]models.DanceLike, error)
	GetNotifications(ctx context.Context, userID uuid.UUID) (*models.NotificationsResponse, error)
	MarkNotificationRead(ctx context.Context, id int64, userID uuid.UUID) error
	MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error
	ClearNotifications(ctx context.Context, userID uuid.UUID) error
	SaveRating(ctx context.Context, userID uuid.UUID, input models.SaveRatingInput) (*models.RatingResponse, error)
	GetAggregatedRating(ctx context.Context, videoID string) (*models.RatingResponse, error)
	GetRating(ctx context.Context, videoID string) (*models.RatingResponse, error)
	GetFriendsFeed(ctx context.Context, userID uuid.UUID, limit int, cursor time.Time) (*models.FeedResponse, error)
}

type SocialRepo interface {
	ToggleLike(ctx context.Context, userID uuid.UUID, danceID string) (bool, error)
	GetLikesCount(ctx context.Context, danceID string) (int64, error)
	GetDanceAuthor(ctx context.Context, danceID string) (*models.DanceAuthor, error)
	GetUserLikedDances(ctx context.Context, userID uuid.UUID) ([]models.DanceLike, error)
	GetNotifications(ctx context.Context, userID uuid.UUID) ([]models.Notification, error)
	MarkNotificationRead(ctx context.Context, id int64, userID uuid.UUID) error
	MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error
	ClearNotifications(ctx context.Context, userID uuid.UUID) error
	SaveRating(ctx context.Context, userID uuid.UUID, input models.SaveRatingInput) error
	GetAggregatedRating(ctx context.Context, videoID string) (*models.RatingResponse, error)
	GetFriendsFeed(ctx context.Context, userID uuid.UUID, limit int, cursor time.Time) ([]models.FeedItem, error)
}

type AchievementTrigger interface {
	CheckAndUnlockAchievements(ctx context.Context, userID uuid.UUID) ([]models.Achievement, error)
}

type KafkaPublisher interface {
	PublishAsync(ctx context.Context, topic, key string, value []byte, errFn func(error))
}
