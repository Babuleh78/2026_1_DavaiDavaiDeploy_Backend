package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/auth"
	"DDDance/internal/pkg/utils/log"
	"DDDance/internal/pkg/utils/password"
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/dgryski/dgoogauth"

	jwt "github.com/golang-jwt/jwt/v5"
	uuid "github.com/satori/go.uuid"
	"github.com/skip2/go-qrcode"
)

func randomSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32.StdEncoding.EncodeToString(buf), nil
}

type AuthUsecase struct {
	secret   string
	authRepo auth.AuthRepo
}

func NewAuthUsecase(repo auth.AuthRepo, secret string) *AuthUsecase {
	return &AuthUsecase{
		authRepo: repo,
		secret:   secret,
	}
}

func (uc *AuthUsecase) GenerateToken(id uuid.UUID, login string, version int) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":      id,
		"login":   login,
		"version": version,
		"exp":     time.Now().Add(30 * 24 * time.Hour).Unix(),
	})
	return token.SignedString([]byte(uc.secret))
}

func (uc *AuthUsecase) ParseToken(token string) (*jwt.Token, error) {
	return jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(uc.secret), nil
	})
}

func (uc *AuthUsecase) SignInVKUser(ctx context.Context, vkid string) (models.User, string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	vkUser, err := uc.authRepo.GetVKUser(ctx, vkid)
	if err != nil {
		return models.User{}, "", auth.ErrorPreconditionFailed
	}

	token, err := uc.GenerateToken(vkUser.ID, vkUser.Login, vkUser.Version)
	if err != nil {
		logger.Error("cannot generate token")
		return models.User{}, "", auth.ErrorInternalServerError
	}

	return vkUser, token, nil
}

func (uc *AuthUsecase) SignUpVKUser(ctx context.Context, vkid string, login string) (models.User, string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	isRegistered, err := uc.authRepo.CheckUserExists(ctx, login)
	if err != nil {
		logger.Error("cannot check if login exists", slog.Any("error", err))
		return models.User{}, "", auth.ErrorInternalServerError
	}
	if isRegistered {
		logger.Warn("Such login already taken")
		return models.User{}, "", auth.ErrorBadRequest
	}

	randomPass, err := randomSecret()
	if err != nil {
		logger.Error("cannot generate vk password")
		return models.User{}, "", auth.ErrorInternalServerError
	}
	passwordHash, err := password.Hash(randomPass)
	if err != nil {
		logger.Error("cannot hash password")
		return models.User{}, "", auth.ErrorInternalServerError
	}

	id := uuid.NewV4()
	defaultAvatar := "avatars/default.png"

	user := models.User{
		ID:           id,
		Login:        login,
		PasswordHash: passwordHash,
		Avatar:       defaultAvatar,
		Version:      1,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	err = uc.authRepo.CreateVKUser(ctx, user, vkid)
	if err != nil {
		return models.User{}, "", err
	}

	token, err := uc.GenerateToken(id, login, user.Version)
	if err != nil {
		logger.Error("cannot generate token")
		return models.User{}, "", auth.ErrorInternalServerError
	}

	return user, token, nil
}

func (uc *AuthUsecase) SignUpUser(ctx context.Context, req models.SignUpInput) (models.User, string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	msg, dataIsValid := auth.Validation(req.Login, req.Password)
	if !dataIsValid {
		logger.Warn(msg)
		return models.User{}, "", auth.ErrorBadRequest
	}

	exists, err := uc.authRepo.CheckUserExists(ctx, req.Login)
	if err != nil {
		return models.User{}, "", err
	}
	if exists {
		logger.Warn("user already exists")
		return models.User{}, "", auth.ErrorConflict
	}

	passwordHash, err := password.Hash(req.Password)
	if err != nil {
		logger.Error("cannot hash password")
		return models.User{}, "", auth.ErrorInternalServerError
	}

	id := uuid.NewV4()
	defaultAvatar := "avatars/default.png"

	user := models.User{
		ID:           id,
		Login:        req.Login,
		PasswordHash: passwordHash,
		Avatar:       defaultAvatar,
		Version:      1,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	err = uc.authRepo.CreateUser(ctx, user)
	if err != nil {
		return models.User{}, "", err
	}

	token, err := uc.GenerateToken(id, req.Login, user.Version)
	if err != nil {
		logger.Error("cannot generate token")
		return models.User{}, "", auth.ErrorInternalServerError
	}

	return user, token, nil
}

func (uc *AuthUsecase) VerifyOTPCode(ctx context.Context, login, secretCode string, userCode string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	otpConfig := &dgoogauth.OTPConfig{
		Secret:      secretCode,
		WindowSize:  5,
		HotpCounter: 0,
	}
	isValid, err := otpConfig.Authenticate(userCode)
	if err != nil || !isValid {
		logger.Warn("OTP authentication failed")
		return auth.ErrorUnauthorized
	}

	logger.Info("OTP code verified successfully", slog.String("login", login))
	return nil
}

func (uc *AuthUsecase) SignInUser(ctx context.Context, req models.SignInInput) (models.User, string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	neededUser, err := uc.authRepo.CheckUserLogin(ctx, req.Login)
	if err != nil {
		return models.User{}, "", err
	}

	if neededUser.IsForeign {
		logger.Warn("password login attempt on VK account")
		return models.User{}, "", auth.ErrorBadRequest
	}

	if !password.Check(neededUser.PasswordHash, req.Password) {
		logger.Warn("wrong password")
		return models.User{}, "", auth.ErrorBadRequest
	}

	has2FA, err := uc.authRepo.CheckUserTwoFactor(ctx, neededUser.ID)
	if err != nil {
		logger.Error("failed to check 2FA status", slog.Any("error", err))
		return models.User{}, "", auth.ErrorInternalServerError
	}
	if has2FA {
		if req.Code == nil || *req.Code == "" {
			logger.Warn("2FA code required but not provided")
			return models.User{}, "", auth.ErrorBadRequest
		}
		secretCode, secErr := uc.authRepo.GetUserSecretCode(ctx, neededUser.ID)
		if secErr != nil {
			return models.User{}, "", auth.ErrorInternalServerError
		}
		if err := uc.VerifyOTPCode(ctx, req.Login, secretCode, *req.Code); err != nil {
			return models.User{}, "", err
		}
	}

	token, err := uc.GenerateToken(neededUser.ID, req.Login, neededUser.Version)
	if err != nil {
		logger.Error("cannot generate token")
		return models.User{}, "", auth.ErrorInternalServerError
	}

	return neededUser, token, nil
}

func (uc *AuthUsecase) LogOutUser(ctx context.Context, userID uuid.UUID) error {
	err := uc.authRepo.IncrementUserVersion(ctx, userID)
	if err != nil {
		return err
	}

	return nil
}

func (uc *AuthUsecase) GenerateQRCode(login string) ([]byte, string, error) {
	secret := make([]byte, 20)
	_, err := rand.Read(secret)
	if err != nil {
		return []byte{}, "", auth.ErrorInternalServerError
	}

	secretBase32 := base32.StdEncoding.EncodeToString(secret)

	issuer := "dddance"
	otpURL := fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s",
		url.PathEscape(issuer),
		url.PathEscape(login),
		secretBase32,
		url.PathEscape(issuer))

	qrCode, err := qrcode.Encode(otpURL, qrcode.Medium, 256)
	if err != nil {
		return []byte{}, "", auth.ErrorInternalServerError
	}

	return qrCode, secretBase32, nil
}

func (uc *AuthUsecase) ValidateAndGetUser(ctx context.Context, token string) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if token == "" {
		logger.Error("user is not authorized")
		return models.User{}, auth.ErrorUnauthorized
	}

	parsedToken, err := uc.ParseToken(token)
	if err != nil || !parsedToken.Valid {
		logger.Error("user is not authorized or invalid token")
		return models.User{}, auth.ErrorUnauthorized
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		logger.Error("invalid claims")
		return models.User{}, auth.ErrorUnauthorized
	}

	exp, ok := claims["exp"].(float64)
	if !ok || int64(exp) < time.Now().Unix() {
		logger.Error("invalid exp claim")
		return models.User{}, auth.ErrorUnauthorized
	}

	login, ok := claims["login"].(string)
	if !ok || login == "" {
		logger.Error("invalid login claim")
		return models.User{}, auth.ErrorUnauthorized
	}

	user, err := uc.authRepo.GetUserByLogin(ctx, login)
	if err != nil {
		return models.User{}, err
	}

	version, ok := claims["version"].(float64)
	if !ok {
		logger.Error("invalid version claim")
		return models.User{}, auth.ErrorUnauthorized
	}

	if int(version) != user.Version {
		logger.Error("token version mismatch")
		return models.User{}, auth.ErrorUnauthorized
	}

	return user, nil
}
