package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
	uuid "github.com/satori/go.uuid"
	"golang.org/x/crypto/argon2"
)

func HashPass(plainPassword string) ([]byte, error) {
	salt := make([]byte, 8)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	hashedPass := argon2.IDKey([]byte(plainPassword), []byte(salt), 1, 64*1024, 4, 32)
	return append(salt, hashedPass...), nil
}

func CheckPass(passHash []byte, plainPassword string) bool {
	salt := make([]byte, 8)
	copy(salt, passHash[:8])
	userHash := argon2.IDKey([]byte(plainPassword), salt, 1, 64*1024, 4, 32)
	userHashedPassword := append(salt, userHash...)
	return subtle.ConstantTimeCompare(userHashedPassword, passHash) == 1
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
		"exp":     time.Now().Add(time.Hour * 12).Unix(),
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

const mlInternalTokenHeader = "X-Internal-Token"

func mlPost(ctx context.Context, client *http.Client, url string, jsonBody []byte) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("ML_INTERNAL_TOKEN"); token != "" {
		req.Header.Set(mlInternalTokenHeader, token)
	}
	return client.Do(req)
}

func mlGet(ctx context.Context, client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if token := os.Getenv("ML_INTERNAL_TOKEN"); token != "" {
		req.Header.Set(mlInternalTokenHeader, token)
	}
	return client.Do(req)
}

func (uc *UserUsecase) AddToHistory(ctx context.Context, userID uuid.UUID, danceID string, sourceURL string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	err := uc.userRepo.AddToHistory(ctx, userID, danceID, sourceURL)
	if err != nil {
		logger.Error("failed to add to history", "error", err)
		return err
	}

	if err := uc.userRepo.CleanHistory(ctx, userID); err != nil {
		logger.Error("failed to clean history", "error", err)
	}

	return nil
}

func (uc *UserUsecase) GetHistory(ctx context.Context, userID uuid.UUID) ([]models.SearchHistoryItem, error) {
	return uc.userRepo.GetHistory(ctx, userID)
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
		logger.Error("token version mismatch")
		return models.User{}, users.ErrorUnauthorized
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

	passwordHash, err := HashPass(newPassword)
	if err != nil {
		logger.Error("cannot hash password")
		return models.User{}, "", users.ErrorInternalServerError
	}

	err = uc.userRepo.UpdateUserPassword(ctx, neededUser.Version, neededUser.ID, passwordHash)
	if err != nil {
		return models.User{}, "", err
	}

	neededUser.PasswordHash = passwordHash
	neededUser.UpdatedAt = time.Now().UTC()

	token, err := uc.GenerateToken(neededUser.ID, neededUser.Login, neededUser.Version)
	if err != nil {
		return models.User{}, "", err
	}

	return neededUser, token, nil
}

func (uc *UserUsecase) UpdateProfile(ctx context.Context, id uuid.UUID, newLogin *string, avatarBuffer []byte, avatarContentType string) (models.User, string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	current, err := uc.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return models.User{}, "", err
	}

	var loginPtr *string
	bumpVersion := false
	if newLogin != nil {
		trimmed := strings.TrimSpace(*newLogin)
		if trimmed != current.Login {
			if !isValidLogin(trimmed) {
				logger.Error("invalid login format")
				return models.User{}, "", users.ErrorBadRequest
			}
			loginPtr = &trimmed
			bumpVersion = true
		}
	}

	var avatarPtr *string
	if len(avatarBuffer) > 0 {
		if avatarContentType != "image/jpeg" && avatarContentType != "image/png" && avatarContentType != "image/webp" {
			logger.Error("invalid avatar content type", "ct", avatarContentType)
			return models.User{}, "", users.ErrorBadRequest
		}
		key, err := uc.storageRepo.UploadAvatar(ctx, avatarBuffer, avatarContentType, id.String())
		if err != nil {
			logger.Error("failed to upload avatar", "error", err)
			return models.User{}, "", users.ErrorInternalServerError
		}
		avatarPtr = &key
	}

	if loginPtr == nil && avatarPtr == nil {
		return current, "", nil
	}

	if err := uc.userRepo.UpdateUserProfile(ctx, id, loginPtr, avatarPtr, bumpVersion); err != nil {
		return models.User{}, "", err
	}

	updated, err := uc.userRepo.GetUserByID(ctx, id)
	if err != nil {
		return models.User{}, "", err
	}

	var newToken string
	if bumpVersion {
		newToken, err = uc.GenerateToken(updated.ID, updated.Login, updated.Version)
		if err != nil {
			return models.User{}, "", err
		}
	}

	return updated, newToken, nil
}

func (uc *UserUsecase) callModerate(ctx context.Context, videoS3Key, danceID, uploaderUserID, uploaderLogin string) (string, error) {
	moderateURL := os.Getenv("ML_SERVICE_URL") + "/moderate"
	requestBody := map[string]string{
		"video_s3_url":     videoS3Key,
		"dance_id":         danceID,
		"uploader_user_id": uploaderUserID,
		"uploader_login":   uploaderLogin,
	}

	jsonBody, _ := json.Marshal(requestBody)
	client := &http.Client{Timeout: 100 * time.Second}

	resp, err := mlPost(ctx, client, moderateURL, jsonBody)
	if err != nil {
		return "", fmt.Errorf("moderation request failed: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Status string `json:"status"`
		Reason string `json:"reason"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("failed to decode moderation response: %w", err)
	}

	if result.Status == "pending" {
		return result.Reason, users.ErrorModerationPending
	}
	return "", nil
}

func (uc *UserUsecase) linkUploaderIfPresent(ctx context.Context, danceID, uploaderUserID string, logger *slog.Logger) {
	if uploaderUserID == "" {
		return
	}
	userUUID, err := uuid.FromString(uploaderUserID)
	if err != nil {
		logger.Warn("invalid uploader user id, skipping upload link", "user_id", uploaderUserID, "error", err)
		return
	}
	if err := uc.userRepo.LinkDanceUpload(ctx, userUUID, danceID); err != nil {
		logger.Warn("failed to link dance upload", "dance_id", danceID, "user_id", uploaderUserID, "error", err)
	}
}

func (uc *UserUsecase) UploadDance(
	ctx context.Context,
	buffer []byte,
	fileFormat string,
	uploaderUserID string,
	uploaderLogin string,
) (*models.UploadDanceResult, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var danceExtension string
	switch fileFormat {
	case "video/mp4":
		danceExtension = ".mp4"
	case "video/quicktime":
		danceExtension = ".mov"
	default:
		logger.Error("invalid format of file")
		return nil, users.ErrorBadRequest
	}

	dancePath, err := uc.storageRepo.UploadDance(ctx, buffer, fileFormat, danceExtension)
	if err != nil {
		logger.Error("failed to upload dance", "error", err)
		return nil, users.ErrorInternalServerError
	}

	danceID := uuid.NewV4().String()
	logger.Info("video uploaded to S3", "path", dancePath, "dance_id", danceID)

	if reason, err := uc.callModerate(ctx, dancePath, danceID, uploaderUserID, uploaderLogin); err != nil {
		if err == users.ErrorModerationPending {
			if dbErr := uc.userRepo.CreateDance(ctx, danceID, "", "pending", "medium", dancePath); dbErr != nil {
				logger.Error("failed to create pending dance record", "error", dbErr)
			}
			if reason != "" {
				if rErr := uc.userRepo.SetDanceModerationReason(ctx, danceID, reason); rErr != nil {
					logger.Warn("failed to save moderation reason", "dance_id", danceID, "error", rErr)
				}
			}
			uc.linkUploaderIfPresent(ctx, danceID, uploaderUserID, logger)
			return &models.UploadDanceResult{
				DanceID:          danceID,
				ModerationReason: reason,
			}, users.ErrorModerationPending
		}
		logger.Warn("pre-moderation check failed, proceeding", "error", err)
	}

	taskID, err := uc.enqueueProcessing(ctx, dancePath, danceID, uploaderUserID)
	if err != nil {
		logger.Error("failed to enqueue processing", "error", err)
		return nil, users.ErrorInternalServerError
	}

	if dbErr := uc.userRepo.CreateDance(ctx, danceID, "", "processing", "medium", dancePath); dbErr != nil {
		logger.Error("failed to create processing dance record", "error", dbErr)
	}
	uc.linkUploaderIfPresent(ctx, danceID, uploaderUserID, logger)

	return &models.UploadDanceResult{
		DanceID: danceID,
		TaskID:  taskID,
	}, nil
}

func (uc *UserUsecase) FinalizeUploadTask(ctx context.Context, danceID string, result *models.ProcessingResult, uploaderUserID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	status, err := uc.userRepo.GetDanceStatus(ctx, danceID)
	if err != nil {
		return err
	}
	if status == "private" || status == "published" {
		return nil
	}
	if result.Status == "moderation_pending" {
		if rErr := uc.userRepo.UpdateDanceStatus(ctx, danceID, "pending"); rErr != nil {
			logger.Warn("failed to set pending status", "dance_id", danceID, "error", rErr)
		}
		if result.Reason != "" {
			if rErr := uc.userRepo.SetDanceModerationReason(ctx, danceID, result.Reason); rErr != nil {
				logger.Warn("failed to save moderation reason", "dance_id", danceID, "error", rErr)
			}
		}
		return users.ErrorModerationPending
	}
	if dbErr := uc.userRepo.UpdateDanceStatus(ctx, danceID, "private"); dbErr != nil {
		logger.Error("failed to set dance private", "dance_id", danceID, "error", dbErr)
		return dbErr
	}
	if uploaderUserID != "" {
		uc.linkUploaderIfPresent(ctx, danceID, uploaderUserID, logger)
	}
	return nil
}

func (uc *UserUsecase) enqueueProcessing(ctx context.Context, videoKey, danceID, uploaderUserID string) (string, error) {
	processingURL := os.Getenv("ML_SERVICE_URL") + "/process"
	requestBody := map[string]interface{}{
		"video_key":        videoKey,
		"dance_id":         danceID,
		"enable_labeling":  true,
		"uploader_user_id": uploaderUserID,
	}

	jsonBody, _ := json.Marshal(requestBody)
	client := &http.Client{Timeout: 20 * time.Second}

	resp, err := mlPost(ctx, client, processingURL, jsonBody)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var response struct {
		TaskID string `json:"task_id"`
	}
	json.NewDecoder(resp.Body).Decode(&response)
	return response.TaskID, nil
}

func (uc *UserUsecase) waitForProcessing(ctx context.Context, taskID string, logger *slog.Logger) (*models.ProcessingResult, error) {
	statusURL := os.Getenv("ML_SERVICE_URL") + "/status/" + taskID

	deadline := time.Now().Add(10 * time.Minute)
	client := &http.Client{Timeout: 10 * time.Second}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}

		resp, err := mlGet(ctx, client, statusURL)
		if err != nil {
			logger.Warn("status check failed", "error", err)
			continue
		}

		var status struct {
			Status string                   `json:"status"`
			Result *models.ProcessingResult `json:"result,omitempty"`
		}
		json.NewDecoder(resp.Body).Decode(&status)
		resp.Body.Close()

		logger.Info("processing status", "status", status.Status)

		switch status.Status {
		case "done":
			if status.Result != nil && status.Result.Status == "moderation_pending" {
				return status.Result, users.ErrorModerationPending
			}
			return status.Result, nil
		case "failed":
			return nil, fmt.Errorf("processing failed")
		}
	}

	return nil, fmt.Errorf("processing timeout")
}

func (uc *UserUsecase) UploadDanceByURL(
	ctx context.Context,
	videoURL string,
	uploaderUserID string,
	uploaderLogin string,
) (*models.UploadDanceResult, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	danceID := uuid.NewV4().String()
	taskID, err := uc.enqueueProcessingByURL(ctx, videoURL, danceID, uploaderUserID)
	if err != nil {
		logger.Error("failed to enqueue processing", "error", err)
		return nil, users.ErrorInternalServerError
	}

	if dbErr := uc.userRepo.CreateDance(ctx, danceID, "", "processing", "medium", ""); dbErr != nil {
		logger.Error("failed to create processing dance record", "error", dbErr)
	}
	uc.linkUploaderIfPresent(ctx, danceID, uploaderUserID, logger)

	return &models.UploadDanceResult{
		DanceID: danceID,
		TaskID:  taskID,
	}, nil
}

func (uc *UserUsecase) enqueueProcessingByURL(ctx context.Context, videoURL, danceID, uploaderUserID string) (string, error) {
	processingURL := os.Getenv("ML_SERVICE_URL") + "/process-url/"
	requestBody := map[string]interface{}{
		"url":              videoURL,
		"dance_id":         danceID,
		"enable_labeling":  true,
		"uploader_user_id": uploaderUserID,
	}

	jsonBody, _ := json.Marshal(requestBody)
	client := &http.Client{Timeout: 20 * time.Second}

	resp, err := mlPost(ctx, client, processingURL, jsonBody)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var response struct {
		TaskID string `json:"task_id"`
	}
	json.NewDecoder(resp.Body).Decode(&response)
	return response.TaskID, nil
}

func userInUploaders(uploaders []uuid.UUID, userID *uuid.UUID) bool {
	if userID == nil {
		return false
	}
	for _, uid := range uploaders {
		if uid == *userID {
			return true
		}
	}
	return false
}

func (uc *UserUsecase) GetDanceByID(ctx context.Context, danceID string, userID *uuid.UUID) (*models.UploadDanceResult, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if status, statusErr := uc.userRepo.GetDanceStatus(ctx, danceID); statusErr != nil || status != "published" {
		uploaders, _ := uc.userRepo.GetDanceUploaders(ctx, danceID)
		if len(uploaders) > 0 && !userInUploaders(uploaders, userID) {
			logger.Warn("access to private dance denied", "dance_id", danceID)
			return nil, users.ErrorNotFound
		}
	}

	segmentsKey := fmt.Sprintf("results/%s/segments.json", danceID)

	data, err := uc.storageRepo.DownloadFile(ctx, segmentsKey)
	if err != nil {
		logger.Error("failed to get segments.json", "error", err)
		return nil, users.ErrorNotFound
	}

	var segments struct {
		DanceID     string `json:"dance_id"`
		NumSegments int    `json:"num_segments"`
		Meta        struct {
			NumFrames   int     `json:"num_frames"`
			DurationSec float64 `json:"duration_sec"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(data, &segments); err != nil {
		logger.Error("failed to parse segments.json", "error", err)
		return nil, users.ErrorInternalServerError
	}

	glbKeys := make([]string, segments.NumSegments)
	for i := 0; i < segments.NumSegments; i++ {
		glbKeys[i] = fmt.Sprintf("results/%s/segment_%d.glb", danceID, i)
	}

	result := &models.UploadDanceResult{
		DanceID:             danceID,
		SegmentsKey:         segmentsKey,
		FullGlbKey:          fmt.Sprintf("results/%s/full_animation.glb", danceID),
		GlbKeys:             glbKeys,
		NumFrames:           segments.Meta.NumFrames,
		NumSegments:         segments.NumSegments,
		NumSegmentsRendered: segments.NumSegments,
		DurationSec:         segments.Meta.DurationSec,
		VideoPath:           fmt.Sprintf("results/%s/video.mp4", danceID),
	}

	count, err := uc.userRepo.GetLikesCount(ctx, danceID)
	if err == nil {
		result.LikesCount = count
	}

	if author, authErr := uc.userRepo.GetDanceAuthor(ctx, danceID); authErr != nil {
		logger.Warn("failed to get dance author", "dance_id", danceID, "error", authErr)
	} else {
		result.Author = author
	}

	if diff, byUsers, dErr := uc.userRepo.GetEffectiveDanceDifficulty(ctx, danceID); dErr != nil {
		logger.Warn("failed to get effective difficulty", "dance_id", danceID, "error", dErr)
	} else {
		result.Difficulty = diff
		result.DifficultyByUsers = byUsers
	}

	if userID != nil {
		liked, err := uc.userRepo.IsLikedByUser(ctx, *userID, danceID)
		if err == nil {
			result.IsLiked = liked
		}

		if err := uc.userRepo.AddToHistory(ctx, *userID, danceID, ""); err != nil {
			logger.Warn("failed to add to history", "error", err)

		}
		if last, lastErr := uc.userRepo.GetLastAttempt(ctx, *userID, danceID); lastErr == nil && last != nil {
			result.LastAttemptID = last.AttemptID
			score := last.Score
			result.LastAttemptScore = &score
		}
	}

	return result, nil
}

func (uc *UserUsecase) GetMainPage(ctx context.Context) ([]models.VideoItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	topDances, err := uc.userRepo.GetTopLikedDances(ctx, 14)
	logger.Info("got top liked dances", "count", len(topDances))
	if err != nil {
		logger.Error("failed to get top dances", "error", err)
		return nil, users.ErrorInternalServerError
	}

	s3Address := strings.TrimRight(os.Getenv("S3_ADDRESS"), "/")
	var videos []models.VideoItem
	ids := make([]string, 0, len(topDances))

	for _, d := range topDances {
		ids = append(ids, d.DanceID)
		videos = append(videos, models.VideoItem{
			ID:  d.DanceID,
			URL: fmt.Sprintf("%s/results/%s/video.mp4", s3Address, d.DanceID),
		})
	}

	if len(videos) < 14 {
		danceIDs, err := uc.storageRepo.ListDances(ctx, 14)
		if err != nil {
			logger.Error("failed to list dances", "error", err)
			return nil, users.ErrorInternalServerError
		}
		existing := make(map[string]bool)
		for _, v := range videos {
			existing[v.ID] = true
		}
		for _, id := range danceIDs {
			if !existing[id] && len(videos) < 14 {
				ids = append(ids, id)
				videos = append(videos, models.VideoItem{
					ID:  id,
					URL: fmt.Sprintf("%s/results/%s/video.mp4", s3Address, id),
				})
			}
		}
	}
	published, err := uc.userRepo.GetPublishedDanceIDs(ctx, ids)
	if err != nil {
		logger.Error("failed to filter published dances", "error", err)
		return nil, users.ErrorInternalServerError
	}
	filteredVideos := make([]models.VideoItem, 0, len(videos))
	filteredIDs := make([]string, 0, len(ids))
	for _, v := range videos {
		if _, ok := published[v.ID]; ok {
			filteredVideos = append(filteredVideos, v)
			filteredIDs = append(filteredIDs, v.ID)
		}
	}
	videos = filteredVideos
	ids = filteredIDs

	if len(ids) > 0 {
		enriched, err := uc.userRepo.GetDancesEnrichedInfo(ctx, ids)
		if err == nil {
			for i, v := range videos {
				if info, ok := enriched[v.ID]; ok {
					videos[i].Title = info.Title
					videos[i].AttemptCount = info.AttemptCount
					videos[i].AvgScore = info.AvgScore
					videos[i].ViewCount = info.ViewCount
					videos[i].LikeCount = info.LikeCount
				}
			}
		}
	}

	return videos, nil
}

func (uc *UserUsecase) GetSegmentDescription(ctx context.Context, danceID string, segmentIdx int) (*models.SegmentDescriptionResult, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	mlURL := fmt.Sprintf("%s/segment_description/%s/%d", os.Getenv("ML_SERVICE_URL"), danceID, segmentIdx)
	client := &http.Client{Timeout: 120 * time.Second}

	resp, err := mlGet(ctx, client, mlURL)
	if err != nil {
		logger.Error("failed to call ml service", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, users.ErrorNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, users.ErrorInternalServerError
	}

	var result models.SegmentDescriptionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Error("failed to decode response", "error", err)
		return nil, users.ErrorInternalServerError
	}

	return &result, nil
}

func (uc *UserUsecase) DeleteFromHistory(ctx context.Context, historyID uuid.UUID, userID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if historyID == uuid.Nil || userID == uuid.Nil {
		logger.Error("invalid historyID or userID")
		return users.ErrorBadRequest
	}

	err := uc.userRepo.DeleteFromHistory(ctx, historyID, userID)
	if err != nil {
		logger.Error("failed to delete from history", "error", err)
		return err
	}

	logger.Info("successfully deleted history item")
	return nil
}

func (uc *UserUsecase) UpdateHistoryName(ctx context.Context, historyID uuid.UUID, userID uuid.UUID, name string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if historyID == uuid.Nil || userID == uuid.Nil {
		logger.Error("invalid historyID or userID")
		return users.ErrorBadRequest
	}

	name = strings.TrimSpace(name)
	if name == "" {
		logger.Error("name is empty")
		return users.ErrorBadRequest
	}
	if len(name) > 100 {
		logger.Error("name is too long")
		return users.ErrorBadRequest
	}

	err := uc.userRepo.UpdateHistoryName(ctx, historyID, userID, name)
	if err != nil {
		logger.Error("failed to update history name", "error", err)
		return err
	}

	logger.Info("successfully updated history name")
	return nil
}

func (uc *UserUsecase) ToggleLike(ctx context.Context, userID uuid.UUID, danceID string) (*models.LikeResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if danceID == "" {
		logger.Error("empty danceID")
		return nil, users.ErrorBadRequest
	}

	liked, err := uc.userRepo.ToggleLike(ctx, userID, danceID)
	if err != nil {
		return nil, err
	}

	count, err := uc.userRepo.GetLikesCount(ctx, danceID)
	if err != nil {
		return nil, err
	}

	return &models.LikeResponse{
		DanceID:    danceID,
		Liked:      liked,
		LikesCount: count,
	}, nil
}

func (uc *UserUsecase) GetTopLikedDances(ctx context.Context, limit int) ([]models.DanceLikeStat, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	if limit <= 0 || limit > 50 {
		limit = 9
	}
	stats, err := uc.userRepo.GetTopLikedDances(ctx, limit)
	if err != nil {
		logger.Error("failed to get top dances", "error", err)
		return nil, err
	}
	return stats, nil
}

func (uc *UserUsecase) CompareDance(ctx context.Context, videoKey string, danceID string, segmentIdx int) (*models.DanceCompareResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if videoKey == "" || danceID == "" {
		logger.Error("videoKey or danceID is empty")
		return nil, users.ErrorBadRequest
	}

	taskID, err := uc.enqueueCompare(ctx, videoKey, danceID, segmentIdx)
	if err != nil {
		logger.Error("failed to enqueue compare", "error", err)
		return nil, users.ErrorInternalServerError
	}

	result, err := uc.waitForCompare(ctx, taskID, logger)
	if err != nil {
		logger.Error("compare failed", "error", err)
		return nil, users.ErrorInternalServerError
	}

	return result, nil
}

func (uc *UserUsecase) enqueueCompare(ctx context.Context, videoKey, danceID string, segmentIdx int) (string, error) {
	compareURL := os.Getenv("ML_SERVICE_URL") + "/dance_compare"
	requestBody := map[string]interface{}{
		"video_key":   videoKey,
		"dance_id":    danceID,
		"segment_idx": segmentIdx,
	}

	jsonBody, _ := json.Marshal(requestBody)
	client := &http.Client{Timeout: 20 * time.Second}

	resp, err := mlPost(ctx, client, compareURL, jsonBody)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var response struct {
		TaskID string `json:"task_id"`
	}
	json.NewDecoder(resp.Body).Decode(&response)
	return response.TaskID, nil
}

func (uc *UserUsecase) waitForCompare(ctx context.Context, taskID string, logger *slog.Logger) (*models.DanceCompareResponse, error) {
	statusURL := os.Getenv("ML_SERVICE_URL") + "/status/" + taskID
	deadline := time.Now().Add(10 * time.Minute)
	client := &http.Client{Timeout: 10 * time.Second}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}

		resp, err := mlGet(ctx, client, statusURL)
		if err != nil {
			logger.Warn("status check failed", "error", err)
			continue
		}

		var status struct {
			Status string                       `json:"status"`
			Result *models.DanceCompareResponse `json:"result,omitempty"`
		}
		json.NewDecoder(resp.Body).Decode(&status)
		resp.Body.Close()

		logger.Info("compare status", "status", status.Status)

		switch status.Status {
		case "done":
			return status.Result, nil
		case "failed":
			return nil, fmt.Errorf("compare failed")
		}
	}

	return nil, fmt.Errorf("compare timeout")
}

func (uc *UserUsecase) GetUserLikedDances(ctx context.Context, userID uuid.UUID) ([]models.DanceLike, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	likes, err := uc.userRepo.GetUserLikedDances(ctx, userID)
	if err != nil {
		logger.Error("failed to get liked dances", "error", err)
		return nil, users.ErrorInternalServerError
	}
	return likes, nil
}

func (uc *UserUsecase) SaveRating(ctx context.Context, userID uuid.UUID, input models.SaveRatingInput) (*models.RatingResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if err := input.Validate(); err != nil {
		logger.Error("invalid rating input", "error", err)
		return nil, users.ErrorBadRequest
	}

	err := uc.userRepo.SaveRating(ctx, userID, input)
	if err != nil {
		logger.Error("failed to save rating", "error", err)
		return nil, users.ErrorInternalServerError
	}

	return uc.GetAggregatedRating(ctx, input.VideoID)
}

func (uc *UserUsecase) GetAggregatedRating(ctx context.Context, videoID string) (*models.RatingResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if videoID == "" {
		logger.Error("empty videoID")
		return nil, users.ErrorBadRequest
	}

	rating, err := uc.userRepo.GetAggregatedRating(ctx, videoID)
	if err != nil {
		logger.Error("failed to get aggregated rating", "error", err)
		return nil, users.ErrorInternalServerError
	}

	return rating, nil
}

func (uc *UserUsecase) CompareDanceFromBuffer(ctx context.Context, buffer []byte, fileFormat string, referenceDanceID string, userID *uuid.UUID) (*models.CompareResult, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	attemptID := uuid.NewV4().String()
	userDanceID := attemptID
	s3UserID := attemptID
	mlUserID := attemptID
	if userID != nil {
		s3UserID = userID.String()
		mlUserID = userID.String()
	}
	videoKey := fmt.Sprintf("users/%s/%s/video.mp4", s3UserID, attemptID)

	tmpFile, err := os.CreateTemp("", "compare-input-*.mp4")
	if err != nil {
		return nil, users.ErrorInternalServerError
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(buffer); err != nil {
		return nil, users.ErrorInternalServerError
	}
	tmpFile.Close()

	if err := uc.storageRepo.UploadFileRaw(ctx, tmpFile.Name(), videoKey); err != nil {
		logger.Error("failed to upload user video", "error", err)
		return nil, users.ErrorInternalServerError
	}

	mlURL := os.Getenv("ML_SERVICE_URL") + "/dance_compare"
	requestBody := map[string]string{
		"original_video_s3_path": fmt.Sprintf("results/%s/video.mp4", referenceDanceID),
		"user_video_s3_path":     videoKey,
		"user_id":                mlUserID,
		"dance_id":               referenceDanceID,
		"attempt_id":             attemptID,
	}

	jsonBody, _ := json.Marshal(requestBody)
	client := &http.Client{Timeout: 20 * time.Second}

	resp, err := mlPost(ctx, client, mlURL, jsonBody)
	if err != nil {
		logger.Error("failed to call ml compare", "error", err)
		uc.storageRepo.DeleteFile(ctx, videoKey)
		return nil, users.ErrorInternalServerError
	}
	defer resp.Body.Close()

	var taskResp struct {
		TaskID  string `json:"task_id"`
		DanceID string `json:"dance_id"`
		UserID  string `json:"user_id"`
		Status  string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&taskResp); err != nil {
		uc.storageRepo.DeleteFile(ctx, videoKey)
		return nil, users.ErrorInternalServerError
	}

	if taskResp.TaskID != "" {
		if err := uc.userRepo.CreateCompareTask(ctx, taskResp.TaskID, referenceDanceID, userDanceID, videoKey); err != nil {
			logger.Error("failed to persist compare task metadata", "error", err)
		}
	}

	return &models.CompareResult{
		TaskID:               taskResp.TaskID,
		UserDanceID:          userDanceID,
		DanceID:              referenceDanceID,
		UserVideoKey:         videoKey,
		ReferenceSkeletonKey: fmt.Sprintf("results/%s/skeleton.json", referenceDanceID),
	}, nil
}

func (uc *UserUsecase) FinalizeCompareTask(ctx context.Context, mlResult *models.CompareStatusResult, referenceDanceID, userDanceID, videoKey string, userID *uuid.UUID, recordAttempt bool) (*models.CompareResult, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if recordAttempt && userID != nil {
		if recErr := uc.userRepo.RecordDanceAttempt(ctx, referenceDanceID, userID, userDanceID, mlResult.ComparisonScore); recErr != nil {
			logger.Warn("failed to record dance attempt", "error", recErr)
		}
	}

	referenceGlbKey := fmt.Sprintf("results/%s/full_animation.glb", referenceDanceID)
	if !uc.storageRepo.FileExists(ctx, referenceGlbKey) {
		referenceGlbKey = fmt.Sprintf("results/%s/segment_0.glb", referenceDanceID)
	}

	s3UserID := userDanceID
	if userID != nil {
		s3UserID = userID.String()
	}

	out := &models.CompareResult{
		UserGlbKey:           mlResult.UserGlbS3,
		ReferenceGlbKey:      referenceGlbKey,
		Score:                mlResult.ComparisonScore,
		DtwDistance:          mlResult.DtwDistance,
		DanceID:              referenceDanceID,
		UserDanceID:          userDanceID,
		Segments:             mlResult.Segments,
		Tips:                 mlResult.Tips,
		FrameScores:          mlResult.FrameScores,
		UserVideoKey:         videoKey,
		UserSkeletonKey:      fmt.Sprintf("users/%s/%s/skeleton.json", s3UserID, userDanceID),
		ReferenceSkeletonKey: fmt.Sprintf("results/%s/skeleton.json", referenceDanceID),
	}

	if stats, err := uc.userRepo.GetDanceStats(ctx, referenceDanceID); err == nil {
		out.DanceStats = &models.CompareDanceStats{
			AttemptCount: stats.AttemptCount,
			BestScore:    stats.TopScore,
		}
	}

	return out, nil
}

func (uc *UserUsecase) generateCompareTips(ctx context.Context, score float64, segments []models.SegmentDiagnostic) ([]models.CompareTip, error) {
	mlURL := os.Getenv("ML_SERVICE_URL") + "/compare_tips"

	segPayload := make([]map[string]interface{}, 0, len(segments))
	for _, s := range segments {
		segPayload = append(segPayload, map[string]interface{}{
			"segment_id":    s.SegmentID,
			"label":         s.Label,
			"score":         s.SegmentScore,
			"timing":        s.TimingScore,
			"amplitude":     s.AmplitudeScore,
			"pose_accuracy": s.PoseAccuracyScore,
			"feedback":      s.Feedback,
		})
	}

	body := map[string]interface{}{
		"attempt_score": score,
		"segments":      segPayload,
	}
	jsonBody, _ := json.Marshal(body)

	client := &http.Client{Timeout: 90 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, mlURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("build tips request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token := os.Getenv("ML_INTERNAL_TOKEN"); token != "" {
		req.Header.Set(mlInternalTokenHeader, token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tips request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tips endpoint returned %d", resp.StatusCode)
	}

	var parsed struct {
		Tips []models.CompareTip `json:"tips"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode tips response: %w", err)
	}
	return parsed.Tips, nil
}

func (uc *UserUsecase) waitForCompareResult(ctx context.Context, taskID string, logger *slog.Logger) (*models.CompareStatusResult, error) {
	statusURL := os.Getenv("ML_SERVICE_URL") + "/status/" + taskID
	deadline := time.Now().Add(10 * time.Minute)
	client := &http.Client{Timeout: 10 * time.Second}

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}

		resp, err := mlGet(ctx, client, statusURL)
		if err != nil {
			logger.Warn("compare status check failed", "error", err)
			continue
		}

		var statusResp struct {
			Status string                      `json:"status"`
			Result *models.CompareStatusResult `json:"result,omitempty"`
		}
		json.NewDecoder(resp.Body).Decode(&statusResp)
		resp.Body.Close()

		switch statusResp.Status {
		case "done":
			if statusResp.Result == nil {
				return nil, users.ErrorInternalServerError
			}
			if statusResp.Result.Success != nil && !*statusResp.Result.Success {
				return nil, fmt.Errorf("comparison pipeline error: %s", statusResp.Result.Error)
			}
			return statusResp.Result, nil
		case "failed":
			return nil, fmt.Errorf("comparison failed")
		}
	}
	return nil, fmt.Errorf("comparison timeout")
}

func (uc *UserUsecase) GetTaskStatus(
	ctx context.Context,
	taskID string,
	taskType string,
	danceID string,
	userDanceID string,
	videoKey string,
	uploaderUserID string,
	userID *uuid.UUID,
) (*models.TaskStatusResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	mlURL := os.Getenv("ML_SERVICE_URL") + "/status/" + taskID
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := mlGet(ctx, client, mlURL)
	if err != nil {
		logger.Error("failed to get task status from ML", "error", err)
		return nil, users.ErrorInternalServerError
	}
	defer resp.Body.Close()

	var mlStatus models.MlStatusResp
	if err := json.NewDecoder(resp.Body).Decode(&mlStatus); err != nil {
		return nil, users.ErrorInternalServerError
	}

	out := &models.TaskStatusResponse{
		Status:     mlStatus.Status,
		Stage:      mlStatus.Stage,
		StageLabel: mlStatus.StageLabel,
		Progress:   mlStatus.Progress,
	}

	if mlStatus.Status != "done" {
		if mlStatus.Error != "" {
			out.Error = mlStatus.Error
		}
		return out, nil
	}

	switch taskType {
	case "upload":
		var mlResult models.ProcessingResult
		if err := json.Unmarshal(mlStatus.Result, &mlResult); err != nil {
			logger.Error("failed to unmarshal upload result", "error", err)
			return nil, users.ErrorInternalServerError
		}

		if finErr := uc.FinalizeUploadTask(ctx, danceID, &mlResult, uploaderUserID); finErr != nil && finErr != users.ErrorModerationPending {
			logger.Warn("FinalizeUploadTask error", "error", finErr)
		}

		loadResp := models.LoadDanceResponse{
			DanceID:             mlResult.DanceID,
			FullGlbKey:          mlResult.FullGlbKey,
			GlbKeys:             mlResult.GlbKeys,
			SegmentsKey:         mlResult.SegmentsKey,
			NumFrames:           mlResult.NumFrames,
			NumSegments:         mlResult.NumSegments,
			NumSegmentsRendered: mlResult.NumSegmentsRendered,
			DurationSec:         mlResult.DurationSec,
			VideoPath:           mlResult.VideoPath,
		}
		resultBytes, _ := json.Marshal(loadResp)
		out.Result = resultBytes

	case "compare":
		var mlResult models.CompareStatusResult
		if err := json.Unmarshal(mlStatus.Result, &mlResult); err != nil {
			logger.Error("failed to unmarshal compare result", "error", err)
			return nil, users.ErrorInternalServerError
		}

		finDanceID, finUserDanceID, finVideoKey := danceID, userDanceID, videoKey
		recordAttempt := false
		if task, taskErr := uc.userRepo.GetCompareTask(ctx, taskID); taskErr == nil {
			finDanceID = task.DanceID
			finUserDanceID = task.UserDanceID
			finVideoKey = task.VideoKey
			claimed, claimErr := uc.userRepo.MarkCompareTaskFinalized(ctx, taskID)
			if claimErr != nil {
				logger.Warn("failed to claim compare task finalization", "error", claimErr)
			}
			recordAttempt = claimed
		} else {
			logger.Warn("compare task metadata not found, finalizing without recording attempt", "task_id", taskID)
		}

		compareResult, finErr := uc.FinalizeCompareTask(ctx, &mlResult, finDanceID, finUserDanceID, finVideoKey, userID, recordAttempt)
		if finErr != nil {
			logger.Warn("FinalizeCompareTask error", "error", finErr)
		}
		if compareResult != nil {
			resultBytes, _ := json.Marshal(compareResult)
			out.Result = resultBytes
		}
	}

	return out, nil
}

func (uc *UserUsecase) GetDanceCatalog(ctx context.Context, sort, search string, page, limit int) (*models.DanceCatalogResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 48 {
		limit = 12
	}

	switch sort {
	case "new":
		sort = "newest"
	case "easiest":
		sort = "easy"
	case "hardest":
		sort = "hard"
	}

	items, err := uc.userRepo.GetDanceCatalog(ctx, sort, search, page, limit)
	if err != nil {
		logger.Error("failed to get dance catalog", "error", err)
		return nil, err
	}

	total, err := uc.userRepo.GetDanceCatalogCount(ctx, sort, search)
	if err != nil {
		logger.Error("failed to get dance catalog count", "error", err)
		return nil, err
	}

	s3Address := strings.TrimRight(os.Getenv("S3_ADDRESS"), "/")
	for i := range items {
		items[i].URL = fmt.Sprintf("%s/results/%s/video.mp4", s3Address, items[i].ID)
	}

	hasMore := (page * limit) < total

	return &models.DanceCatalogResponse{
		Count:  total,
		Dances: items,
		Pagination: models.PaginationMeta{
			Page:    page,
			Limit:   limit,
			Total:   total,
			HasMore: hasMore,
		},
	}, nil
}

func (uc *UserUsecase) GetDanceTrending(ctx context.Context) (*models.TrendingResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	items, err := uc.userRepo.GetDanceTrending(ctx)
	if err != nil {
		logger.Error("failed to get trending dances", "error", err)
		return nil, err
	}

	s3Address := strings.TrimRight(os.Getenv("S3_ADDRESS"), "/")
	videos := make([]models.VideoItem, 0, len(items))
	for _, item := range items {
		videos = append(videos, models.VideoItem{
			ID:           item.ID,
			URL:          fmt.Sprintf("%s/results/%s/video.mp4", s3Address, item.ID),
			Title:        item.Title,
			AttemptCount: item.AttemptCount,
			AvgScore:     item.AvgScore,
			ViewCount:    item.ViewCount,
			LikeCount:    item.LikeCount,
		})
	}

	return &models.TrendingResponse{
		Count:  len(videos),
		Videos: videos,
	}, nil
}

func (uc *UserUsecase) GetDanceStats(ctx context.Context, danceID string) (*models.DanceStats, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	stats, err := uc.userRepo.GetDanceStats(ctx, danceID)
	if err != nil {
		logger.Error("failed to get dance stats", "error", err)
		return nil, err
	}

	return stats, nil
}

func (uc *UserUsecase) GetDanceTimeline(ctx context.Context, danceID string, userID string) ([]byte, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	key := fmt.Sprintf("users/%s/%s/timeline.json", userID, danceID)
	data, err := uc.storageRepo.DownloadFile(ctx, key)
	if err != nil {
		if isS3NotFoundError(err) {
			logger.Warn("timeline not found", "key", key)
			return nil, users.ErrorNotFound
		}
		logger.Error("failed to get timeline from storage", "error", err)
		return nil, users.ErrorInternalServerError
	}

	return data, nil
}

func (uc *UserUsecase) GetDanceKeyframes(ctx context.Context, danceID string) ([]byte, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	key := fmt.Sprintf("results/%s/keyframes.json", danceID)
	data, err := uc.storageRepo.DownloadFile(ctx, key)
	if err != nil {
		if isS3NotFoundError(err) {
			logger.Warn("keyframes not found", "key", key)
			return nil, users.ErrorNotFound
		}
		logger.Error("failed to get keyframes from storage", "error", err)
		return nil, users.ErrorInternalServerError
	}

	return data, nil
}

func (uc *UserUsecase) UpdateDanceStatus(ctx context.Context, id, status string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var dbStatus string
	switch status {
	case "approved":
		dbStatus = "published"
	case "rejected":
		dbStatus = "rejected"
	default:
		logger.Error("invalid dance status", "status", status)
		return users.ErrorBadRequest
	}

	currentStatus, err := uc.userRepo.GetDanceStatus(ctx, id)
	if err != nil {
		return err
	}

	if err := uc.userRepo.UpdateDanceStatus(ctx, id, dbStatus); err != nil {
		logger.Error("failed to update dance status", "error", err)
		return err
	}

	if dbStatus == "rejected" {
		reason, _ := uc.userRepo.GetDanceModerationReason(ctx, id)
		uc.notifyDanceUploaders(ctx, id, "dance_rejected", reason, logger)
		return nil
	}

	if currentStatus == "pending" && dbStatus == "published" {
		videoPath, err := uc.userRepo.GetDanceVideoPath(ctx, id)
		if err != nil || videoPath == "" {
			logger.Warn("approved dance has no video path, skipping processing", "dance_id", id)

			return nil
		}

		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()
			taskID, enqErr := uc.enqueueProcessing(bgCtx, videoPath, id, "")
			if enqErr != nil {
				logger.Error("failed to enqueue approved dance", "error", enqErr, "dance_id", id)
				return
			}
			if _, waitErr := uc.waitForProcessing(bgCtx, taskID, logger); waitErr != nil {
				logger.Error("approved dance processing failed", "error", waitErr, "dance_id", id)
				return
			}
			uc.notifyDanceUploaders(bgCtx, id, "dance_approved", "", logger)
		}()
	}

	return nil
}

func (uc *UserUsecase) notifyDanceUploaders(ctx context.Context, danceID, notifType, reason string, logger *slog.Logger) {
	uploaders, err := uc.userRepo.GetDanceUploaders(ctx, danceID)
	if err != nil {
		logger.Warn("failed to get dance uploaders for notification", "dance_id", danceID, "error", err)
		return
	}
	for _, uid := range uploaders {
		if err := uc.userRepo.CreateNotification(ctx, uid, notifType, danceID, reason); err != nil {
			logger.Warn("failed to create notification", "dance_id", danceID, "user_id", uid.String(), "error", err)
		}
	}
}

func (uc *UserUsecase) GetNotifications(ctx context.Context, userID uuid.UUID) (*models.NotificationsResponse, error) {
	items, err := uc.userRepo.GetNotifications(ctx, userID)
	if err != nil {
		return nil, err
	}
	unread := 0
	for _, n := range items {
		if !n.IsRead {
			unread++
		}
	}
	return &models.NotificationsResponse{
		Notifications: items,
		UnreadCount:   unread,
	}, nil
}

func (uc *UserUsecase) MarkNotificationRead(ctx context.Context, id int64, userID uuid.UUID) error {
	return uc.userRepo.MarkNotificationRead(ctx, id, userID)
}

func (uc *UserUsecase) MarkAllNotificationsRead(ctx context.Context, userID uuid.UUID) error {
	return uc.userRepo.MarkAllNotificationsRead(ctx, userID)
}

func (uc *UserUsecase) ClaimDanceUploads(ctx context.Context, userID uuid.UUID, danceIDs []string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	for _, id := range danceIDs {
		if id == "" {
			continue
		}
		if err := uc.userRepo.LinkDanceUpload(ctx, userID, id); err != nil {
			logger.Warn("failed to claim dance upload", "dance_id", id, "user_id", userID.String(), "error", err)
		}
	}
	return nil
}

func (uc *UserUsecase) GetRating(ctx context.Context, videoID string) (*models.RatingResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	rating, err := uc.userRepo.GetAggregatedRating(ctx, videoID)
	if err != nil {
		logger.Error("failed to get rating", "error", err)
		return nil, err
	}

	return rating, nil
}

func isValidLogin(login string) bool {
	if len(login) < 6 || len(login) > 15 {
		return false
	}
	for _, char := range login {
		if !strings.ContainsRune(users.ValidChars, char) {
			return false
		}
	}
	return true
}

func isS3NotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "NoSuchKey") || strings.Contains(errMsg, "404")
}

func (uc *UserUsecase) RecordDanceView(ctx context.Context, danceID string, viewerID string) error {
	if danceID == "" || viewerID == "" {
		return users.ErrorBadRequest
	}
	return uc.userRepo.RecordDanceView(ctx, danceID, viewerID)
}

func (uc *UserUsecase) SaveAttempt(ctx context.Context, userID uuid.UUID, attemptID, danceID string, includeVideo bool, fallbackScore *float64, userName string, isPrivate bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	if attemptID == "" || danceID == "" {
		return users.ErrorBadRequest
	}

	var score float64
	if fallbackScore != nil {
		score = *fallbackScore
	} else if rank, err := uc.userRepo.GetUserDanceRank(ctx, danceID, userID); err == nil && rank != nil {
		score = rank.Score
	}

	if fallbackScore != nil {
		if recErr := uc.userRepo.RecordDanceAttempt(ctx, danceID, &userID, attemptID, *fallbackScore); recErr != nil {
			logger.Warn("failed to record fallback attempt (may already exist)", "error", recErr)
		}
	}

	if err := uc.userRepo.SaveAttempt(ctx, userID, attemptID, danceID, score, includeVideo, userName, isPrivate); err != nil {
		return err
	}

	if !includeVideo {
		videoKey := fmt.Sprintf("users/%s/%s/video.mp4", userID.String(), attemptID)
		if delErr := uc.storageRepo.DeleteFile(ctx, videoKey); delErr != nil {
			logger.Warn("failed to delete user video on save-without-video", "error", delErr, "key", videoKey)
		}
	}
	return nil
}

func (uc *UserUsecase) UnsaveAttempt(ctx context.Context, userID uuid.UUID, attemptID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	if attemptID == "" {
		return users.ErrorBadRequest
	}

	hadVideo, err := uc.userRepo.IsSavedAttemptWithVideo(ctx, userID, attemptID)
	if err != nil {
		logger.Warn("failed to read has_video before unsave", "error", err)
		hadVideo = false
	}

	if err := uc.userRepo.UnsaveAttempt(ctx, userID, attemptID); err != nil {
		return err
	}

	if hadVideo {
		videoKey := fmt.Sprintf("users/%s/%s/video.mp4", userID.String(), attemptID)
		if delErr := uc.storageRepo.DeleteFile(ctx, videoKey); delErr != nil {
			logger.Warn("failed to delete user video on unsave", "error", delErr, "key", videoKey)
		}
	}
	return nil
}

func (uc *UserUsecase) CleanupExpiredUserVideos(ctx context.Context, ttl time.Duration) (int, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	videos, err := uc.storageRepo.ListUserVideos(ctx)
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().Add(-ttl)
	deleted := 0
	for _, v := range videos {
		if v.LastModified.After(cutoff) {
			continue
		}
		userUUID, parseErr := uuid.FromString(v.UserID)
		var protected bool
		if parseErr == nil {
			protected, err = uc.userRepo.IsSavedAttemptWithVideo(ctx, userUUID, v.DanceID)
			if err != nil {
				logger.Warn("cleanup: failed to check saved attempt", "error", err, "key", v.Key)
				continue
			}
		}
		if protected {
			continue
		}
		if delErr := uc.storageRepo.DeleteFile(ctx, v.Key); delErr != nil {
			logger.Warn("cleanup: failed to delete video", "error", delErr, "key", v.Key)
			continue
		}
		deleted++
	}
	if deleted > 0 {
		logger.Info("cleanup: deleted expired user videos", "count", deleted, "ttl", ttl.String())
	}
	return deleted, nil
}

func (uc *UserUsecase) GetSavedAttempts(ctx context.Context, userID uuid.UUID) ([]models.SavedAttemptItem, error) {
	return uc.userRepo.GetSavedAttempts(ctx, userID)
}

func (uc *UserUsecase) GetUserAttempts(ctx context.Context, userID uuid.UUID) ([]models.UserAttemptItem, error) {
	return uc.userRepo.GetUserAttempts(ctx, userID)
}

func (uc *UserUsecase) GetPublicProfile(ctx context.Context, profileUserID uuid.UUID, viewerUserID *uuid.UUID) (*models.PublicProfileResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	user, err := uc.userRepo.GetUserByID(ctx, profileUserID)
	if err != nil {
		return nil, err
	}

	saved, err := uc.userRepo.GetSavedAttempts(ctx, profileUserID)
	if err != nil {
		logger.Warn("failed to load saved attempts", "error", err)
		saved = []models.SavedAttemptItem{}
	}

	top, err := uc.userRepo.GetPersonalTop(ctx, profileUserID)
	if err != nil {
		logger.Warn("failed to load personal top", "error", err)
		top = []models.PersonalTopItem{}
	}

	isOwn := viewerUserID != nil && *viewerUserID == profileUserID

	if !isOwn {
		filtered := saved[:0]
		for _, s := range saved {
			if !s.IsPrivate {
				filtered = append(filtered, s)
			}
		}
		saved = filtered
	}

	friendsCount, _ := uc.userRepo.GetFriendsCount(ctx, profileUserID)

	var friendshipStatus *models.FriendshipStatus
	if viewerUserID != nil && !isOwn {
		friendshipStatus, _ = uc.userRepo.GetFriendshipBetween(ctx, *viewerUserID, profileUserID)
	}

	return &models.PublicProfileResponse{
		User: models.PublicProfileUser{
			ID:        user.ID,
			Login:     user.Login,
			Avatar:    user.Avatar,
			UpdatedAt: user.UpdatedAt,
		},
		SavedAttempts:    saved,
		PersonalTop:      top,
		IsOwnProfile:     isOwn,
		FriendsCount:     friendsCount,
		FriendshipStatus: friendshipStatus,
	}, nil
}

func (uc *UserUsecase) GetCompareResult(ctx context.Context, userID uuid.UUID, userDanceID string) (*models.CompareResult, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	ownerID, ownerErr := uc.userRepo.GetAttemptOwner(ctx, userDanceID)
	if ownerErr != nil {
		logger.Warn("failed to lookup attempt owner", "error", ownerErr, "attempt_id", userDanceID)
	}

	candidates := make([]string, 0, 3)
	if ownerID != nil {
		candidates = append(candidates, fmt.Sprintf("users/%s/%s/comparison_result.json", ownerID.String(), userDanceID))
	}
	candidates = append(candidates, fmt.Sprintf("users/%s/%s/comparison_result.json", userDanceID, userDanceID))
	candidates = append(candidates, fmt.Sprintf("users/%s/%s/comparison_result.json", userID.String(), userDanceID))

	var data []byte
	var s3UserID string
	for _, key := range candidates {
		d, err := uc.storageRepo.DownloadFile(ctx, key)
		if err == nil {
			data = d
			s3UserID = strings.Split(key, "/")[1]
			break
		}
		if !isS3NotFoundError(err) {
			logger.Error("failed to download comparison_result.json", "error", err, "key", key)
			return nil, users.ErrorInternalServerError
		}
	}
	if data == nil {
		return nil, users.ErrorNotFound
	}

	var stored models.CompareStatusResult
	if err := json.Unmarshal(data, &stored); err != nil {
		logger.Error("failed to unmarshal comparison_result.json", "error", err)
		return nil, users.ErrorInternalServerError
	}

	danceID := stored.DanceID
	if danceID == "" {
		danceID = userDanceID
	}

	referenceGlbKey := fmt.Sprintf("results/%s/full_animation.glb", danceID)
	if !uc.storageRepo.FileExists(ctx, referenceGlbKey) {
		referenceGlbKey = fmt.Sprintf("results/%s/segment_0.glb", danceID)
	}

	out := &models.CompareResult{
		UserGlbKey:           stored.UserGlbS3,
		ReferenceGlbKey:      referenceGlbKey,
		Score:                stored.ComparisonScore,
		DtwDistance:          stored.DtwDistance,
		DanceID:              danceID,
		UserDanceID:          userDanceID,
		Segments:             stored.Segments,
		Tips:                 stored.Tips,
		FrameScores:          stored.FrameScores,
		UserSkeletonKey:      fmt.Sprintf("users/%s/%s/skeleton.json", s3UserID, userDanceID),
		ReferenceSkeletonKey: fmt.Sprintf("results/%s/skeleton.json", danceID),
	}

	if ownerID != nil {
		if hasVideo, _ := uc.userRepo.IsSavedAttemptWithVideo(ctx, *ownerID, userDanceID); hasVideo {
			out.UserVideoKey = fmt.Sprintf("users/%s/%s/video.mp4", s3UserID, userDanceID)
		}
	}

	if stats, err := uc.userRepo.GetDanceStats(ctx, danceID); err == nil {
		out.DanceStats = &models.CompareDanceStats{
			AttemptCount: stats.AttemptCount,
			BestScore:    stats.TopScore,
		}
	}

	if ownerID != nil && *ownerID != userID {
		if owner, oErr := uc.userRepo.GetUserByID(ctx, *ownerID); oErr == nil {
			out.Owner = &models.CompareAttemptOwner{
				UserID: owner.ID.String(),
				Login:  owner.Login,
			}
		}
	}

	return out, nil
}

func (uc *UserUsecase) GetLeaderboard(ctx context.Context, danceID string, userID *uuid.UUID) (*models.LeaderboardResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	entries, err := uc.userRepo.GetDanceLeaderboard(ctx, danceID)
	if err != nil {
		logger.Error("failed to get leaderboard", "error", err)
		return nil, err
	}

	var userEntry *models.LeaderboardEntry
	inTop := false

	if userID != nil {
		for i := range entries {
			if entries[i].UserID == *userID {
				entries[i].IsMe = true
				inTop = true
				break
			}
		}
		if !inTop {
			rank, err := uc.userRepo.GetUserDanceRank(ctx, danceID, *userID)
			if err != nil {
				logger.Warn("failed to get user rank", "error", err)
			} else if rank != nil {
				userEntry = rank
			}
		}
	}

	return &models.LeaderboardResponse{
		Top:       entries,
		UserEntry: userEntry,
	}, nil
}

func (uc *UserUsecase) SendFriendRequest(ctx context.Context, senderID, receiverID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	if senderID == receiverID {
		return users.ErrorBadRequest
	}

	if _, err := uc.userRepo.GetUserByID(ctx, receiverID); err != nil {
		return users.ErrorNotFound
	}
	friendshipID, err := uc.userRepo.CreateFriendship(ctx, senderID, receiverID)
	if err != nil {
		return err
	}
	if notifErr := uc.userRepo.CreateFriendNotification(ctx, receiverID, senderID, "friend_request", friendshipID); notifErr != nil {
		logger.Warn("failed to create friend_request notification", "error", notifErr)
	}
	return nil
}

func (uc *UserUsecase) RespondFriendRequest(ctx context.Context, userID uuid.UUID, friendshipID int64, accept bool) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	status := "declined"
	if accept {
		status = "accepted"
	}
	senderID, err := uc.userRepo.UpdateFriendshipStatus(ctx, friendshipID, userID, status)
	if err != nil {
		return err
	}
	notifType := "friend_declined"
	if accept {
		notifType = "friend_accepted"
	}
	if notifErr := uc.userRepo.CreateFriendNotification(ctx, senderID, userID, notifType, friendshipID); notifErr != nil {
		logger.Warn("failed to create friend response notification", "error", notifErr)
	}
	return nil
}

func (uc *UserUsecase) GetFriends(ctx context.Context, userID uuid.UUID) ([]models.Friend, error) {
	return uc.userRepo.GetFriends(ctx, userID)
}

func (uc *UserUsecase) RemoveFriend(ctx context.Context, userID, friendID uuid.UUID) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	if userID == friendID {
		return users.ErrorBadRequest
	}

	var friendshipID int64
	if fs, fsErr := uc.userRepo.GetFriendshipBetween(ctx, userID, friendID); fsErr == nil && fs != nil {
		friendshipID = fs.FriendshipID
	}
	if err := uc.userRepo.DeleteFriendship(ctx, userID, friendID); err != nil {
		return err
	}

	if notifErr := uc.userRepo.CreateFriendNotification(ctx, friendID, userID, "friend_removed", friendshipID); notifErr != nil {
		logger.Warn("failed to create friend_removed notification", "error", notifErr)
	}
	return nil
}

func (uc *UserUsecase) GetUploadedDances(ctx context.Context, userID uuid.UUID) ([]models.UploadedDance, error) {
	return uc.userRepo.GetUploadedDancesByUser(ctx, userID)
}

func normalizeDanceDifficulty(label string) (string, int) {
	switch label {
	case "easy":
		return "easy", 25
	case "hard":
		return "hard", 75
	default:
		return "medium", 50
	}
}

func (uc *UserUsecase) SetDanceName(ctx context.Context, userID uuid.UUID, danceID, title, difficulty string) error {
	uploaders, err := uc.userRepo.GetDanceUploaders(ctx, danceID)
	if err != nil {
		return err
	}
	found := false
	for _, uid := range uploaders {
		if uid == userID {
			found = true
			break
		}
	}
	if !found {
		return users.ErrorForbidden
	}
	if err := uc.userRepo.UpdateDanceTitle(ctx, danceID, title); err != nil {
		return err
	}
	if difficulty != "" {
		label, score := normalizeDanceDifficulty(difficulty)
		if err := uc.userRepo.UpdateDanceDifficulty(ctx, danceID, label, score); err != nil {
			return err
		}
	}
	return nil
}

func (uc *UserUsecase) DeleteDance(ctx context.Context, userID uuid.UUID, danceID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	uploaders, err := uc.userRepo.GetDanceUploaders(ctx, danceID)
	if err != nil {
		return err
	}
	if !userInUploaders(uploaders, &userID) {
		return users.ErrorForbidden
	}

	if dbErr := uc.userRepo.DeleteDanceAndRelated(ctx, danceID); dbErr != nil {
		return dbErr
	}

	if err := uc.storageRepo.DeleteByPrefix(ctx, fmt.Sprintf("results/%s/", danceID)); err != nil {
		logger.Warn("failed to delete S3 results prefix", "dance_id", danceID, "error", err)
	}
	if err := uc.storageRepo.DeleteFile(ctx, fmt.Sprintf("dance-landmarks-cache/%s.json", danceID)); err != nil {
		logger.Warn("failed to delete landmarks cache", "dance_id", danceID, "error", err)
	}

	return nil
}

func (uc *UserUsecase) GetDanceModerationStatus(ctx context.Context, danceID string) (string, string, error) {
	status, err := uc.userRepo.GetDanceStatus(ctx, danceID)
	if err != nil {
		return "", "", users.ErrorNotFound
	}
	reason, _ := uc.userRepo.GetDanceModerationReason(ctx, danceID)
	return status, reason, nil
}

func (uc *UserUsecase) PublishDance(ctx context.Context, userID uuid.UUID, danceID string) error {
	uploaders, err := uc.userRepo.GetDanceUploaders(ctx, danceID)
	if err != nil {
		return err
	}
	found := false
	for _, uid := range uploaders {
		if uid == userID {
			found = true
			break
		}
	}
	if !found {
		return users.ErrorForbidden
	}
	return uc.userRepo.UpdateDanceStatus(ctx, danceID, "published")
}

func (uc *UserUsecase) UnpublishDance(ctx context.Context, userID uuid.UUID, danceID string) error {
	uploaders, err := uc.userRepo.GetDanceUploaders(ctx, danceID)
	if err != nil {
		return err
	}
	found := false
	for _, uid := range uploaders {
		if uid == userID {
			found = true
			break
		}
	}
	if !found {
		return users.ErrorForbidden
	}
	return uc.userRepo.UpdateDanceStatus(ctx, danceID, "private")
}
