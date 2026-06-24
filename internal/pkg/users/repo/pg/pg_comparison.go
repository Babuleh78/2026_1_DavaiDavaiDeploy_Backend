package repo

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v4"
	uuid "github.com/satori/go.uuid"
	"log/slog"
	"math"
)

func (u *UserRepository) RecordDanceAttempt(ctx context.Context, danceID string, userID *uuid.UUID, attemptID string, score float64) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("record_dance_attempt", func() error {
		_, e := u.db.Exec(ctx, RecordDanceAttemptQuery, danceID, userID, attemptID, score)
		return e
	})
	if err != nil {
		logger.Error("failed to record dance attempt: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) CreateCompareTask(ctx context.Context, taskID, danceID, userDanceID, videoKey string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("create_compare_task", func() error {
		_, e := u.db.Exec(ctx, CreateCompareTaskQuery, taskID, danceID, userDanceID, videoKey)
		return e
	})
	if err != nil {
		logger.Error("failed to create compare task: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetCompareTask(ctx context.Context, taskID string) (models.CompareTask, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	task := models.CompareTask{TaskID: taskID}
	err := appmetrics.ObserveDBQuery("get_compare_task", func() error {
		return u.db.QueryRow(ctx, GetCompareTaskQuery, taskID).Scan(
			&task.DanceID, &task.UserDanceID, &task.VideoKey,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.CompareTask{}, users.ErrorNotFound
		}
		logger.Error("failed to get compare task: " + err.Error())
		return models.CompareTask{}, users.ErrorInternalServerError
	}
	return task, nil
}

func (u *UserRepository) MarkCompareTaskFinalized(ctx context.Context, taskID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rowsAffected int64
	err := appmetrics.ObserveDBQuery("mark_compare_task_finalized", func() error {
		tag, e := u.db.Exec(ctx, MarkCompareTaskFinalizedQuery, taskID)
		if e == nil {
			rowsAffected = tag.RowsAffected()
		}
		return e
	})
	if err != nil {
		logger.Error("failed to mark compare task finalized: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return rowsAffected == 1, nil
}

const minRatingsForCrowdDifficulty = 3

const popular7dExpr = "(SELECT COUNT(*) FROM dance_attempts pa WHERE pa.dance_id = d.id AND pa.created_at >= NOW() - INTERVAL '7 days') DESC NULLS LAST"

func (u *UserRepository) GetDanceLeaderboard(ctx context.Context, danceID string) ([]models.LeaderboardEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_dance_leaderboard", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetLeaderboardQuery, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to get leaderboard: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var entries []models.LeaderboardEntry
	for rows.Next() {
		var e models.LeaderboardEntry
		if err := rows.Scan(&e.Rank, &e.Login, &e.Score, &e.UserID, &e.Avatar); err != nil {
			logger.Error("failed to scan leaderboard row: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDanceLeaderboard: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	if entries == nil {
		entries = []models.LeaderboardEntry{}
	}
	return entries, nil
}

func (u *UserRepository) GetUserDanceRank(ctx context.Context, danceID string, userID uuid.UUID) (*models.LeaderboardEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var e models.LeaderboardEntry
	err := appmetrics.ObserveDBQuery("get_user_dance_rank", func() error {
		return u.db.QueryRow(ctx, GetUserDanceRankQuery, danceID, userID).Scan(&e.Rank, &e.Login, &e.Score, &e.Avatar)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.Error("failed to get user rank: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	e.UserID = userID
	e.IsMe = true
	return &e, nil
}

func (u *UserRepository) GetUserGlobalRank(ctx context.Context, userID uuid.UUID) (int, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rank int
	err := appmetrics.ObserveDBQuery("get_user_global_rank", func() error {
		return u.db.QueryRow(ctx, GetUserGlobalRankQuery, userID).Scan(&rank)
	})
	if err != nil {
		logger.Error("failed to get user global rank: " + err.Error())
		return 0, users.ErrorInternalServerError
	}
	return rank, nil
}

func (u *UserRepository) SaveAttempt(ctx context.Context, userID uuid.UUID, attemptID, danceID string, score float64, hasVideo bool, userName string, isPrivate bool, timingScore, amplitudeScore, poseScore float64) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("save_attempt", func() error {
		_, e := u.db.Exec(ctx, SaveAttemptQuery, attemptID, userID, danceID, score, hasVideo, userName, isPrivate, timingScore, amplitudeScore, poseScore)
		return e
	})
	if err != nil {
		logger.Error("failed to save attempt: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) EnsureSavedAttemptForDuel(ctx context.Context, userID uuid.UUID, attemptID, danceID string, score float64, isPrivate bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("ensure_saved_attempt_for_duel", func() error {
		_, e := u.db.Exec(ctx, EnsureSavedAttemptForDuelQuery, attemptID, userID, danceID, score, isPrivate)
		return e
	})
	if err != nil {
		logger.Error("failed to ensure saved attempt for duel: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetUserWeakSpots(ctx context.Context, userID uuid.UUID) (*models.WeakSpots, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var timing, amplitude, pose float64
	err := appmetrics.ObserveDBQuery("get_user_weak_spots", func() error {
		return u.db.QueryRow(ctx, GetUserWeakSpotsQuery, userID).Scan(&timing, &amplitude, &pose)
	})
	if err != nil {
		logger.Warn("get_user_weak_spots query error", "error", err)
		return nil, nil
	}
	if timing+amplitude+pose == 0 {
		return nil, nil
	}
	type metric struct {
		name string
		val  float64
	}
	metrics := []metric{{"timing", timing}, {"amplitude", amplitude}, {"pose", pose}}
	worst := metrics[0]
	for _, m := range metrics[1:] {
		if m.val < worst.val {
			worst = m
		}
	}
	suggestions := map[string]string{
		"timing":    "Поработай над синхронизацией с ритмом — попробуй отсчитывать такты",
		"amplitude": "Попробуй танцы с широкими движениями чтобы улучшить амплитуду",
		"pose":      "Обрати внимание на точность поз в ключевых кадрах",
	}
	return &models.WeakSpots{
		WorstMetric: worst.name,
		Avg:         math.Round(worst.val*10) / 10,
		Suggestion:  suggestions[worst.name],
	}, nil
}

func (u *UserRepository) IsSavedAttemptWithVideo(ctx context.Context, userID uuid.UUID, attemptID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var exists bool
	err := appmetrics.ObserveDBQuery("is_saved_attempt_with_video", func() error {
		return u.db.QueryRow(ctx, IsSavedAttemptWithVideoQuery, attemptID, userID).Scan(&exists)
	})
	if err != nil {
		logger.Error("failed to check saved attempt video flag: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return exists, nil
}

func (u *UserRepository) SavedAttemptExists(ctx context.Context, userID uuid.UUID, attemptID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var exists bool
	err := appmetrics.ObserveDBQuery("saved_attempt_exists", func() error {
		return u.db.QueryRow(ctx, SavedAttemptExistsQuery, attemptID, userID).Scan(&exists)
	})
	if err != nil {
		logger.Error("failed to check saved attempt existence: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return exists, nil
}

func (u *UserRepository) IsAttemptPrivate(ctx context.Context, attemptID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var isPrivate bool
	err := appmetrics.ObserveDBQuery("is_attempt_private", func() error {
		return u.db.QueryRow(ctx, IsAttemptPrivateQuery, attemptID).Scan(&isPrivate)
	})
	if err != nil {
		logger.Error("failed to check attempt privacy: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return isPrivate, nil
}

func (u *UserRepository) UnsaveAttempt(ctx context.Context, userID uuid.UUID, attemptID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("unsave_attempt", func() error {
		_, e := u.db.Exec(ctx, UnsaveAttemptQuery, attemptID, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to unsave attempt: " + err.Error())
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetSavedAttempts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.SavedAttemptItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_saved_attempts", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetSavedAttemptsQuery, userID, limit, offset)
		return e
	})
	if err != nil {
		logger.Error("failed to query saved attempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.SavedAttemptItem{}
	for rows.Next() {
		var it models.SavedAttemptItem
		if err := rows.Scan(&it.UserDanceID, &it.DanceID, &it.DanceTitle, &it.Score, &it.SavedAt, &it.HasVideo, &it.UserName, &it.IsPrivate); err != nil {
			logger.Error("failed to scan saved attempt: " + err.Error())
			return nil, users.ErrorInternalServerError
		}

		it.ReferenceVideoKey = fmt.Sprintf("results/%s/video.mp4", it.DanceID)
		it.UserAnimationKey = fmt.Sprintf("users/%s/%s/user_animation.glb", userID.String(), it.UserDanceID)
		it.UserSkeletonKey = fmt.Sprintf("users/%s/%s/skeleton.json", userID.String(), it.UserDanceID)
		if it.HasVideo {
			it.UserVideoKey = fmt.Sprintf("users/%s/%s/video.mp4", userID.String(), it.UserDanceID)
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetSavedAttempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) GetUserAttempts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.UserAttemptItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_user_attempts", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetUserAttemptsQuery, userID, limit, offset)
		return e
	})
	if err != nil {
		logger.Error("failed to query user attempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.UserAttemptItem{}
	for rows.Next() {
		var it models.UserAttemptItem
		if err := rows.Scan(&it.AttemptID, &it.DanceID, &it.DanceTitle, &it.Score, &it.CreatedAt, &it.IsSaved, &it.IsOpen, &it.UserName, &it.Rank, &it.TotalDancers); err != nil {
			logger.Error("failed to scan user attempt: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetUserAttempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) GetDanceProgress(ctx context.Context, userID uuid.UUID, danceID string) ([]models.DanceProgressEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_dance_progress", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetDanceProgressQuery, userID, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to query dance progress: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.DanceProgressEntry{}
	for rows.Next() {
		var it models.DanceProgressEntry
		if err := rows.Scan(&it.AttemptID, &it.Score, &it.CreatedAt); err != nil {
			logger.Error("failed to scan dance progress entry: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetDanceProgress: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) GetUserActivity(ctx context.Context, userID uuid.UUID) ([]models.ActivityEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_user_activity", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetUserActivityQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query user activity: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.ActivityEntry{}
	for rows.Next() {
		var it models.ActivityEntry
		if err := rows.Scan(&it.Day, &it.Count); err != nil {
			logger.Error("failed to scan activity entry: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetUserActivity: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) GetMostImprovedDance(ctx context.Context, userID uuid.UUID) (*models.MostImprovedDance, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var it models.MostImprovedDance
	err := appmetrics.ObserveDBQuery("get_most_improved_dance", func() error {
		return u.db.QueryRow(ctx, GetMostImprovedDanceQuery, userID).
			Scan(&it.DanceID, &it.Title, &it.FirstScore, &it.LastScore, &it.Delta)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.Error("failed to query most improved dance: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return &it, nil
}

func (u *UserRepository) GetReelsAttempts(ctx context.Context, limit, offset int) ([]models.ReelsAttemptItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_reels_attempts", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetReelsAttemptsQuery, limit, offset)
		return e
	})
	if err != nil {
		logger.Error("failed to query reels attempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.ReelsAttemptItem{}
	for rows.Next() {
		var it models.ReelsAttemptItem
		if err := rows.Scan(&it.AttemptID, &it.DanceID, &it.DanceTitle, &it.UserID, &it.UserLogin, &it.UserAvatar, &it.Score); err != nil {
			logger.Error("failed to scan reels attempt: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		it.VideoKey = fmt.Sprintf("users/%s/%s/video.mp4", it.UserID, it.AttemptID)
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetReelsAttempts: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) GetCreatorAnalytics(ctx context.Context, userID uuid.UUID) (*models.CreatorAnalytics, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var dailyRows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_creator_daily_stats", func() error {
		var e error
		dailyRows, e = u.db.Query(ctx, GetCreatorDailyStatsQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query creator daily stats: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer dailyRows.Close()

	daily := []models.CreatorDailyStats{}
	for dailyRows.Next() {
		var it models.CreatorDailyStats
		if err := dailyRows.Scan(&it.Day, &it.Views, &it.Likes, &it.Attempts); err != nil {
			logger.Error("failed to scan creator daily stats: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		daily = append(daily, it)
	}
	if err := dailyRows.Err(); err != nil {
		logger.Error("rows iteration error in GetCreatorAnalytics (daily): " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	dailyRows.Close()

	var topRows pgx.Rows
	err = appmetrics.ObserveDBQuery("get_creator_top_dances", func() error {
		var e error
		topRows, e = u.db.Query(ctx, GetCreatorTopDancesQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query creator top dances: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer topRows.Close()

	top := []models.CreatorTopDance{}
	for topRows.Next() {
		var it models.CreatorTopDance
		if err := topRows.Scan(&it.DanceID, &it.Title, &it.Attempts, &it.Likes); err != nil {
			logger.Error("failed to scan creator top dance: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		top = append(top, it)
	}
	if err := topRows.Err(); err != nil {
		logger.Error("rows iteration error in GetCreatorAnalytics (top): " + err.Error())
		return nil, users.ErrorInternalServerError
	}

	return &models.CreatorAnalytics{Daily: daily, TopDances: top}, nil
}

func (u *UserRepository) GetAttemptOwner(ctx context.Context, attemptID string) (*uuid.UUID, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var ownerID *uuid.UUID
	err := appmetrics.ObserveDBQuery("get_attempt_owner", func() error {
		return u.db.QueryRow(ctx, GetAttemptOwnerQuery, attemptID).Scan(&ownerID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.Error("failed to get attempt owner: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return ownerID, nil
}

func (u *UserRepository) GetLastAttempt(ctx context.Context, userID uuid.UUID, danceID string) (*models.UserAttemptItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var it models.UserAttemptItem
	err := appmetrics.ObserveDBQuery("get_last_attempt", func() error {
		return u.db.QueryRow(ctx, GetLastAttemptQuery, userID, danceID).
			Scan(&it.AttemptID, &it.DanceID, &it.DanceTitle, &it.Score, &it.CreatedAt, &it.IsSaved)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		logger.Error("failed to query last attempt: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return &it, nil
}

func (u *UserRepository) GetPersonalTop(ctx context.Context, userID uuid.UUID) ([]models.PersonalTopItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_personal_top", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetPersonalTopQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query personal top: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.PersonalTopItem{}
	for rows.Next() {
		var it models.PersonalTopItem
		if err := rows.Scan(&it.DanceID, &it.UserDanceID, &it.DanceTitle, &it.UserName, &it.BestScore, &it.AchievedAt); err != nil {
			logger.Error("failed to scan personal top item: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetPersonalTop: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) HasOpenDuel(ctx context.Context, challengerID, opponentID uuid.UUID, danceID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var exists bool
	err := appmetrics.ObserveDBQuery("has_open_duel", func() error {
		return u.db.QueryRow(ctx, HasOpenDuelQuery, challengerID, opponentID, danceID).Scan(&exists)
	})
	if err != nil {
		logger.Error("failed to check open duel: " + err.Error())
		return false, users.ErrorInternalServerError
	}
	return exists, nil
}

func (u *UserRepository) GetActiveDuelsForUserDance(ctx context.Context, userID uuid.UUID, danceID string) ([]models.ActiveDuelForDance, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_active_duels_for_user_dance", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetActiveDuelsForUserDanceQuery, userID, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to query active duels for dance: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := []models.ActiveDuelForDance{}
	for rows.Next() {
		var it models.ActiveDuelForDance
		if err := rows.Scan(&it.DuelID, &it.OpponentLogin); err != nil {
			logger.Error("failed to scan active duel: " + err.Error())
			return nil, users.ErrorInternalServerError
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetActiveDuelsForUserDance: " + err.Error())
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}
