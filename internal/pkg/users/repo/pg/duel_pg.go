package repo

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v4"
	uuid "github.com/satori/go.uuid"
)

func (u *UserRepository) CreateDuel(ctx context.Context, mode string, challengerID, opponentID uuid.UUID, danceID string, expiresAt time.Time, token string) (*models.DuelWithUsers, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var duelID uuid.UUID
	var inviteToken string
	err := appmetrics.ObserveDBQuery("create_duel", func() error {
		return u.db.QueryRow(ctx, CreateDuelQuery, mode, challengerID, opponentID, danceID, expiresAt, token).
			Scan(&duelID, &inviteToken)
	})
	if err != nil {
		logger.Error("failed to create duel: " + err.Error())
		return nil, users.ErrorInternalServerError
	}

	return u.GetDuelByID(ctx, duelID)
}

func (u *UserRepository) GetDuelByID(ctx context.Context, duelID uuid.UUID) (*models.DuelWithUsers, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var d models.DuelWithUsers
	err := appmetrics.ObserveDBQuery("get_duel_by_id", func() error {
		return u.db.QueryRow(ctx, GetDuelByIDQuery, duelID).Scan(
			&d.ID,
			&d.Mode,
			&d.ChallengerID,
			&d.OpponentID,
			&d.DanceID,
			&d.Status,
			&d.ChallengerAttemptID,
			&d.OpponentAttemptID,
			&d.ChallengerScore,
			&d.OpponentScore,
			&d.WinnerID,
			&d.ExpiresAt,
			&d.CreatedAt,
			&d.CompletedAt,
			&d.IsPublic,
			&d.ChallengerLogin,
			&d.ChallengerAvatar,
			&d.OpponentLogin,
			&d.OpponentAvatar,
			&d.DanceTitle,
			&d.InviteToken,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, users.ErrorNotFound
		}
		logger.Error("failed to get duel by id: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return &d, nil
}

func (u *UserRepository) GetDuelsByUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.DuelWithUsers, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_duels_by_user", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetDuelsByUserQuery, userID, limit, offset)
		return e
	})
	if err != nil {
		logger.Error("failed to get duels by user: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var result []models.DuelWithUsers
	for rows.Next() {
		var d models.DuelWithUsers
		if err := rows.Scan(
			&d.ID,
			&d.Mode,
			&d.ChallengerID,
			&d.OpponentID,
			&d.DanceID,
			&d.Status,
			&d.ChallengerAttemptID,
			&d.OpponentAttemptID,
			&d.ChallengerScore,
			&d.OpponentScore,
			&d.WinnerID,
			&d.ExpiresAt,
			&d.CreatedAt,
			&d.CompletedAt,
			&d.IsPublic,
			&d.ChallengerLogin,
			&d.ChallengerAvatar,
			&d.OpponentLogin,
			&d.OpponentAvatar,
			&d.DanceTitle,
			&d.InviteToken,
		); err != nil {
			logger.Error("failed to scan duel row: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDuelsByUser: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) GetPublicDuels(ctx context.Context, limit, offset int) ([]models.DuelWithUsers, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_public_duels", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetPublicDuelsQuery, limit, offset)
		return e
	})
	if err != nil {
		logger.Error("failed to get public duels: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var result []models.DuelWithUsers
	for rows.Next() {
		var d models.DuelWithUsers
		if err := rows.Scan(
			&d.ID,
			&d.Mode,
			&d.ChallengerID,
			&d.OpponentID,
			&d.DanceID,
			&d.Status,
			&d.ChallengerAttemptID,
			&d.OpponentAttemptID,
			&d.ChallengerScore,
			&d.OpponentScore,
			&d.WinnerID,
			&d.ExpiresAt,
			&d.CreatedAt,
			&d.CompletedAt,
			&d.IsPublic,
			&d.ChallengerLogin,
			&d.ChallengerAvatar,
			&d.OpponentLogin,
			&d.OpponentAvatar,
			&d.DanceTitle,
			&d.InviteToken,
		); err != nil {
			logger.Error("failed to scan public duel row: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetPublicDuels: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) UpdateDuelStatus(ctx context.Context, duelID uuid.UUID, status string, challengerAttemptID, opponentAttemptID *uuid.UUID, challengerScore, opponentScore *float64, winnerID *uuid.UUID, completedAt *time.Time, isSubmitChallenger *bool, publicConsent bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var rowsAffected int64
	err := appmetrics.ObserveDBQuery("update_duel_status", func() error {
		tag, e := u.db.Exec(ctx, UpdateDuelStatusQuery,
			status,
			challengerAttemptID,
			challengerScore,
			opponentAttemptID,
			opponentScore,
			winnerID,
			completedAt,
			duelID,
			isSubmitChallenger,
			publicConsent,
		)
		if e == nil {
			rowsAffected = tag.RowsAffected()
		}
		return e
	})
	if err != nil {
		logger.Error("failed to update duel status: " + err.Error())
		return users.ErrorInternalServerError
	}
	if rowsAffected == 0 {
		logger.Warn("update_duel_status affected 0 rows", "duel_id", duelID, "attempted_status", status, "is_submit_challenger", isSubmitChallenger)
		return users.ErrorConflict
	}
	return nil
}

func (u *UserRepository) ExpireDuels(ctx context.Context) ([]uuid.UUID, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("expire_duels", func() error {
		var e error
		rows, e = u.db.Query(ctx, ExpireDuelsQuery)
		return e
	})
	if err != nil {
		logger.Error("failed to expire duels: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			logger.Error("failed to scan expired duel id: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in ExpireDuels: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return ids, nil
}

func (u *UserRepository) CreateDuelNotification(ctx context.Context, toUserID uuid.UUID, fromUserID *uuid.UUID, notifType, duelID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var fromArg interface{}
	if fromUserID != nil {
		fromArg = *fromUserID
	}
	err := appmetrics.ObserveDBQuery("create_duel_notification", func() error {
		_, e := u.db.Exec(ctx, CreateDuelNotificationQuery, toUserID, notifType, duelID, fromArg)
		return e
	})
	if err != nil {
		logger.Error("failed to create duel notification: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetBatchDuelParticipants(ctx context.Context, duelIDs []uuid.UUID) ([]models.DuelParticipants, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if len(duelIDs) == 0 {
		return nil, nil
	}

	raw := make([]string, len(duelIDs))
	for i, id := range duelIDs {
		raw[i] = id.String()
	}

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_batch_duel_participants", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetBatchDuelParticipantsQuery, raw)
		return e
	})
	if err != nil {
		logger.Error("failed to get batch duel participants: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var result []models.DuelParticipants
	for rows.Next() {
		var p models.DuelParticipants
		if err := rows.Scan(&p.ID, &p.ChallengerID, &p.OpponentID); err != nil {
			logger.Error("failed to scan duel participants row: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetBatchDuelParticipants: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) GetDistinctChallengersCount(ctx context.Context, opponentID uuid.UUID) (int, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var count int
	err := appmetrics.ObserveDBQuery("get_distinct_challengers_count", func() error {
		return u.db.QueryRow(ctx, GetDistinctChallengersCountQuery, opponentID).Scan(&count)
	})
	if err != nil {
		logger.Error("failed to get distinct challengers count: " + err.Error())
		return 0, users.ErrorInternalServerError
	}
	return count, nil
}

func (u *UserRepository) GetRandomDance(ctx context.Context) (string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var danceID string
	err := appmetrics.ObserveDBQuery("get_random_dance", func() error {
		return u.db.QueryRow(ctx, GetRandomDanceQuery).Scan(&danceID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", users.ErrorNotFound
		}
		logger.Error("failed to get random dance: " + err.Error())
		return "", users.ErrorInternalServerError
	}
	return danceID, nil
}

func (u *UserRepository) GetDuelStats(ctx context.Context, userID uuid.UUID) (*models.DuelStats, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var s models.DuelStats
	err := appmetrics.ObserveDBQuery("get_duel_stats", func() error {
		return u.db.QueryRow(ctx, GetDuelStatsQuery, userID).Scan(&s.Total, &s.Wins, &s.AvgScore)
	})
	if err != nil {
		logger.Error("failed to get duel stats: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	if s.Total > 0 {
		s.WinRate = float64(s.Wins) / float64(s.Total) * 100
	}
	return &s, nil
}
