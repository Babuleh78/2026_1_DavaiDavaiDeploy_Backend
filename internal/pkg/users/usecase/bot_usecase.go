package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	uuid "github.com/satori/go.uuid"
)

const telegramLinkCodeTTL = 10 * time.Minute

func (u *UserUsecase) BotGetUserByTelegramID(ctx context.Context, telegramID int64) (*models.User, error) {
	user, err := u.userRepo.GetUserByTelegramID(ctx, telegramID)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (u *UserUsecase) GenerateTelegramLinkCode(ctx context.Context, userID uuid.UUID) (string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	buf := make([]byte, 5) // 40 bits -> 10 hex chars
	if _, err := rand.Read(buf); err != nil {
		logger.Error("rand.Read failed", "error", err)
		return "", err
	}
	code := hex.EncodeToString(buf)

	if err := u.userRepo.SetTelegramLinkCode(ctx, userID, code, time.Now().Add(telegramLinkCodeTTL)); err != nil {
		return "", err
	}
	return code, nil
}

func (u *UserUsecase) LinkTelegramAccount(ctx context.Context, code string, telegramID int64) (*models.BotAuthResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	code = strings.ToLower(strings.TrimSpace(code))
	if code == "" {
		return &models.BotAuthResponse{OK: false, Error: "empty code"}, nil
	}

	user, err := u.userRepo.LinkTelegramByCode(ctx, code, telegramID)
	if err != nil {
		if errors.Is(err, users.ErrorNotFound) {
			return &models.BotAuthResponse{OK: false, Error: "invalid or expired code"}, nil
		}
		logger.Error("LinkTelegramByCode failed", "error", err)
		return nil, err
	}

	return &models.BotAuthResponse{
		OK:       true,
		UserID:   user.ID.String(),
		Username: user.Login,
	}, nil
}

func (u *UserUsecase) GetUserBotStats(ctx context.Context, userID uuid.UUID) (*models.BotUserStats, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	stats, err := u.userRepo.GetUserStatsForAchievements(ctx, userID)
	if err != nil {
		logger.Warn("GetUserBotStats: failed to get user stats", "user_id", userID, "error", err)
		return nil, err
	}

	all, err := u.userRepo.GetAllAchievements(ctx)
	if err != nil {
		logger.Warn("GetUserBotStats: failed to get all achievements", "user_id", userID, "error", err)
		return nil, err
	}

	unlocked, err := u.userRepo.GetUserAchievements(ctx, userID)
	if err != nil {
		logger.Warn("GetUserBotStats: failed to get user achievements", "user_id", userID, "error", err)
		return nil, err
	}

	return &models.BotUserStats{
		AttemptCount:         stats.AttemptCount,
		MaxScore:             stats.MaxScore,
		DuelCount:            stats.DuelCount,
		DuelWinCount:         stats.DuelWinCount,
		AchievementsUnlocked: len(unlocked),
		AchievementsTotal:    len(all),
	}, nil
}

func (u *UserUsecase) BotPushNotification(ctx context.Context, telegramID int64, notifType string, payload map[string]any) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if u.botNotifier == nil {
		logger.Warn("botNotifier not configured, dropping notification", "type", notifType, "telegram_id", telegramID)
		return nil
	}

	msg := map[string]any{
		"telegram_id": telegramID,
		"type":        notifType,
		"payload":     payload,
	}
	data, err := json.Marshal(msg)
	if err != nil {
		logger.Error("BotPushNotification marshal failed", "error", err)
		return nil
	}

	return u.botNotifier.Push(ctx, data)
}
