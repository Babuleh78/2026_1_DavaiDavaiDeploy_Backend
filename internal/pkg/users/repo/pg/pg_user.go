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
	"time"
)

func (u *UserRepository) GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("get_user_by_id", func() error {
		return u.db.QueryRow(ctx, GetUserByIDQuery, id).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("user not exists")
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("failed to scan user", "error", err)
		return models.User{}, users.ErrorInternalServerError
	}

	logger.Info("succesfully got user by id from db")
	return user, nil
}

func (u *UserRepository) GetUserByLogin(ctx context.Context, login string) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("get_user_by_login", func() error {
		return u.db.QueryRow(ctx, GetUserByLoginQuery, login).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("user not exists")
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("failed to scan user", "error", err)
		return models.User{}, users.ErrorInternalServerError
	}

	logger.Info("succesfully got user by login from db")
	return user, nil
}

func (u *UserRepository) UpdateUserPassword(ctx context.Context, version int, userID uuid.UUID, passwordHash []byte) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("update_user_password", func() error {
		_, e := u.db.Exec(ctx, UpdateUserPasswordQuery, passwordHash, version, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to update password", "error", err)
		return users.ErrorInternalServerError
	}

	logger.Info("succesfully updated password of user from db")
	return nil
}

func (u *UserRepository) UpdateUserProfile(ctx context.Context, userID uuid.UUID, login *string, avatar *string, bumpVersion bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("update_user_profile", func() error {
		_, e := u.db.Exec(ctx, UpdateUserProfileQuery, login, avatar, bumpVersion, userID)
		return e
	})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint") || strings.Contains(msg, "23505") {
			logger.Info("login already taken")
			return users.ErrorBadRequest
		}
		logger.Error("failed to update profile: " + msg)
		return users.ErrorInternalServerError
	}
	logger.Info("successfully updated profile")
	return nil
}

func (u *UserRepository) SearchUsers(ctx context.Context, query string, limit int) ([]models.UserSearchItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var rows pgx.Rows
	err := appmetrics.ObserveDBQuery("search_users", func() error {
		var e error
		rows, e = u.db.Query(ctx, SearchUsersQuery, query, limit)
		return e
	})
	if err != nil {
		logger.Error("failed to query users search", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer rows.Close()

	result := []models.UserSearchItem{}
	for rows.Next() {
		var it models.UserSearchItem
		if err := rows.Scan(&it.ID, &it.Login, &it.Avatar); err != nil {
			logger.Error("failed to scan user search item", "error", err)
			return nil, users.ErrorInternalServerError
		}
		result = append(result, it)
	}
	if err := rows.Err(); err != nil {
		logger.Error("rows iteration error in SearchUsers", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return result, nil
}

func (u *UserRepository) FindUserByLoginOrEmail(ctx context.Context, loginOrEmail string) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("find_user_by_login_or_email", func() error {
		return u.db.QueryRow(ctx, BotFindUserQuery, loginOrEmail).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("botFindUser scan failed", "error", err)
		return models.User{}, users.ErrorInternalServerError
	}
	return user, nil
}

func (u *UserRepository) UpdateUserTelegramID(ctx context.Context, userID uuid.UUID, telegramID int64) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("update_user_telegram_id", func() error {
		_, e := u.db.Exec(ctx, BotUpdateTelegramIDQuery, telegramID, userID)
		return e
	})
	if err != nil {
		logger.Error("botUpdateTelegramID failed", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) GetUserTelegramID(ctx context.Context, userID uuid.UUID) (*int64, error) {
	var tid *int64
	err := appmetrics.ObserveDBQuery("get_user_telegram_id", func() error {
		return u.db.QueryRow(ctx, GetUserTelegramIDQuery, userID).Scan(&tid)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, users.ErrorNotFound
		}
		return nil, users.ErrorInternalServerError
	}
	return tid, nil
}

func (u *UserRepository) SetTelegramLinkCode(ctx context.Context, userID uuid.UUID, code string, expiresAt time.Time) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("set_telegram_link_code", func() error {
		_, e := u.db.Exec(ctx,
			`UPDATE user_table SET telegram_link_code = $2, telegram_link_code_expires = $3 WHERE id = $1`,
			userID, code, expiresAt)
		return e
	})
	if err != nil {
		logger.Error("SetTelegramLinkCode failed", "error", err)
		return users.ErrorInternalServerError
	}
	return nil
}

func (u *UserRepository) LinkTelegramByCode(ctx context.Context, code string, telegramID int64) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("link_telegram_by_code", func() error {
		if _, e := u.db.Exec(ctx,
			`UPDATE user_table SET telegram_id = NULL WHERE telegram_id = $1`, telegramID); e != nil {
			return e
		}
		return u.db.QueryRow(ctx,
			`UPDATE user_table
			    SET telegram_id = $2, telegram_link_code = NULL, telegram_link_code_expires = NULL
			  WHERE telegram_link_code = $1 AND telegram_link_code_expires > NOW()
			  RETURNING id, login`,
			code, telegramID).Scan(&user.ID, &user.Login)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("LinkTelegramByCode failed", "error", err)
		return models.User{}, users.ErrorInternalServerError
	}
	return user, nil
}

func (u *UserRepository) GetUserByTelegramID(ctx context.Context, telegramID int64) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("get_user_by_telegram_id", func() error {
		return u.db.QueryRow(ctx, BotGetUserByTelegramIDQuery, telegramID).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return models.User{}, users.ErrorNotFound
		}
		logger.Error("GetUserByTelegramID scan failed", "error", err)
		return models.User{}, users.ErrorInternalServerError
	}
	return user, nil
}
