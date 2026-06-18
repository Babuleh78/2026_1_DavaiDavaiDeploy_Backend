package repo

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/auth"
	appmetrics "DDDance/internal/pkg/metrics"
	"DDDance/internal/pkg/utils/log"
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/jackc/pgtype/pgxtype"
	"github.com/jackc/pgx/v4"
	uuid "github.com/satori/go.uuid"
)

type AuthRepository struct {
	db pgxtype.Querier
}

func NewAuthRepository(db pgxtype.Querier) *AuthRepository {
	return &AuthRepository{db: db}
}

func (r *AuthRepository) CheckUserExists(ctx context.Context, login string) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var exists bool
	err := appmetrics.ObserveDBQuery("auth_check_user_exists", func() error {
		return r.db.QueryRow(ctx, CheckUserExistsQuery, login).Scan(&exists)
	})
	if err != nil {
		logger.Error("failed to scan user: " + err.Error())
		return false, auth.ErrorInternalServerError
	}
	logger.Info("succesfully checked user")
	return exists, nil
}

func (r *AuthRepository) CreateUser(ctx context.Context, user models.User) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("auth_create_user", func() error {
		_, e := r.db.Exec(ctx, CreateUserQuery, user.ID, user.Login, user.PasswordHash, user.CreatedAt, user.UpdatedAt)
		return e
	})
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "23505") || strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint") {
			logger.Error("login already taken: " + msg)
			return auth.ErrorLoginAlreadyExists
		}
		logger.Error("failed to create user: " + msg)
		return auth.ErrorInternalServerError
	}
	logger.Info("succesfully created user")
	return nil
}

func (r *AuthRepository) CheckUserLogin(ctx context.Context, login string) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("auth_check_user_login", func() error {
		return r.db.QueryRow(ctx, CheckUserLoginQuery, login).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt, &user.IsForeign,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("user not exists")
			return models.User{}, auth.ErrorBadRequest
		}
		logger.Error("failed to scan user: " + err.Error())
		return models.User{}, auth.ErrorInternalServerError
	}
	logger.Info("succesfully got user by login from db")
	return user, nil
}

func (r *AuthRepository) IncrementUserVersion(ctx context.Context, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("auth_increment_user_version", func() error {
		_, e := r.db.Exec(ctx, IncrementUserVersionQuery, userID)
		return e
	})
	if err != nil {
		logger.Error("failed to increment version: " + err.Error())
		return auth.ErrorInternalServerError
	}
	logger.Info("succesfully incremented version of personal data")
	return nil
}

func (r *AuthRepository) GetUserByLogin(ctx context.Context, login string) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("auth_get_user_by_login", func() error {
		return r.db.QueryRow(ctx, GetUserByLoginQuery, login).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.Has2FA, &user.CreatedAt, &user.UpdatedAt, &user.IsForeign,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("user not exists")
			return models.User{}, auth.ErrorBadRequest
		}
		logger.Error("failed to scan user: " + err.Error())
		return models.User{}, auth.ErrorInternalServerError
	}
	logger.Info("succesfully got user by login from db")
	return user, nil
}

func (r *AuthRepository) GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("auth_get_user_by_id", func() error {
		return r.db.QueryRow(ctx, GetUserByIDQuery, id).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("user not exists")
			return models.User{}, auth.ErrorBadRequest
		}
		logger.Error("failed to scan user: " + err.Error())
		return models.User{}, auth.ErrorInternalServerError
	}
	logger.Info("succesfully got user by id from db")
	return user, nil
}

func (r *AuthRepository) CheckUserTwoFactor(ctx context.Context, userID uuid.UUID) (bool, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var has2FA bool
	err := appmetrics.ObserveDBQuery("auth_check_user_two_factor", func() error {
		return r.db.QueryRow(ctx, CheckUserTwoFactorQuery, userID).Scan(&has2FA)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("user not exists")
			return false, auth.ErrorBadRequest
		}
		logger.Error("failed to check 2FA status: " + err.Error())
		return false, auth.ErrorInternalServerError
	}
	logger.Info("successfully checked 2FA status")
	return has2FA, nil
}

func (r *AuthRepository) GetUserSecretCode(ctx context.Context, userID uuid.UUID) string {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var secretCode string
	err := appmetrics.ObserveDBQuery("auth_get_user_secret_code", func() error {
		return r.db.QueryRow(ctx, CheckUserSecretCodeQuery, userID).Scan(&secretCode)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("user not exists")
		} else {
			logger.Error("failed to check 2FA status: " + err.Error())
		}
		return ""
	}
	logger.Info("successfully checked 2FA status")
	return secretCode
}

func (r *AuthRepository) GetVKUser(ctx context.Context, vkid string) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	var user models.User
	err := appmetrics.ObserveDBQuery("auth_get_vk_user", func() error {
		return r.db.QueryRow(ctx, CheckVKUserExistsQuery, vkid).Scan(
			&user.ID, &user.Version, &user.Login,
			&user.PasswordHash, &user.Avatar, &user.CreatedAt, &user.UpdatedAt,
		)
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			logger.Error("vk user not exists")
			return models.User{}, auth.ErrorBadRequest
		}
		logger.Error("failed to scan vk user: " + err.Error())
		return models.User{}, auth.ErrorInternalServerError
	}
	logger.Info("succesfully got vk user by id from db")
	return user, nil
}

func (r *AuthRepository) CreateVKUser(ctx context.Context, user models.User, vkid string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	err := appmetrics.ObserveDBQuery("auth_create_vk_user", func() error {
		_, e := r.db.Exec(ctx, CreateVKUserQuery, user.ID, user.Login, user.PasswordHash, user.CreatedAt, user.UpdatedAt, vkid)
		return e
	})
	if err != nil {
		logger.Error("failed to create vk user: " + err.Error())
		return auth.ErrorInternalServerError
	}
	logger.Info("succesfully created vk user")
	return nil
}
