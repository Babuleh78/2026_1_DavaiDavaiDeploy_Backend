package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"DDDance/internal/models"
	"DDDance/internal/pkg/recommendation"
	"DDDance/internal/pkg/utils/log"

	uuid "github.com/satori/go.uuid"
	"golang.org/x/sync/singleflight"
)

const (
	recommenderCacheTTL     = 5 * time.Minute
	reelsCandidatesCacheTTL = 3 * time.Minute
)

type candidateCache struct {
	mu        sync.Mutex
	items     []models.RecommenderDanceItem
	updatedAt time.Time
	group     singleflight.Group
}

func (c *candidateCache) load(ctx context.Context, ttl time.Duration, fetch func(context.Context) ([]models.RecommenderDanceItem, error)) ([]models.RecommenderDanceItem, error) {
	c.mu.Lock()
	if c.items != nil && time.Since(c.updatedAt) < ttl {
		items := c.items
		c.mu.Unlock()
		return items, nil
	}
	c.mu.Unlock()

	v, err, _ := c.group.Do("load", func() (interface{}, error) {
		items, err := fetch(ctx)
		if err != nil {
			return nil, err
		}
		c.mu.Lock()
		c.items = items
		c.updatedAt = time.Now()
		c.mu.Unlock()
		return items, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]models.RecommenderDanceItem), nil
}

func (c *candidateCache) invalidate() {
	c.mu.Lock()
	c.items = nil
	c.mu.Unlock()
}

type RecommendationUsecase struct {
	dataRepo   recommendation.RecommendationDataRepo
	mlClient   recommendation.MLClientInterface
	reelsCache recommendation.ReelsCache
	s3Address  string

	recommenderCandidates candidateCache
	reelsCandidates       candidateCache
}

func NewRecommendationUsecase(dataRepo recommendation.RecommendationDataRepo, mlClient recommendation.MLClientInterface, s3Address string) *RecommendationUsecase {
	return &RecommendationUsecase{
		dataRepo:  dataRepo,
		mlClient:  mlClient,
		s3Address: s3Address,
	}
}

func (uc *RecommendationUsecase) SetReelsCache(c recommendation.ReelsCache) {
	uc.reelsCache = c
}

func (uc *RecommendationUsecase) InvalidateCache() {
	uc.recommenderCandidates.invalidate()
	uc.reelsCandidates.invalidate()
}

func (uc *RecommendationUsecase) getDancesForRecommender(ctx context.Context) ([]models.RecommenderDanceItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	items, err := uc.recommenderCandidates.load(ctx, recommenderCacheTTL, uc.dataRepo.GetDancesForRecommender)
	if err != nil {
		logger.Error("failed to get dances for recommender from db", "error", err)
		return nil, err
	}
	return items, nil
}

func (uc *RecommendationUsecase) getCandidateDancesForReels(ctx context.Context) ([]models.RecommenderDanceItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	items, err := uc.reelsCandidates.load(ctx, reelsCandidatesCacheTTL, uc.dataRepo.GetCandidateDancesForReels)
	if err != nil {
		logger.Error("failed to fetch candidate dances for reels", "error", err)
		return nil, err
	}
	return items, nil
}

func (uc *RecommendationUsecase) GetRecommendations(ctx context.Context, query string) (*models.RecommendResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	dances, err := uc.getDancesForRecommender(ctx)
	if err != nil {
		return nil, err
	}

	recommendedIDs, reasoning, err := uc.mlClient.Recommend(ctx, query, dances, 10)
	if err != nil {
		logger.Error("ml recommend call failed", "error", err)
		return nil, fmt.Errorf("recommend request failed: %w", err)
	}

	s3Address := strings.TrimRight(uc.s3Address, "/")
	danceByID := make(map[string]models.RecommenderDanceItem, len(dances))
	for _, d := range dances {
		danceByID[d.ID] = d
	}

	result := make([]models.RecommendDance, 0, len(recommendedIDs))
	for _, id := range recommendedIDs {
		d, ok := danceByID[id]
		if !ok {
			continue
		}
		result = append(result, models.RecommendDance{
			ID:        d.ID,
			Title:     d.Title,
			URL:       fmt.Sprintf("%s/results/%s/video.mp4", s3Address, d.ID),
			AvgScore:  d.AvgScore,
			ViewCount: d.ViewCount,
		})
	}

	return &models.RecommendResponse{
		Reasoning: reasoning,
		Dances:    result,
	}, nil
}

func (uc *RecommendationUsecase) GetSimilarDances(ctx context.Context, danceID string) ([]models.RecommendDance, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	dances, err := uc.getDancesForRecommender(ctx)
	if err != nil {
		return nil, err
	}

	recommendedIDs, err := uc.mlClient.GetSimilar(ctx, danceID, dances, 4)
	if err != nil {
		logger.Error("ml similar call failed", "error", err)
		return nil, fmt.Errorf("similar request failed: %w", err)
	}

	s3Address := strings.TrimRight(uc.s3Address, "/")
	danceByID := make(map[string]models.RecommenderDanceItem, len(dances))
	for _, d := range dances {
		danceByID[d.ID] = d
	}

	result := make([]models.RecommendDance, 0, 4)
	for _, id := range recommendedIDs {
		if id == danceID {
			continue
		}
		d, ok := danceByID[id]
		if !ok {
			continue
		}
		result = append(result, models.RecommendDance{
			ID:        d.ID,
			Title:     d.Title,
			URL:       fmt.Sprintf("%s/results/%s/video.mp4", s3Address, d.ID),
			AvgScore:  d.AvgScore,
			ViewCount: d.ViewCount,
		})
		if len(result) == 4 {
			break
		}
	}

	return result, nil
}

func (uc *RecommendationUsecase) GetReelsFeed(ctx context.Context, limit, offset int, excludeIDs []string, userID *uuid.UUID, behaviorLog []models.BehaviorLogEntry) (*models.ReelsFeedResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if userID != nil && uc.reelsCache != nil && len(excludeIDs) == 0 {
		if cached, ok := uc.reelsCache.Get(ctx, *userID); ok {
			logger.Info("reels cache hit", "user_id", userID)
			return cached, nil
		}
	}

	if userID != nil {
		items, err := uc.getPersonalized(ctx, logger, limit, excludeIDs, userID, behaviorLog)
		if err == nil {
			total, countErr := uc.dataRepo.GetReelsFeedCount(ctx, excludeIDs)
			if countErr != nil {
				logger.Warn("failed to get reels feed count", "error", countErr)
			}
			resp := &models.ReelsFeedResponse{Items: items, Total: total}
			if uc.reelsCache != nil && len(excludeIDs) == 0 {
				if setErr := uc.reelsCache.Set(ctx, *userID, resp, 600*time.Second); setErr != nil {
					logger.Warn("failed to set reels cache", "error", setErr)
				}
			}
			return resp, nil
		}
		logger.Info("personalized reels failed, falling back to default", "error", err)
	}

	return uc.getFallback(ctx, logger, limit, offset, excludeIDs, userID)
}

func (uc *RecommendationUsecase) getPersonalized(
	ctx context.Context,
	logger *slog.Logger,
	limit int,
	excludeIDs []string,
	userID *uuid.UUID,
	behaviorLog []models.BehaviorLogEntry,
) ([]models.ReelItem, error) {
	history, err := uc.dataRepo.GetUserReelsHistory(ctx, *userID)
	if err != nil {
		return nil, fmt.Errorf("get reels history: %w", err)
	}

	candidates, err := uc.getCandidateDancesForReels(ctx)
	if err != nil {
		return nil, fmt.Errorf("get candidates: %w", err)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no candidates available")
	}

	friendIDs, friendErr := uc.dataRepo.GetFriendIDs(ctx, *userID)
	if friendErr != nil {
		logger.Warn("failed to get friend ids for reels, proceeding without friend signal", "error", friendErr)
		friendIDs = []string{}
	}

	mlIDs, err := uc.mlClient.GetPersonalizedReels(ctx, history, candidates, limit, excludeIDs, behaviorLog, friendIDs)
	if err != nil {
		return nil, err
	}

	mlItems, err := uc.dataRepo.GetReelsByIDs(ctx, mlIDs, userID)
	if err != nil {
		return nil, fmt.Errorf("fetch reels by ids: %w", err)
	}

	byID := make(map[string]models.ReelItem, len(mlItems))
	for _, it := range mlItems {
		byID[it.DanceID] = it
	}
	ordered := make([]models.ReelItem, 0, len(mlIDs))
	for _, id := range mlIDs {
		if it, ok := byID[id]; ok {
			ordered = append(ordered, it)
		}
	}

	if len(ordered) < limit {
		usedIDs := make([]string, 0, len(ordered)+len(excludeIDs))
		for _, it := range ordered {
			usedIDs = append(usedIDs, it.DanceID)
		}
		usedIDs = append(usedIDs, excludeIDs...)

		extra, extraErr := uc.dataRepo.GetReelsFeed(ctx, limit-len(ordered), 0, usedIDs, userID)
		if extraErr == nil {
			ordered = append(ordered, extra...)
		}
	}

	logger.Info("personalized reels feed ready", "count", len(ordered))
	return ordered, nil
}

func (uc *RecommendationUsecase) getFallback(
	ctx context.Context,
	logger *slog.Logger,
	limit, offset int,
	excludeIDs []string,
	userID *uuid.UUID,
) (*models.ReelsFeedResponse, error) {
	items, err := uc.dataRepo.GetReelsFeed(ctx, limit, offset, excludeIDs, userID)
	if err != nil {
		logger.Error("failed to get reels feed", "error", err)
		return nil, err
	}

	if offset < limit && len(items) > 1 {
		rand.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })
	}

	total, err := uc.dataRepo.GetReelsFeedCount(ctx, excludeIDs)
	if err != nil {
		logger.Error("failed to count reels feed", "error", err)
		return nil, err
	}

	return &models.ReelsFeedResponse{Items: items, Total: total}, nil
}
