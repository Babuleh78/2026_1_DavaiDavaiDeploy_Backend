package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/kafka"
	"DDDance/internal/pkg/social"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"encoding/json"
	"log/slog"
	"time"

	uuid "github.com/satori/go.uuid"
)

type SocialUsecase struct {
	socialRepo    social.SocialRepo
	achTrigger    social.AchievementTrigger
	kafkaProducer social.KafkaPublisher
}

func NewSocialUsecase(socialRepo social.SocialRepo) *SocialUsecase {
	return &SocialUsecase{socialRepo: socialRepo}
}

func (uc *SocialUsecase) SetAchievementTrigger(at social.AchievementTrigger) {
	uc.achTrigger = at
}

func (uc *SocialUsecase) SetKafkaProducer(kp social.KafkaPublisher) {
	uc.kafkaProducer = kp
}

func (uc *SocialUsecase) triggerAchievementCheck(ctx context.Context, userID uuid.UUID) {
	if uc.achTrigger == nil {
		return
	}
	detached := context.WithoutCancel(ctx)
	go func() {
		if _, err := uc.achTrigger.CheckAndUnlockAchievements(detached, userID); err != nil {
			log.GetLoggerFromContext(detached).Warn("async achievement check failed", "user_id", userID, "error", err)
		}
	}()
}

func (uc *SocialUsecase) ToggleLike(ctx context.Context, userID uuid.UUID, danceID string) (*models.LikeResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if danceID == "" {
		logger.Error("empty danceID")
		return nil, users.ErrorBadRequest
	}

	liked, err := uc.socialRepo.ToggleLike(ctx, userID, danceID)
	if err != nil {
		return nil, err
	}

	count, err := uc.socialRepo.GetLikesCount(ctx, danceID)
	if err != nil {
		return nil, err
	}

	if liked {
		if author, aErr := uc.socialRepo.GetDanceAuthor(ctx, danceID); aErr == nil && author != nil {
			if authorID, pErr := uuid.FromString(author.ID); pErr == nil {
				if uc.kafkaProducer != nil {
					payload, _ := json.Marshal(map[string]interface{}{
						"user_id":  authorID.String(),
						"liker_id": userID.String(),
						"dance_id": danceID,
						"liked":    true,
					})
					uc.kafkaProducer.PublishAsync(ctx, kafka.TopicLikeToggled, authorID.String(), payload, func(err error) {
						log.GetLoggerFromContext(ctx).Warn("kafka publish TopicLikeToggled failed", "dance_id", danceID, "error", err)
					})
				} else {
					uc.triggerAchievementCheck(ctx, authorID)
				}
			}
		}
	}

	return &models.LikeResponse{
		DanceID:    danceID,
		Liked:      liked,
		LikesCount: count,
	}, nil
}

func (uc *SocialUsecase) GetUserLikedDances(ctx context.Context, userID uuid.UUID) ([]models.DanceLike, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	likes, err := uc.socialRepo.GetUserLikedDances(ctx, userID)
	if err != nil {
		logger.Error("failed to get liked dances", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return likes, nil
}

func (uc *SocialUsecase) GetNotifications(ctx context.Context, userID uuid.UUID) (*models.NotificationsResponse, error) {
	items, err := uc.socialRepo.GetNotifications(ctx, userID)
	if err != nil {
		return nil, err
	}
	unread := 0
	for _, n := range items {
		if !n.IsRead {
			unread++
		}
	}
	return &models.NotificationsResponse{
		Notifications: items,
		UnreadCount:   unread,
	}, nil
}

func (uc *SocialUsecase) MarkNotificationRead(ctx context.Context, id int64, userID uuid.UUID) error {
	return uc.socialRepo.MarkNotificationRead(ctx, id, userID)
}

func (uc *SocialUsecase) MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error {
	return uc.socialRepo.MarkAllNotificationsRead(ctx, userID)
}

func (uc *SocialUsecase) ClearNotifications(ctx context.Context, userID uuid.UUID) error {
	return uc.socialRepo.ClearNotifications(ctx, userID)
}

func (uc *SocialUsecase) SaveRating(ctx context.Context, userID uuid.UUID, input models.SaveRatingInput) (*models.RatingResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if err := input.Validate(); err != nil {
		logger.Error("invalid rating input", "error", err)
		return nil, users.ErrorBadRequest
	}

	if err := uc.socialRepo.SaveRating(ctx, userID, input); err != nil {
		logger.Error("failed to save rating", "error", err)
		return nil, users.ErrorInternalServerError
	}

	return uc.GetAggregatedRating(ctx, input.VideoID)
}

func (uc *SocialUsecase) GetAggregatedRating(ctx context.Context, videoID string) (*models.RatingResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if videoID == "" {
		logger.Error("empty videoID")
		return nil, users.ErrorBadRequest
	}

	rating, err := uc.socialRepo.GetAggregatedRating(ctx, videoID)
	if err != nil {
		logger.Error("failed to get aggregated rating", "error", err)
		return nil, users.ErrorInternalServerError
	}

	return rating, nil
}

func (uc *SocialUsecase) GetRating(ctx context.Context, videoID string) (*models.RatingResponse, error) {
	return uc.GetAggregatedRating(ctx, videoID)
}

func (uc *SocialUsecase) GetFriendsFeed(ctx context.Context, userID uuid.UUID, limit int, cursor time.Time) (*models.FeedResponse, error) {
	items, err := uc.socialRepo.GetFriendsFeed(ctx, userID, limit, cursor)
	if err != nil {
		return nil, users.ErrorInternalServerError
	}
	resp := &models.FeedResponse{Items: items}
	if len(items) == limit {
		resp.NextCursor = items[len(items)-1].CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	return resp, nil
}
