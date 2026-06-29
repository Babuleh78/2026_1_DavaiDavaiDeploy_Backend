package repo

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"errors"
	"github.com/jackc/pgx/v4"
	uuid "github.com/satori/go.uuid"
	"log/slog"
	"strings"
)

func (u *UserRepository) ToggleLike(ctx context.Context, userID uuid.UUID, danceID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var inserted bool
	err := appmetrics.ObserveDBQuery("toggle_like_insert", func() error {
		return u.db.QueryRow(ctx, ToggleLikeQuery, userID, danceID).Scan(&inserted)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			err = appmetrics.ObserveDBQuery("toggle_like_delete", func() error {
				_, e := u.db.Exec(ctx, DeleteLikeQuery, userID, danceID)
				return e
			})
			if err != nil {
				logger.Error("failed to delete like", "error", err)
				return false, users.ErrorInternalServerError
			}
			logger.Info("like removed")
			return false, nil
		}
		logger.Error("failed to toggle like", "error", err)
		return false, users.ErrorInternalServerError
	}

	logger.Info("like added")
	return true, nil
}

func (u *UserRepository) GetLikesCount(ctx context.Context, danceID string) (int64, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var count int64
	err := appmetrics.ObserveDBQuery("get_likes_count", func() error {
		return u.db.QueryRow(ctx, GetLikesCountQuery, danceID).Scan(&count)
	})
	if err != nil {
		logger.Error("failed to get likes count", "error", err)
		return 0, users.ErrorInternalServerError
	}
	return count, nil
}

func (u *UserRepository) IsLikedByUser(ctx context.Context, userID uuid.UUID, danceID string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var exists bool
	err := appmetrics.ObserveDBQuery("is_liked_by_user", func() error {
		return u.db.QueryRow(ctx, IsLikedByUserQuery, userID, danceID).Scan(&exists)
	})
	if err != nil {
		logger.Error("failed to check like", "error", err)
		return false, users.ErrorInternalServerError
	}
	return exists, nil
}

func (u *UserRepository) GetTopLikedDances(ctx context.Context, limit int) ([]models.DanceLikeStat, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_top_liked_dances", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetTopLikedDancesQuery, limit)
		return e
	})
	if err != nil {
		logger.Error("failed to get top dances", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var stats []models.DanceLikeStat
	for rows.Next() {
		var s models.DanceLikeStat
		if err := rows.Scan(&s.DanceID, &s.LikesCount); err != nil {
			logger.Error("failed to scan dance stat", "error", err)
			return nil, users.ErrorInternalServerError
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetTopLikedDances", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return stats, nil
}

func (u *UserRepository) GetUserLikedDances(ctx context.Context, userID uuid.UUID) ([]models.DanceLike, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_user_liked_dances", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetUserLikedDancesQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to get liked dances", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	var likes []models.DanceLike
	for rows.Next() {
		var l models.DanceLike
		var historyID *string
		if err := rows.Scan(&historyID, &l.DanceID, &l.Name, &l.DanceTitle, &l.CreatedAt); err != nil {
			logger.Error("failed to scan like", "error", err)
			return nil, users.ErrorInternalServerError
		}
		if historyID != nil {
			l.HistoryID = *historyID
		}
		likes = append(likes, l)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetUserLikedDances", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return likes, nil
}

func (u *UserRepository) SaveRating(ctx context.Context, userID uuid.UUID, input models.SaveRatingInput) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("save_rating", func() error {
		_, e := u.db.Exec(ctx, SaveRatingQuery,
			input.VideoID, userID, input.Physical, input.Speed, input.Coordination, input.Repeatability)
		return e
	})
	if err != nil {
		logger.Error("failed to save rating", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetAggregatedRating(ctx context.Context, videoID string) (*models.RatingResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var r models.RatingResponse
	r.VideoID = videoID
	err := appmetrics.ObserveDBQuery("get_aggregated_rating", func() error {
		return u.db.QueryRow(ctx, GetAggregatedRatingQuery, videoID).Scan(
			&r.AvgPhysical, &r.AvgSpeed, &r.AvgCoordination, &r.AvgRepeatability, &r.AvgScore, &r.TotalRatings,
		)
	})
	if err != nil {
		logger.Error("failed to get rating", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return &r, nil
}

func (u *UserRepository) GetFriendsDanceScores(ctx context.Context, userID uuid.UUID, danceID string) ([]models.FriendScore, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_friends_dance_scores", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetFriendsScoresQuery, userID, danceID)
		return e
	})
	if err != nil {
		logger.Error("failed to query friends dance scores", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := []models.FriendScore{}
	for rows.Next() {
		var fs models.FriendScore
		if err := rows.Scan(&fs.Login, &fs.AvatarURL, &fs.BestScore); err != nil {
			logger.Error("failed to scan friend score", "error", err)
			return nil, users.ErrorInternalServerError
		}
		result = append(result, fs)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetFriendsDanceScores", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) CreateNotification(ctx context.Context, userID uuid.UUID, notifType, danceID, reason string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var reasonArg interface{}
	if reason == "" {
		reasonArg = nil
	} else {
		reasonArg = reason
	}
	err := appmetrics.ObserveDBQuery("create_notification", func() error {
		_, e := u.db.Exec(ctx, CreateNotificationQuery, userID, notifType, danceID, reasonArg)
		return e
	})
	if err != nil {
		logger.Error("failed to create notification", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetNotifications(ctx context.Context, userID uuid.UUID) ([]models.Notification, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_notifications", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetNotificationsQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query notifications", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := []models.Notification{}
	for rows.Next() {
		var n models.Notification
		if err := rows.Scan(&n.ID, &n.Type, &n.DanceID, &n.Reason, &n.IsRead, &n.CreatedAt, &n.FromUserID, &n.FromLogin, &n.RefID, &n.DuelID); err != nil {
			logger.Error("failed to scan notification", "error", err)
			return nil, users.ErrorInternalServerError
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetNotifications", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}

func (u *UserRepository) MarkNotificationRead(ctx context.Context, id int64, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("mark_notification_read", func() error {
		_, e := u.db.Exec(ctx, MarkNotificationReadQuery, id, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to mark notification read", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("mark_all_notifications_read", func() error {
		_, e := u.db.Exec(ctx, MarkAllNotificationsReadQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to mark all notifications read", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) ClearNotifications(ctx context.Context, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("clear_notifications", func() error {
		_, e := u.db.Exec(ctx, ClearNotificationsQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to clear notifications", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) CreateFriendship(ctx context.Context, senderID, receiverID uuid.UUID) (int64, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var id int64
	err := appmetrics.ObserveDBQuery("create_friendship", func() error {
		return u.db.QueryRow(ctx, CreateFriendshipQuery, senderID, receiverID).Scan(&id)
	})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint") || strings.Contains(msg, "23505") {
			return 0, users.ErrorAlreadyExists
		}
		logger.Error("failed to create friendship: " + msg)
		return 0, users.ErrorInternalServerError
	}
	return id, nil
}

func (u *UserRepository) UpdateFriendshipStatus(ctx context.Context, friendshipID int64, receiverID uuid.UUID, status string) (uuid.UUID, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var senderID uuid.UUID
	err := appmetrics.ObserveDBQuery("update_friendship_status", func() error {
		return u.db.QueryRow(ctx, UpdateFriendshipStatusQuery, friendshipID, receiverID, status).Scan(&senderID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, users.ErrorNotFound
		}
		logger.Error("failed to update friendship status", "error", err)
		return uuid.Nil, users.ErrorInternalServerError
	}
	return senderID, nil
}

func (u *UserRepository) GetFriends(ctx context.Context, userID uuid.UUID) ([]models.Friend, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_friends", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetFriendsQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to query friends", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := []models.Friend{}
	for rows.Next() {
		var f models.Friend
		if err := rows.Scan(&f.UserID, &f.Login, &f.Avatar, &f.FriendedAt, &f.ActiveDuelID); err != nil {
			logger.Error("failed to scan friend", "error", err)
			return nil, users.ErrorInternalServerError
		}
		result = append(result, f)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in GetFriends", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) GetFriendsCount(ctx context.Context, userID uuid.UUID) (int, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var count int
	err := appmetrics.ObserveDBQuery("get_friends_count", func() error {
		return u.db.QueryRow(ctx, GetFriendsCountQuery, userID).Scan(&count)
	})
	if err != nil {
		logger.Error("failed to count friends", "error", err)
		return 0, users.ErrorInternalServerError
	}
	return count, nil
}

func (u *UserRepository) GetFriendshipBetween(ctx context.Context, userID, otherID uuid.UUID) (*models.FriendshipStatus, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var id int64
	var status string
	var senderID uuid.UUID
	err := appmetrics.ObserveDBQuery("get_friendship_between", func() error {
		return u.db.QueryRow(ctx, GetFriendshipBetweenQuery, userID, otherID).Scan(&id, &status, &senderID)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &models.FriendshipStatus{Status: "none"}, nil
		}
		logger.Error("failed to get friendship", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return &models.FriendshipStatus{
		Status:       status,
		IsSender:     senderID == userID,
		FriendshipID: id,
	}, nil
}

func (u *UserRepository) DeleteFriendship(ctx context.Context, userID, friendID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("delete_friendship", func() error {
		_, e := u.db.Exec(ctx, DeleteFriendshipQuery, userID, friendID)
		return e
	})
	if err != nil {
		logger.Error("failed to delete friendship", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) CreateFriendNotification(ctx context.Context, toUserID, fromUserID uuid.UUID, notifType string, friendshipID int64) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("create_friend_notification", func() error {
		_, e := u.db.Exec(ctx, CreateFriendNotificationQuery, toUserID, notifType, fromUserID, friendshipID)
		return e
	})
	if err != nil {
		logger.Error("failed to create friend notification", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}
