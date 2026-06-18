package recommendation

import (
	"DDDance/internal/models"
	"context"
	"time"

	uuid "github.com/satori/go.uuid"
)

type RecommendationUsecase interface {
	GetRecommendations(ctx context.Context, query string) (*models.RecommendResponse, error)
	GetReelsFeed(ctx context.Context, limit, offset int, excludeIDs []string, userID *uuid.UUID, behaviorLog []models.BehaviorLogEntry) (*models.ReelsFeedResponse, error)
	GetSimilarDances(ctx context.Context, danceID string) ([]models.RecommendDance, error)
	InvalidateCache()
}

type RecommendationDataRepo interface {
	GetDancesForRecommender(ctx context.Context) ([]models.RecommenderDanceItem, error)
	GetCandidateDancesForReels(ctx context.Context) ([]models.RecommenderDanceItem, error)
	GetReelsByIDs(ctx context.Context, ids []string, userID *uuid.UUID) ([]models.ReelItem, error)
	GetReelsFeed(ctx context.Context, limit, offset int, excludeIDs []string, userID *uuid.UUID) ([]models.ReelItem, error)
	GetReelsFeedCount(ctx context.Context, excludeIDs []string) (int, error)
	GetUserReelsHistory(ctx context.Context, userID uuid.UUID) ([]models.UserReelsHistoryItem, error)
	GetFriendIDs(ctx context.Context, userID uuid.UUID) ([]string, error)
}

type ReelsCache interface {
	Get(ctx context.Context, userID uuid.UUID) (*models.ReelsFeedResponse, bool)
	Set(ctx context.Context, userID uuid.UUID, resp *models.ReelsFeedResponse, ttl time.Duration) error
}

type MLClientInterface interface {
	Recommend(ctx context.Context, query string, dances []models.RecommenderDanceItem, limit int) (recommendedIDs []string, reasoning string, err error)
	GetSimilar(ctx context.Context, danceID string, dances []models.RecommenderDanceItem, limit int) ([]string, error)
	GetPersonalizedReels(ctx context.Context, history []models.UserReelsHistoryItem, candidates []models.RecommenderDanceItem, limit int, excludeIDs []string, behaviorLog []models.BehaviorLogEntry, friendUploaderIDs []string) ([]string, error)
}
