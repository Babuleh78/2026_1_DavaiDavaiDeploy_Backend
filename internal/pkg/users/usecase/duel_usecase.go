package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/kafka"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	uuid "github.com/satori/go.uuid"
)

func (uc *UserUsecase) notifyTelegram(ctx context.Context, userID uuid.UUID, notifType string, payload map[string]any) {
	tid, err := uc.userRepo.GetUserTelegramID(ctx, userID)
	if err != nil || tid == nil {
		return
	}
	if pushErr := uc.BotPushNotification(ctx, *tid, notifType, payload); pushErr != nil {
		log.GetLoggerFromContext(ctx).Warn("notifyTelegram push failed", "type", notifType, "error", pushErr)
	}
}

func (uc *UserUsecase) sendDuelNotif(ctx context.Context, toUserID uuid.UUID, fromUserID *uuid.UUID, notifType, duelID string, tgPayload map[string]any) {
	logger := log.GetLoggerFromContext(ctx)
	if uc.notifSender != nil {
		go func() {
			callCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := uc.notifSender.SendDuelNotification(callCtx, toUserID, fromUserID, notifType, duelID, tgPayload); err != nil {
				logger.Warn("notifSender.SendDuelNotification failed", "type", notifType, "error", err)
			}
		}()
		return
	}
	if uc.kafkaProducer != nil {
		fromStr := ""
		if fromUserID != nil {
			fromStr = fromUserID.String()
		}
		msg := map[string]any{
			"to_user_id":       toUserID.String(),
			"from_user_id":     fromStr,
			"type":             notifType,
			"duel_id":          duelID,
			"telegram_payload": tgPayload,
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			logger.Error("failed to marshal notification payload", "type", notifType, "error", err)
			return
		}
		uc.kafkaProducer.PublishAsync(ctx, kafka.TopicNotificationSend, toUserID.String(), payload, func(err error) {
			logger.Warn("kafka publish TopicNotificationSend failed", "type", notifType, "error", err)
		})
		return
	}
	if notifErr := uc.userRepo.CreateDuelNotification(ctx, toUserID, fromUserID, notifType, duelID); notifErr != nil {
		logger.Warn("CreateDuelNotification failed", "type", notifType, "error", notifErr)
	}
	if len(tgPayload) > 0 {
		go uc.notifyTelegram(ctx, toUserID, notifType, tgPayload)
	}
}

func (uc *UserUsecase) CreateDuel(ctx context.Context, challengerID uuid.UUID, opponentID uuid.UUID, mode string, danceID string) (*models.DuelWithUsers, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if challengerID == opponentID {
		return nil, users.ErrorBadRequest
	}

	if mode != models.DuelModeSingleDance && mode != models.DuelModeRandomDance {
		return nil, users.ErrorBadRequest
	}

	if _, err := uc.userRepo.GetUserByID(ctx, opponentID); err != nil {
		return nil, users.ErrorNotFound
	}

	switch mode {
	case models.DuelModeSingleDance:
		if danceID == "" {
			return nil, users.ErrorBadRequest
		}
		status, err := uc.userRepo.GetDanceStatus(ctx, danceID)
		if err != nil {
			return nil, users.ErrorNotFound
		}
		if status != "published" {
			return nil, users.ErrorNotFound
		}
	case models.DuelModeRandomDance:
		randomID, err := uc.userRepo.GetRandomDance(ctx)
		if err != nil {
			logger.Warn("failed to get random dance for duel", "error", err)
			return nil, users.ErrorInternalServerError
		}
		danceID = randomID
	}

	if exists, err := uc.userRepo.HasOpenDuel(ctx, challengerID, opponentID, danceID); err != nil {
		logger.Warn("failed to check existing duel", "error", err)
		return nil, users.ErrorInternalServerError
	} else if exists {
		return nil, users.ErrorConflict
	}

	token := uuid.NewV4().String()
	expiresAt := time.Now().Add(24 * time.Hour)

	duel, err := uc.userRepo.CreateDuel(ctx, mode, challengerID, opponentID, danceID, expiresAt, token)
	if err != nil {
		logger.Warn("failed to create duel", "error", err)
		return nil, err
	}

	uc.sendDuelNotif(ctx, opponentID, &challengerID, models.NotifDuelChallengeReceived, duel.ID.String(), map[string]any{
		"challenger_name": duel.ChallengerLogin,
		"dance_title":     duel.DanceTitle,
		"duel_id":         duel.ID.String(),
	})

	uc.triggerSocialButterflyAchievement(ctx, opponentID)

	return duel, nil
}

func (uc *UserUsecase) GetActiveDuelsForUserDance(ctx context.Context, userID uuid.UUID, danceID string) ([]models.ActiveDuelForDance, error) {
	return uc.userRepo.GetActiveDuelsForUserDance(ctx, userID, danceID)
}

func (uc *UserUsecase) SubmitAttemptToActiveDuels(ctx context.Context, userID uuid.UUID, danceID string, attemptID uuid.UUID, publicConsent bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	duels, err := uc.userRepo.GetActiveDuelsForUserDance(ctx, userID, danceID)
	if err != nil {
		return err
	}

	for _, d := range duels {
		if subErr := uc.SubmitDuelAttempt(ctx, userID, d.DuelID, attemptID, publicConsent); subErr != nil {
			logger.Warn("fan-out duel submit failed", "duel_id", d.DuelID.String(), "error", subErr)
		}
	}
	return nil
}

func (uc *UserUsecase) AcceptDuel(ctx context.Context, userID uuid.UUID, duelID uuid.UUID) (*models.DuelWithUsers, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	duel, err := uc.userRepo.GetDuelByID(ctx, duelID)
	if err != nil {
		return nil, users.ErrorNotFound
	}

	if duel.OpponentID != userID {
		return nil, users.ErrorForbidden
	}
	if duel.Status != models.DuelStatusPending {
		return nil, users.ErrorBadRequest
	}
	if time.Now().After(duel.ExpiresAt) {
		return nil, users.ErrorBadRequest
	}

	if err := uc.userRepo.UpdateDuelStatus(ctx, duelID, models.DuelStatusActive, nil, nil, nil, nil, nil, nil, nil, false); err != nil {
		logger.Warn("failed to accept duel", "duel_id", duelID, "error", err)
		return nil, err
	}

	uc.sendDuelNotif(ctx, duel.ChallengerID, &userID, "duel_accepted", duel.ID.String(), map[string]any{
		"opponent_name": duel.OpponentLogin,
		"dance_title":   duel.DanceTitle,
		"duel_id":       duel.ID.String(),
	})

	updated, err := uc.userRepo.GetDuelByID(ctx, duelID)
	if err != nil {
		logger.Warn("failed to re-fetch duel after accept", "error", err)
		return nil, err
	}
	return updated, nil
}

func (uc *UserUsecase) DeclineDuel(ctx context.Context, userID uuid.UUID, duelID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	duel, err := uc.userRepo.GetDuelByID(ctx, duelID)
	if err != nil {
		return users.ErrorNotFound
	}

	if duel.OpponentID != userID {
		return users.ErrorForbidden
	}
	if duel.Status != models.DuelStatusPending {
		return users.ErrorBadRequest
	}

	if err := uc.userRepo.UpdateDuelStatus(ctx, duelID, models.DuelStatusDeclined, nil, nil, nil, nil, nil, nil, nil, false); err != nil {
		logger.Warn("failed to decline duel", "duel_id", duelID, "error", err)
		return err
	}

	uc.sendDuelNotif(ctx, duel.ChallengerID, &userID, "duel_declined", duel.ID.String(), map[string]any{
		"opponent_name": duel.OpponentLogin,
		"duel_id":       duel.ID.String(),
	})

	return nil
}

func (uc *UserUsecase) SubmitDuelAttempt(ctx context.Context, userID uuid.UUID, duelID uuid.UUID, attemptID uuid.UUID, publicConsent bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	duel, err := uc.userRepo.GetDuelByID(ctx, duelID)
	if err != nil {
		return users.ErrorNotFound
	}

	owner, ownerErr := uc.userRepo.GetAttemptOwner(ctx, attemptID.String())
	if ownerErr != nil || owner == nil || *owner != userID {
		return users.ErrorForbidden
	}

	s3Key := fmt.Sprintf("users/%s/%s/comparison_result.json", userID.String(), attemptID.String())
	data, dlErr := uc.storageRepo.DownloadFile(ctx, s3Key)
	if dlErr != nil {
		logger.Warn("failed to download comparison_result.json for duel", "error", dlErr, "key", s3Key)
		return users.ErrorNotFound
	}
	var stored models.CompareStatusResult
	if err := json.Unmarshal(data, &stored); err != nil {
		logger.Error("failed to unmarshal comparison_result.json for duel", "error", err)
		return users.ErrorInternalServerError
	}
	score := stored.ComparisonScore

	if stored.DanceID != "" && stored.DanceID != duel.DanceID {
		logger.Warn("attempt dance_id does not match duel dance_id", "attempt_dance_id", stored.DanceID, "duel_dance_id", duel.DanceID)
		return users.ErrorBadRequest
	}
	validStatuses := map[string]bool{
		models.DuelStatusActive:         true,
		models.DuelStatusChallengerDone: true,
		models.DuelStatusOpponentDone:   true,
	}
	if !validStatuses[duel.Status] {
		return users.ErrorBadRequest
	}

	isChallenger := duel.ChallengerID == userID
	isOpponent := duel.OpponentID == userID
	if !isChallenger && !isOpponent {
		return users.ErrorForbidden
	}

	if isChallenger && duel.ChallengerAttemptID != nil {
		return users.ErrorBadRequest
	}
	if isOpponent && duel.OpponentAttemptID != nil {
		return users.ErrorBadRequest
	}

	var (
		newStatus           string
		challengerAttemptID *uuid.UUID
		opponentAttemptID   *uuid.UUID
		challengerScore     *float64
		opponentScore       *float64
		winnerID            *uuid.UUID
		completedAt         *time.Time
	)

	if isChallenger {
		challengerAttemptID = &attemptID
		challengerScore = &score

		if duel.Status == models.DuelStatusOpponentDone {
			newStatus = models.DuelStatusCompleted
			now := time.Now()
			completedAt = &now
			opponentAttemptID = duel.OpponentAttemptID
			opponentScore = duel.OpponentScore
			winner := duel.ChallengerID
			if duel.OpponentScore != nil && *duel.OpponentScore > score {
				winner = duel.OpponentID
			}
			winnerID = &winner
		} else {
			newStatus = models.DuelStatusChallengerDone
		}
	} else {
		opponentAttemptID = &attemptID
		opponentScore = &score

		if duel.Status == models.DuelStatusChallengerDone {
			newStatus = models.DuelStatusCompleted
			now := time.Now()
			completedAt = &now
			challengerAttemptID = duel.ChallengerAttemptID
			challengerScore = duel.ChallengerScore
			winner := duel.OpponentID
			if duel.ChallengerScore != nil && *duel.ChallengerScore > score {
				winner = duel.ChallengerID
			}
			winnerID = &winner
		} else {
			newStatus = models.DuelStatusOpponentDone
		}
	}

	if err := uc.userRepo.EnsureSavedAttemptForDuel(ctx, userID, attemptID.String(), duel.DanceID, score, !publicConsent); err != nil {
		logger.Warn("failed to ensure saved attempt for duel", "duel_id", duelID, "error", err)
		return err
	}

	if err := uc.userRepo.UpdateDuelStatus(ctx, duelID, newStatus, challengerAttemptID, opponentAttemptID, challengerScore, opponentScore, winnerID, completedAt, &isChallenger, publicConsent); err != nil {
		logger.Warn("failed to submit duel attempt", "duel_id", duelID, "error", err)
		return err
	}

	if newStatus == models.DuelStatusCompleted {
		var finalChallengerScore, finalOpponentScore float64
		if challengerScore != nil {
			finalChallengerScore = *challengerScore
		}
		if opponentScore != nil {
			finalOpponentScore = *opponentScore
		}
		uc.sendDuelNotif(ctx, duel.ChallengerID, nil, models.NotifDuelCompleted, duel.ID.String(), map[string]any{
			"your_score":     finalChallengerScore,
			"opponent_score": finalOpponentScore,
			"duel_id":        duel.ID.String(),
		})
		uc.sendDuelNotif(ctx, duel.OpponentID, nil, models.NotifDuelCompleted, duel.ID.String(), map[string]any{
			"your_score":     finalOpponentScore,
			"opponent_score": finalChallengerScore,
			"duel_id":        duel.ID.String(),
		})

		uc.publishDuelCompleted(ctx, duel.ChallengerID, duel.OpponentID, winnerID, duel.ID)
	} else {
		if isChallenger {
			uc.sendDuelNotif(ctx, duel.OpponentID, &userID, models.NotifDuelChallengerDone, duel.ID.String(), nil)
		} else {
			uc.sendDuelNotif(ctx, duel.ChallengerID, &userID, models.NotifDuelOpponentDone, duel.ID.String(), nil)
		}
	}

	return nil
}

func (uc *UserUsecase) GetPublicDuels(ctx context.Context, limit, offset int) (*models.DuelHistoryResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if limit <= 0 {
		limit = 20
	}

	duels, err := uc.userRepo.GetPublicDuels(ctx, limit+1, offset)
	if err != nil {
		logger.Warn("failed to get public duels", "error", err)
		return nil, err
	}

	hasMore := len(duels) > limit
	if hasMore {
		duels = duels[:limit]
	}

	if duels == nil {
		duels = []models.DuelWithUsers{}
	}

	page := 1
	if limit > 0 {
		page = offset/limit + 1
	}

	return &models.DuelHistoryResponse{
		Duels: duels,
		Pagination: models.PaginationMeta{
			Page:    page,
			Limit:   limit,
			HasMore: hasMore,
		},
	}, nil
}

func (uc *UserUsecase) GetDuelHistory(ctx context.Context, userID uuid.UUID, limit, offset int) (*models.DuelHistoryResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if limit <= 0 {
		limit = 20
	}

	duels, err := uc.userRepo.GetDuelsByUser(ctx, userID, limit+1, offset)
	if err != nil {
		logger.Warn("failed to get duel history", "error", err)
		return nil, err
	}

	hasMore := len(duels) > limit
	if hasMore {
		duels = duels[:limit]
	}

	for i := range duels {
		if duels[i].Status != models.DuelStatusCompleted {
			duels[i].ChallengerScore = nil
			duels[i].OpponentScore = nil
			duels[i].WinnerID = nil
		}
	}

	if duels == nil {
		duels = []models.DuelWithUsers{}
	}

	page := 1
	if limit > 0 {
		page = offset/limit + 1
	}

	return &models.DuelHistoryResponse{
		Duels: duels,
		Pagination: models.PaginationMeta{
			Page:    page,
			Limit:   limit,
			HasMore: hasMore,
		},
	}, nil
}

func (uc *UserUsecase) GetDuelByID(ctx context.Context, userID uuid.UUID, duelID uuid.UUID) (*models.DuelWithUsers, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	duel, err := uc.userRepo.GetDuelByID(ctx, duelID)
	if err != nil {
		logger.Warn("failed to get duel by id", "duel_id", duelID, "error", err)
		return nil, users.ErrorNotFound
	}

	if duel.ChallengerID != userID && duel.OpponentID != userID {
		return nil, users.ErrorForbidden
	}

	if duel.Status != models.DuelStatusCompleted {
		duel.ChallengerScore = nil
		duel.OpponentScore = nil
		duel.WinnerID = nil
	}

	return duel, nil
}

func (uc *UserUsecase) RunExpireJob(ctx context.Context) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	expiredIDs, err := uc.userRepo.ExpireDuels(ctx)
	if err != nil {
		logger.Warn("expire duels job failed", "error", err)
		return err
	}

	if len(expiredIDs) == 0 {
		return nil
	}

	logger.Info("expired duels", "count", len(expiredIDs))

	participants, err := uc.userRepo.GetBatchDuelParticipants(ctx, expiredIDs)
	if err != nil {
		logger.Error("failed to get participants for expired duels", "error", err)
		return err
	}

	for _, p := range participants {
		duelIDStr := p.ID.String()
		uc.sendDuelNotif(ctx, p.ChallengerID, nil, models.NotifDuelExpired, duelIDStr, nil)
		uc.sendDuelNotif(ctx, p.OpponentID, nil, models.NotifDuelExpired, duelIDStr, nil)
	}

	return nil
}

func (uc *UserUsecase) GetDuelStats(ctx context.Context, userID uuid.UUID) (*models.DuelStats, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	stats, err := uc.userRepo.GetDuelStats(ctx, userID)
	if err != nil {
		logger.Warn("failed to get duel stats", "user_id", userID, "error", err)
		return nil, err
	}
	return stats, nil
}

func (uc *UserUsecase) publishDuelCompleted(ctx context.Context, challengerID, opponentID uuid.UUID, winnerID *uuid.UUID, duelID uuid.UUID) {
	logger := log.GetLoggerFromContext(ctx)
	if uc.kafkaProducer != nil {
		winnerStr := ""
		if winnerID != nil {
			winnerStr = winnerID.String()
		}
		payload, mErr := json.Marshal(map[string]string{
			"challenger_id": challengerID.String(),
			"opponent_id":   opponentID.String(),
			"winner_id":     winnerStr,
			"duel_id":       duelID.String(),
		})
		if mErr != nil {
			logger.Warn("failed to marshal duel.completed payload", "error", mErr)
			return
		}
		uc.kafkaProducer.PublishAsync(ctx, kafka.TopicDuelCompleted, duelID.String(), payload, func(err error) {
			logger.Warn("kafka publish TopicDuelCompleted failed", "duel_id", duelID, "error", err)
		})
		return
	}
	go uc.triggerAchievementCheck(ctx, challengerID)
	go uc.triggerAchievementCheck(ctx, opponentID)
}
