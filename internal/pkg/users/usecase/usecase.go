package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/argon2"
)

func HashPass(plainPassword string) []byte {
	salt := make([]byte, 8)
	_, err := rand.Read(salt)
	if err != nil {
		return []byte{}
	}
	hashedPass := argon2.IDKey([]byte(plainPassword), []byte(salt), 1, 64*1024, 4, 32)
	return append(salt, hashedPass...)
}

func CheckPass(passHash []byte, plainPassword string) bool {
	salt := make([]byte, 8)
	copy(salt, passHash[:8])
	userHash := argon2.IDKey([]byte(plainPassword), salt, 1, 64*1024, 4, 32)
	userHashedPassword := append(salt, userHash...)
	return bytes.Equal(userHashedPassword, passHash)
}

type UserUsecase struct {
	secret      string
	userRepo    users.UsersRepo
	storageRepo users.StorageRepo
}

func NewUserUsecase(userRepo users.UsersRepo, storageRepo users.StorageRepo) *UserUsecase {
	return &UserUsecase{
		secret:      os.Getenv("JWT_SECRET"),
		userRepo:    userRepo,
		storageRepo: storageRepo,
	}
}

func (uc *UserUsecase) GenerateToken(id uuid.UUID, login string, version int) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"id":      id,
		"login":   login,
		"version": version,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	})
	return token.SignedString([]byte(uc.secret))
}

func (uc *UserUsecase) ParseToken(token string) (*jwt.Token, error) {
	return jwt.Parse(token, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(uc.secret), nil
	})
}

func (uc *UserUsecase) ValidateAndGetUser(ctx context.Context, token string) (models.User, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if token == "" {
		logger.Error("no token")
		return models.User{}, users.ErrorUnauthorized
	}

	parsedToken, err := uc.ParseToken(token)
	if err != nil || !parsedToken.Valid {
		return models.User{}, users.ErrorUnauthorized
	}

	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		logger.Error("invalid claims")
		return models.User{}, users.ErrorUnauthorized
	}

	exp, ok := claims["exp"].(float64)
	if !ok || int64(exp) < time.Now().Unix() {
		logger.Error("invalid exp claim")
		return models.User{}, users.ErrorUnauthorized
	}

	login, ok := claims["login"].(string)
	if !ok || login == "" {
		logger.Error("invalid login claim")
		return models.User{}, users.ErrorUnauthorized
	}

	user, err := uc.userRepo.GetUserByLogin(ctx, login)
	if err != nil {
		return models.User{}, users.ErrorUnauthorized
	}

	version, ok := claims["version"].(float64)
	if !ok {
		logger.Error("invalid version claim")
		return models.User{}, users.ErrorUnauthorized
	}

	if int(version) != user.Version {
		return models.User{}, err
	}

	return user, nil
}

func (uc *UserUsecase) GetUser(ctx context.Context, id uuid.UUID) (models.User, error) {
	user, err := uc.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

func (uc *UserUsecase) ChangePassword(ctx context.Context, id uuid.UUID, oldPassword string, newPassword string) (models.User, string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	neededUser, err := uc.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return models.User{}, "", err
	}

	if !CheckPass(neededUser.PasswordHash, oldPassword) {
		logger.Error("wrong old password")
		return models.User{}, "", users.ErrorBadRequest
	}

	msg, passwordIsValid := users.Validation(neededUser.Login, newPassword)
	if !passwordIsValid {
		logger.Error(msg)
		return models.User{}, "", users.ErrorBadRequest
	}

	if newPassword == oldPassword {
		logger.Error("passwords are equal")
		return models.User{}, "", users.ErrorBadRequest
	}

	neededUser.Version += 1

	err = uc.userRepo.UpdateUserPassword(ctx, neededUser.Version, neededUser.ID, HashPass(newPassword))
	if err != nil {
		return models.User{}, "", err
	}

	neededUser.PasswordHash = HashPass(newPassword)
	neededUser.UpdatedAt = time.Now().UTC()

	token, err := uc.GenerateToken(neededUser.ID, neededUser.Login, neededUser.Version)
	if err != nil {
		return models.User{}, "", err
	}

	return neededUser, token, nil
}

func (uc *UserUsecase) UploadDance(ctx context.Context, buffer []byte, fileFormat string) (resultKey string, segmentsKey string, numFrames int, numSegments int, durationSec float64, err error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var danceExtension string
	switch fileFormat {
	case "video/mp4":
		danceExtension = ".mp4"
	case "video/quicktime":
		danceExtension = ".mov"
	default:
		logger.Error("invalid format of file")
		return "", "", 0, 0, 0.0, users.ErrorBadRequest
	}

	dancePath, err := uc.storageRepo.UploadDance(ctx, buffer, fileFormat, danceExtension)
	if err != nil {
		logger.Error("failed to upload dance", "error", err)
		return "", "", 0, 0, 0.0, users.ErrorInternalServerError
	}
	logger.Info("video uploaded to S3", "path", dancePath)

	processingURL := os.Getenv("ML_SERVICE_URL") + "/process"

	requestBody := map[string]string{
		"bucket":    os.Getenv("AWS_S3_BUCKET"),
		"video_key": dancePath,
	}

	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		logger.Error("failed to marshal request", "error", err)
		return dancePath, "", 0, 0, 0.0, users.ErrorInternalServerError
	}

	logger.Info("sending request to processing service", "url", processingURL)

	client := &http.Client{
		Timeout: 300 * time.Second,
	}

	resp, err := client.Post(processingURL, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		logger.Error("failed to call processing service", "error", err)
		return dancePath, "", 0, 0, 0.0, users.ErrorInternalServerError
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("failed to read processing service response", "error", err)
		return dancePath, "", 0, 0, 0.0, users.ErrorInternalServerError
	}

	if resp.StatusCode != http.StatusOK {
		logger.Error("processing service returned error",
			"status", resp.StatusCode,
			"body", string(body))
		return dancePath, "", 0, 0, 0.0, users.ErrorInternalServerError
	}

	var processingResponse struct {
		ResultKey   string  `json:"result_key"`
		SegmentsKey string  `json:"segments_key"`
		NumFrames   int     `json:"num_frames"`
		NumSegments int     `json:"num_segments"`
		DurationSec float64 `json:"duration_sec"`
	}

	if err := json.Unmarshal(body, &processingResponse); err != nil {
		logger.Error("failed to parse processing service response", "error", err)
		return dancePath, "", 0, 0, 0.0, users.ErrorInternalServerError
	}

	logger.Info("processing completed successfully",
		"result_key", processingResponse.ResultKey,
		"segments_key", processingResponse.SegmentsKey,
		"num_frames", processingResponse.NumFrames,
		"num_segments", processingResponse.NumSegments,
		"duration_sec", processingResponse.DurationSec)

	return processingResponse.ResultKey,
		processingResponse.SegmentsKey,
		processingResponse.NumFrames,
		processingResponse.NumSegments,
		processingResponse.DurationSec,
		nil
}
