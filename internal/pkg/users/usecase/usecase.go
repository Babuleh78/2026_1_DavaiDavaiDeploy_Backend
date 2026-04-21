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
	"log/slog"
	"net/http"
	"os"
	"time"
	"strings"
	

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

func (uc *UserUsecase) AddToHistory(ctx context.Context, userID uuid.UUID, danceID string, sourceURL string) error {
    return uc.userRepo.AddToHistory(ctx, userID, danceID, sourceURL)
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




func (uc *UserUsecase) UploadDance(
    ctx context.Context, 
    buffer []byte, 
    fileFormat string,
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

    taskID, err := uc.enqueueProcessing(ctx, dancePath, danceID)
    if err != nil {
        logger.Error("failed to enqueue processing", "error", err)
        return nil, users.ErrorInternalServerError
    }

    result, err := uc.waitForProcessing(ctx, taskID, logger)
    if err != nil {
        logger.Error("processing failed", "error", err)
        return nil, users.ErrorInternalServerError
    }

    if result == nil {
        logger.Error("processing returned nil result")
        return nil, users.ErrorInternalServerError
    }

    return &models.UploadDanceResult{
        DanceID:             result.DanceID,
		FullGlbKey:          result.FullGlbKey,
        SegmentsKey:         result.SegmentsKey,
        GlbKeys:             result.GlbKeys,
        NumFrames:           result.NumFrames,
        NumSegments:         result.NumSegments,
        NumSegmentsRendered: result.NumSegmentsRendered,
        DurationSec:         result.DurationSec,
		VideoPath: 		 	 result.VideoPath,
    }, nil
}


func (uc *UserUsecase) enqueueProcessing(ctx context.Context, videoKey, danceID string) (string, error) {
    processingURL := os.Getenv("ML_SERVICE_URL") + "/process"
    requestBody := map[string]string{
        "video_key": videoKey, 
        "dance_id":  danceID,
    }

    jsonBody, _ := json.Marshal(requestBody)
	client := &http.Client{Timeout: 20 * time.Second}

    resp, err := client.Post(processingURL, "application/json", bytes.NewBuffer(jsonBody))
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
        time.Sleep(5 * time.Second)

        resp, err := client.Get(statusURL)
        if err != nil {
            logger.Warn("status check failed", "error", err)
            continue
        }

        var status struct {
            Status string           `json:"status"`
            Result *models.ProcessingResult `json:"result,omitempty"`
        }
        json.NewDecoder(resp.Body).Decode(&status)
        resp.Body.Close()

        logger.Info("processing status", "status", status.Status)

        switch status.Status {
        case "done":
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
) (*models.UploadDanceResult, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

    danceID := uuid.NewV4().String()
    

    taskID, err := uc.enqueueProcessingByURL(ctx, videoURL, danceID)
    if err != nil {
        logger.Error("failed to enqueue processing", "error", err)
        return nil, users.ErrorInternalServerError
    }

    result, err := uc.waitForProcessing(ctx, taskID, logger)
    if err != nil {
        logger.Error("processing failed", "error", err)
        return nil, users.ErrorInternalServerError
    }

    if result == nil {
        logger.Error("processing returned nil result")
        return nil, users.ErrorInternalServerError
    }

    return &models.UploadDanceResult{
        DanceID:             result.DanceID,
        FullGlbKey:          result.FullGlbKey,
		VideoPath:			 result.VideoPath,
        SegmentsKey:         result.SegmentsKey,
        GlbKeys:             result.GlbKeys,
        NumFrames:           result.NumFrames,
        NumSegments:         result.NumSegments,
        NumSegmentsRendered: result.NumSegmentsRendered,
        DurationSec:         result.DurationSec,
    }, nil
}

func (uc *UserUsecase) enqueueProcessingByURL(ctx context.Context, videoURL, danceID string) (string, error) {
    processingURL := os.Getenv("ML_SERVICE_URL") + "/process-url"
    requestBody := map[string]string{
        "url":      videoURL,
		"dance_id": danceID,
    }

    jsonBody, _ := json.Marshal(requestBody)
    client := &http.Client{Timeout: 20 * time.Second}

    resp, err := client.Post(processingURL, "application/json", bytes.NewBuffer(jsonBody))
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

func (uc *UserUsecase) GetDanceByID(ctx context.Context, danceID string, userID *uuid.UUID) (*models.UploadDanceResult, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

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

    if userID != nil {
        liked, err := uc.userRepo.IsLikedByUser(ctx, *userID, danceID)
        if err == nil {
            result.IsLiked = liked
        }
    }

    return result, nil
}
func (uc *UserUsecase) GetMainPage(ctx context.Context) ([]models.VideoItem, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

    // Сначала берём залайканные танцы
    topDances, err := uc.userRepo.GetTopLikedDances(ctx, 9)
    if err != nil {
        logger.Error("failed to get top dances", "error", err)
        return nil, users.ErrorInternalServerError
    }

    s3Address := strings.TrimRight(os.Getenv("S3_ADDRESS"), "/")
    var videos []models.VideoItem

    for _, d := range topDances {
        videos = append(videos, models.VideoItem{
            ID:  d.DanceID,
            URL: fmt.Sprintf("%s/results/%s/video.mp4", s3Address, d.DanceID),
        })
    }

    if len(videos) < 9 {
        danceIDs, err := uc.storageRepo.ListDances(ctx, 9)
        if err != nil {
            logger.Error("failed to list dances", "error", err)
            return nil, users.ErrorInternalServerError
        }
        existing := make(map[string]bool)
        for _, v := range videos {
            existing[v.ID] = true
        }
        for _, id := range danceIDs {
            if !existing[id] && len(videos) < 9 {
                videos = append(videos, models.VideoItem{
                    ID:  id,
                    URL: fmt.Sprintf("%s/results/%s/video.mp4", s3Address, id),
                })
            }
        }
    }

    return videos, nil
}

func (uc *UserUsecase) GetSegmentDescription(ctx context.Context, danceID string, segmentIdx int) (*models.SegmentDescriptionResult, error) {
    logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

    mlURL := fmt.Sprintf("%s/segment_description/%s/%d", os.Getenv("ML_SERVICE_URL"), danceID, segmentIdx)
    client := &http.Client{Timeout: 1000 * time.Second}

    resp, err := client.Get(mlURL)
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

    resp, err := client.Post(compareURL, "application/json", bytes.NewBuffer(jsonBody))
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
        time.Sleep(5 * time.Second)

        resp, err := client.Get(statusURL)
        if err != nil {
            logger.Warn("status check failed", "error", err)
            continue
        }

        var status struct {
            Status string                      `json:"status"`
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