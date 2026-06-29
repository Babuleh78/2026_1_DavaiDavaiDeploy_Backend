package repo

import (
	"DDDance/internal/models"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v4"
	uuid "github.com/satori/go.uuid"
)

func (u *UserRepository) GetFriendsFeed(ctx context.Context, userID uuid.UUID, limit int, cursor time.Time) ([]models.FeedItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if cursor.IsZero() {
		cursor = time.Now().Add(time.Second)
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("get_friends_feed", func() error {
		var e error
		rows, e = u.db.Query(ctx, GetFriendsFeedQuery, userID, limit, cursor)
		return e
	})
	if err != nil {
		logger.Error("failed to query friends feed", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	items := make([]models.FeedItem, 0, limit)
	for rows.Next() {
		var item models.FeedItem
		if err := rows.Scan(&item.ID, &item.ActionType, &item.Metadata, &item.CreatedAt, &item.ActorLogin, &item.ActorAvatar, &item.DanceTitle, &item.ActorID); err != nil {
			logger.Error("failed to scan feed item", "error", err)
			return nil, users.ErrorInternalServerError
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows error in friends feed", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return items, nil
}
