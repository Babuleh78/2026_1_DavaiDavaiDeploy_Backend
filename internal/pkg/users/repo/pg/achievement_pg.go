package repo

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"log/slog"

	"github.com/jackc/pgx/v4"
	uuid "github.com/satori/go.uuid"
)

func (u *UserRepository) GetAllAchievements(ctx context.Context) ([]models.Achievement, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_all_achievements", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetAllAchievementsQuery)
		return e
	})
	if err != nil {
		logger.Error("failed to get achievements", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var result []models.Achievement
	for rows.Next() {
		var a models.Achievement
		if err := rows.Scan(&a.ID, &a.Code, &a.Title, &a.Description, &a.IconKey, &a.Category, &a.Threshold); err != nil {
			logger.Error("failed to scan achievement", "error", err)
			return nil, users.ErrorInternalServerError
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetAllAchievements", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) GetUserAchievements(ctx context.Context, userID uuid.UUID) ([]models.UserAchievement, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_user_achievements", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetUserAchievementsQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to get user achievements", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var result []models.UserAchievement
	for rows.Next() {
		var a models.UserAchievement
		if err := rows.Scan(
			&a.ID, &a.Code, &a.Title, &a.Description,
			&a.IconKey, &a.Category, &a.Threshold, &a.UnlockedAt,
		); err != nil {
			logger.Error("failed to scan user achievement", "error", err)
			return nil, users.ErrorInternalServerError
		}
		result = append(result, a)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetUserAchievements", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) UnlockAchievement(ctx context.Context, userID uuid.UUID, achievementID int) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rowsAffected int64
	err := appmetrics.ObserveDBQuery("unlock_achievement", func() error {
		result, e := u.db.Exec(ctx, UnlockAchievementQuery, userID, achievementID)
		if e == nil {
			rowsAffected = result.RowsAffected()
		}
		return e
	})
	if err != nil {
		logger.Error("failed to unlock achievement", "error", err)
		return false, users.ErrorInternalServerError
	}
	return rowsAffected > 0, nil
}

func (u *UserRepository) GetAvgAchievementUnlockCount(ctx context.Context) (float64, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var avg float64
	err := appmetrics.ObserveDBQuery("get_avg_unlocked_achievements", func() error {
		return u.db.QueryRow(ctx, GetAvgUnlockedAchievementsQuery).Scan(&avg)
	})
	if err != nil {
		logger.Error("failed to get avg unlocked achievements", "error", err)
		return 0, users.ErrorInternalServerError
	}
	return avg, nil
}

func (u *UserRepository) GetUserStatsForAchievements(ctx context.Context, userID uuid.UUID) (*models.UserAchievementStats, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var s models.UserAchievementStats
	err := appmetrics.ObserveDBQuery("get_user_stats_for_achievements", func() error {
		return u.db.QueryRow(ctx, GetUserStatsForAchievementsQuery, userID).Scan(
			&s.TotalLikes,
			&s.MaxScore,
			&s.UploadCount,
			&s.AttemptCount,
			&s.DuelCount,
			&s.DuelWinCount,
			&s.UniqueDanceCount,
			&s.DuelWinStreak,
		)
	})
	if err != nil {
		logger.Error("failed to get user stats for achievements", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return &s, nil
}
