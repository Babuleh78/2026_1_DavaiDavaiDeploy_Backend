package repo

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgtype/pgxtype"
	"github.com/jackc/pgx/v4"
	uuid "github.com/satori/go.uuid"
)

type UserRepository struct {
	db pgxtype.Querier
}

func NewUserRepository(db pgxtype.Querier) *UserRepository {
	return &UserRepository{db: db}
}

func (u *UserRepository) GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := u.db.QueryRow(
		ctx,
		GetUserByIDQuery,
		id,
	).Scan(
		&user.ID, &user.Version, &user.Login,
		&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("user not exists")
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("failed to scan user: " + err.Error())
		return models.User{}, users.ErrorInternalServerError
	}

	logger.Info("succesfully got user by id from db")
	return user, nil
}

func (u *UserRepository) GetUserByLogin(ctx context.Context, login string) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := u.db.QueryRow(
		ctx,
		GetUserByLoginQuery,
		login,
	).Scan(
		&user.ID, &user.Version, &user.Login,
		&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("user not exists")
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("failed to scan user: " + err.Error())
		return models.User{}, users.ErrorInternalServerError
	}

	logger.Info("succesfully got user by login from db")
	return user, nil
}

func (u *UserRepository) UpdateUserPassword(ctx context.Context, version int, userID uuid.UUID, passwordHash []byte) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	_, err := u.db.Exec(
		ctx,
		UpdateUserPasswordQuery,
		passwordHash, version, userID,
	)
	if err != nil {
		logger.Error("failed to update password: " + err.Error())
		return users.ErrorInternalServerError
	}

	logger.Info("succesfully updated password of user from db")
	return err
}

func (u *UserRepository) AddToHistory(ctx context.Context, userID uuid.UUID, danceID string, sourceURL string) error {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
    _, err := u.db.Exec(ctx, AddToHistoryQuery, userID, danceID, sourceURL)
    if err != nil {
        logger.Error("failed to add to history: " + err.Error())
        return users.ErrorInternalServerError
    }
    logger.Info("successfully added to search history")
    return nil
}

func (u *UserRepository) GetHistory(ctx context.Context, userID uuid.UUID) ([]models.SearchHistoryItem, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
    rows, err := u.db.Query(ctx, GetHistoryQuery, userID)
    if err != nil {
        logger.Error("failed to get history: " + err.Error())
        return nil, users.ErrorInternalServerError
    }
    defer rows.Close()

    var items []models.SearchHistoryItem
    for rows.Next() {
        var item models.SearchHistoryItem
        if err := rows.Scan(&item.ID, &item.UserID, &item.DanceID, &item.Name, &item.SourceURL, &item.CreatedAt); err != nil {
            logger.Error("failed to scan history item: " + err.Error())
            return nil, users.ErrorInternalServerError
        }
        items = append(items, item)
    }
    return items, nil
}

func (u *UserRepository) DeleteFromHistory(ctx context.Context, historyID uuid.UUID, userID uuid.UUID) error {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
    result, err := u.db.Exec(ctx, DeleteFromHistoryQuery, historyID, userID)
    if err != nil {
        logger.Error("failed to delete from history: " + err.Error())
        return users.ErrorInternalServerError
    }
    if result.RowsAffected() == 0 {
        return users.ErrorNotFound
    }
    logger.Info("successfully deleted from search history")
    return nil
}

func (u *UserRepository) UpdateHistoryName(ctx context.Context, historyID uuid.UUID, userID uuid.UUID, name string) error {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
    result, err := u.db.Exec(ctx, UpdateHistoryNameQuery, name, historyID, userID)
    if err != nil {
        logger.Error("failed to update history name: " + err.Error())
        return users.ErrorInternalServerError
    }
    if result.RowsAffected() == 0 {
        return users.ErrorNotFound
    }
    logger.Info("successfully updated history item name")
    return nil
}

func (u *UserRepository) ToggleLike(ctx context.Context, userID uuid.UUID, danceID string) (bool, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

    var inserted bool
    err := u.db.QueryRow(ctx, ToggleLikeQuery, userID, danceID).Scan(&inserted)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            _, err = u.db.Exec(ctx, DeleteLikeQuery, userID, danceID)
            if err != nil {
                logger.Error("failed to delete like: " + err.Error())
                return false, users.ErrorInternalServerError
            }
            logger.Info("like removed")
            return false, nil
        }
        logger.Error("failed to toggle like: " + err.Error())
        return false, users.ErrorInternalServerError
    }

    logger.Info("like added")
    return true, nil
}

func (u *UserRepository) GetLikesCount(ctx context.Context, danceID string) (int64, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
    var count int64
    err := u.db.QueryRow(ctx, GetLikesCountQuery, danceID).Scan(&count)
    if err != nil {
        logger.Error("failed to get likes count: " + err.Error())
        return 0, users.ErrorInternalServerError
    }
    return count, nil
}

func (u *UserRepository) IsLikedByUser(ctx context.Context, userID uuid.UUID, danceID string) (bool, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
    var exists bool
    err := u.db.QueryRow(ctx, IsLikedByUserQuery, userID, danceID).Scan(&exists)
    if err != nil {
        logger.Error("failed to check like: " + err.Error())
        return false, users.ErrorInternalServerError
    }
    return exists, nil
}

func (u *UserRepository) GetTopLikedDances(ctx context.Context, limit int) ([]models.DanceLikeStat, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
    rows, err := u.db.Query(ctx, GetTopLikedDancesQuery, limit)
    if err != nil {
        logger.Error("failed to get top dances: " + err.Error())
        return nil, users.ErrorInternalServerError
    }
    defer rows.Close()

    var stats []models.DanceLikeStat
    for rows.Next() {
        var s models.DanceLikeStat
        if err := rows.Scan(&s.DanceID, &s.LikesCount); err != nil {
            logger.Error("failed to scan dance stat: " + err.Error())
            return nil, users.ErrorInternalServerError
        }
        stats = append(stats, s)
    }
    return stats, nil
}

func (u *UserRepository) GetUserLikedDances(ctx context.Context, userID uuid.UUID) ([]models.DanceLike, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
    rows, err := u.db.Query(ctx, GetUserLikedDancesQuery, userID)
    if err != nil {
        logger.Error("failed to get liked dances: " + err.Error())
        return nil, users.ErrorInternalServerError
    }
    defer rows.Close()

    var likes []models.DanceLike
    for rows.Next() {
        var l models.DanceLike
        var historyID *string
        if err := rows.Scan(&historyID, &l.DanceID, &l.Name, &l.CreatedAt); err != nil {
            logger.Error("failed to scan like: " + err.Error())
            return nil, users.ErrorInternalServerError
        }
        if historyID != nil {
            l.HistoryID = *historyID
        }
        likes = append(likes, l)
    }
    return likes, nil
}

func (u *UserRepository) CleanHistory(ctx context.Context, userID uuid.UUID) error {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
    _, err := u.db.Exec(ctx, CleanHistoryQuery, userID)
    if err != nil {
        logger.Error("failed to clean history: " + err.Error())
        return users.ErrorInternalServerError
    }
    return nil
}

func (u *UserRepository) SaveRating(ctx context.Context, userID uuid.UUID, input models.SaveRatingInput) error {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
    _, err := u.db.Exec(ctx, SaveRatingQuery,
        input.VideoID, userID, input.Physical, input.Speed, input.Coordination, input.Repeatability)
    if err != nil {
        logger.Error("failed to save rating: " + err.Error())
        return users.ErrorInternalServerError
    }
    return nil
}

func (u *UserRepository) GetAggregatedRating(ctx context.Context, videoID string) (*models.RatingResponse, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
    row := u.db.QueryRow(ctx, GetAggregatedRatingQuery, videoID)

    var r models.RatingResponse
    r.VideoID = videoID
    if err := row.Scan(&r.AvgPhysical, &r.AvgSpeed, &r.AvgCoordination, &r.AvgRepeatability, &r.AvgScore, &r.TotalRatings); err != nil {
        logger.Error("failed to get rating: " + err.Error())
        return nil, users.ErrorInternalServerError
    }
    return &r, nil
}

