package usecase

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/utils/log"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	uuid "github.com/satori/go.uuid"
)

type avgAchievementsCache struct {
	mu        sync.Mutex
	value     float64
	updatedAt time.Time
}

var avgAchievementsCacheInstance avgAchievementsCache

// DECISION: "special" achievements that require event-specific data (night_dancer: upload time,
func (uc *UserUsecase) triggerAchievementCheck(ctx context.Context, userID uuid.UUID) {
	detached := context.WithoutCancel(ctx)
	go func() {
		if _, err := uc.CheckAndUnlockAchievements(detached, userID); err != nil {
			log.GetLoggerFromContext(detached).Warn("async achievement check failed", "user_id", userID, "error", err)
		}
	}()
}

func (uc *UserUsecase) TriggerNightDancerAchievement(ctx context.Context, userID uuid.UUID) {
	uc.triggerNightDancerAchievement(ctx, userID)
}

func (uc *UserUsecase) TriggerSpeedLearnerAchievement(ctx context.Context, userID uuid.UUID, danceID string) {
	detached := context.WithoutCancel(ctx)
	go func() {
		logger := log.GetLoggerFromContext(detached)
		danceCreatedAt, err := uc.userRepo.GetDanceCreatedAt(detached, danceID)
		if err != nil {
			logger.Warn("speed_learner: failed to get dance created_at", "dance_id", danceID, "error", err)
			return
		}
		if time.Since(danceCreatedAt) > 24*time.Hour {
			return
		}
		all, err := uc.userRepo.GetAllAchievements(detached)
		if err != nil {
			logger.Warn("speed_learner: failed to get achievements", "user_id", userID, "error", err)
			return
		}
		for _, a := range all {
			if a.Code == "speed_learner" {
				if _, uErr := uc.userRepo.UnlockAchievement(detached, userID, a.ID); uErr != nil {
					logger.Warn("speed_learner: failed to unlock", "user_id", userID, "error", uErr)
				}
				return
			}
		}
	}()
}

func meetsThreshold(a models.Achievement, stats *models.UserAchievementStats) bool {
	t := int64(a.Threshold)
	switch a.Category {
	case "likes":
		return stats.TotalLikes >= t
	case "score":
		return stats.MaxScore >= float64(a.Threshold)
	case "upload":
		return stats.UploadCount >= t
	case "attempt":
		return stats.AttemptCount >= t
	case "duel":
		return stats.DuelCount >= t
	case "duel_win":
		return stats.DuelWinCount >= t
	case "duel_streak":
		return stats.DuelWinStreak >= t
	case "special":
		if a.Code == "variety_dancer" {
			return stats.UniqueDanceCount >= t
		}
		return false
	default:
		return false
	}
}

func (uc *UserUsecase) CheckAndUnlockAchievements(ctx context.Context, userID uuid.UUID) ([]models.Achievement, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	stats, err := uc.userRepo.GetUserStatsForAchievements(ctx, userID)
	if err != nil {
		logger.Warn("failed to get user stats for achievements", "error", err)
		return nil, err
	}

	allAchievements, err := uc.userRepo.GetAllAchievements(ctx)
	if err != nil {
		logger.Warn("failed to get all achievements", "error", err)
		return nil, err
	}

	var newlyUnlocked []models.Achievement
	for _, a := range allAchievements {
		if !meetsThreshold(a, stats) {
			continue
		}
		unlocked, err := uc.userRepo.UnlockAchievement(ctx, userID, a.ID)
		if err != nil {
			logger.Warn("failed to unlock achievement", "code", a.Code, "error", err)
			continue
		}
		if unlocked {
			newlyUnlocked = append(newlyUnlocked, a)
			appmetrics.AchievementsUnlocked.With(prometheus.Labels{"code": a.Code}).Inc()
		}
	}

	if len(newlyUnlocked) > 0 {
		detached := context.WithoutCancel(ctx)
		for _, a := range newlyUnlocked {
			ac := a
			go uc.notifyTelegram(detached, userID, "achievement_unlocked", map[string]any{
				"title":       ac.Title,
				"description": ac.Description,
				"code":        ac.Code,
			})
			if uc.ssePublisher != nil {
				payload, mErr := json.Marshal(map[string]any{
					"type":        "achievement_unlocked",
					"id":          ac.ID,
					"title":       ac.Title,
					"description": ac.Description,
					"icon_key":    ac.IconKey,
				})
				if mErr == nil {
					if pErr := uc.ssePublisher.Publish(detached, userID.String(), payload); pErr != nil {
						logger.Warn("failed to publish achievement SSE", "code", ac.Code, "error", pErr)
					}
				}
			}
		}
	}

	return newlyUnlocked, nil
}

// DECISION: uses UTC because user timezone is not stored in the database.
func (uc *UserUsecase) triggerNightDancerAchievement(ctx context.Context, userID uuid.UUID) {
	if time.Now().UTC().Hour() >= 5 {
		return
	}
	detached := context.WithoutCancel(ctx)
	go func() {
		logger := log.GetLoggerFromContext(detached)
		all, err := uc.userRepo.GetAllAchievements(detached)
		if err != nil {
			logger.Warn("night_dancer: failed to get achievements", "user_id", userID, "error", err)
			return
		}
		for _, a := range all {
			if a.Code == "night_dancer" {
				if _, uErr := uc.userRepo.UnlockAchievement(detached, userID, a.ID); uErr != nil {
					logger.Warn("night_dancer: failed to unlock", "user_id", userID, "error", uErr)
				}
				return
			}
		}
	}()
}

func (uc *UserUsecase) TriggerTopDancerAchievement(ctx context.Context, userID uuid.UUID) {
	detached := context.WithoutCancel(ctx)
	go func() {
		logger := log.GetLoggerFromContext(detached)
		rank, err := uc.userRepo.GetUserGlobalRank(detached, userID)
		if err != nil {
			logger.Warn("top_dancer: failed to get global rank", "user_id", userID, "error", err)
			return
		}
		if rank > 10 {
			return
		}
		all, err := uc.userRepo.GetAllAchievements(detached)
		if err != nil {
			logger.Warn("top_dancer: failed to get achievements", "user_id", userID, "error", err)
			return
		}
		for _, a := range all {
			if a.Code == "top_dancer" {
				if _, uErr := uc.userRepo.UnlockAchievement(detached, userID, a.ID); uErr != nil {
					logger.Warn("top_dancer: failed to unlock", "user_id", userID, "error", uErr)
				}
				return
			}
		}
	}()
}

func (uc *UserUsecase) triggerSocialButterflyAchievement(ctx context.Context, opponentID uuid.UUID) {
	detached := context.WithoutCancel(ctx)
	go func() {
		logger := log.GetLoggerFromContext(detached)
		count, err := uc.userRepo.GetDistinctChallengersCount(detached, opponentID)
		if err != nil {
			logger.Warn("social_butterfly: failed to get distinct challengers count", "opponent_id", opponentID, "error", err)
			return
		}
		if count < 3 {
			return
		}
		all, err := uc.userRepo.GetAllAchievements(detached)
		if err != nil {
			logger.Warn("social_butterfly: failed to get achievements", "opponent_id", opponentID, "error", err)
			return
		}
		for _, a := range all {
			if a.Code == "social_butterfly" {
				if _, uErr := uc.userRepo.UnlockAchievement(detached, opponentID, a.ID); uErr != nil {
					logger.Warn("social_butterfly: failed to unlock", "opponent_id", opponentID, "error", uErr)
				}
				return
			}
		}
	}()
}

func (uc *UserUsecase) GetAllAchievements(ctx context.Context) ([]models.Achievement, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	all, err := uc.userRepo.GetAllAchievements(ctx)
	if err != nil {
		logger.Warn("failed to get all achievements", "error", err)
		return nil, err
	}
	return all, nil
}

func (uc *UserUsecase) getAvgUnlocked(ctx context.Context) float64 {
	avgAchievementsCacheInstance.mu.Lock()
	defer avgAchievementsCacheInstance.mu.Unlock()
	if avgAchievementsCacheInstance.value > 0 && time.Since(avgAchievementsCacheInstance.updatedAt) < time.Hour {
		return avgAchievementsCacheInstance.value
	}
	avg, err := uc.userRepo.GetAvgAchievementUnlockCount(ctx)
	if err != nil || avg <= 0 {
		return 1 // safe fallback: avoid division by zero
	}
	avgAchievementsCacheInstance.value = avg
	avgAchievementsCacheInstance.updatedAt = time.Now()
	return avg
}

func (uc *UserUsecase) GetUserAchievements(ctx context.Context, userID uuid.UUID) (*models.AchievementsWithMeta, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	allAchievements, err := uc.userRepo.GetAllAchievements(ctx)
	if err != nil {
		logger.Warn("failed to get all achievements", "error", err)
		return nil, err
	}

	userUnlocked, err := uc.userRepo.GetUserAchievements(ctx, userID)
	if err != nil {
		logger.Warn("failed to get user achievements", "error", err)
		return nil, err
	}

	unlockedMap := make(map[int]time.Time, len(userUnlocked))
	for _, ua := range userUnlocked {
		unlockedMap[ua.ID] = ua.UnlockedAt
	}

	result := make([]models.AchievementWithStatus, 0, len(allAchievements))
	for _, a := range allAchievements {
		item := models.AchievementWithStatus{Achievement: a}
		if t, ok := unlockedMap[a.ID]; ok {
			item.Unlocked = true
			item.UnlockedAt = &t
		}
		result = append(result, item)
	}

	unlockedCount := len(userUnlocked)
	avg := uc.getAvgUnlocked(ctx)
	percentile := math.Min(float64(unlockedCount)/avg*50, 99)

	return &models.AchievementsWithMeta{
		Achievements:  result,
		UnlockedCount: unlockedCount,
		TotalCount:    len(allAchievements),
		Percentile:    percentile,
	}, nil
}
