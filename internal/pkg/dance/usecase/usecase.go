package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/dance"
	"DDDance/internal/pkg/kafka"
	"DDDance/internal/pkg/utils/log"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	uuid "github.com/satori/go.uuid"
)

type DanceUsecase struct {
	danceRepo        dance.DanceRepo
	storageRepo      dance.DanceStorageRepo
	viewCache        dance.ViewCache
	kafkaProducer    dance.KafkaPublisher
	achTrigger       dance.AchievementTrigger
	cacheInvalidator dance.CacheInvalidator
	mlLock           dance.MLLock
}

func mlServiceURL(path string) string {
	return strings.TrimRight(os.Getenv("ML_SERVICE_URL"), "/") + "/ml/" + strings.TrimLeft(path, "/")
}

func NewDanceUsecase(danceRepo dance.DanceRepo, storageRepo dance.DanceStorageRepo) *DanceUsecase {
	return &DanceUsecase{
		danceRepo:   danceRepo,
		storageRepo: storageRepo,
	}
}

func (uc *DanceUsecase) SetViewCache(vc dance.ViewCache) {
	uc.viewCache = vc
}

func (uc *DanceUsecase) SetKafkaProducer(kp dance.KafkaPublisher) {
	uc.kafkaProducer = kp
}

func (uc *DanceUsecase) SetAchievementTrigger(at dance.AchievementTrigger) {
	uc.achTrigger = at
}

func (uc *DanceUsecase) SetCacheInvalidator(ci dance.CacheInvalidator) {
	uc.cacheInvalidator = ci
}

func (uc *DanceUsecase) SetMLLock(l dance.MLLock) {
	uc.mlLock = l
}

func (uc *DanceUsecase) triggerAchievementCheck(ctx context.Context, userID uuid.UUID) {
	if uc.achTrigger == nil {
		return
	}
	detached := context.WithoutCancel(ctx)
	go func() {
		if _, err := uc.achTrigger.CheckAndUnlockAchievements(detached, userID); err != nil {
			log.GetLoggerFromContext(detached).Warn("async achievement check failed", "user_id", userID, "error", err)
		}
	}()
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

func isS3NotFoundError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
	return strings.Contains(errMsg, "NoSuchKey") || strings.Contains(errMsg, "StatusCode: 404")
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

func (uc *DanceUsecase) callModerate(ctx context.Context, videoS3Key, danceID, uploaderUserID, uploaderLogin string) (string, error) {
	moderateURL := mlServiceURL("moderate")
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
		return result.Reason, dance.ErrorModerationPending
	}
	return "", nil
}

func (uc *DanceUsecase) linkUploaderIfPresent(ctx context.Context, danceID, uploaderUserID string, logger *slog.Logger) {
	if uploaderUserID == "" {
		return
	}
	userUUID, err := uuid.FromString(uploaderUserID)
	if err != nil {
		logger.Warn("invalid uploader user id, skipping upload link", "user_id", uploaderUserID, "error", err)
		return
	}
	if err := uc.danceRepo.LinkDanceUpload(ctx, userUUID, danceID); err != nil {
		logger.Warn("failed to link dance upload", "dance_id", danceID, "user_id", uploaderUserID, "error", err)
	}
}

func (uc *DanceUsecase) enqueueProcessing(ctx context.Context, videoKey, danceID, uploaderUserID string) (string, error) {
	processingURL := mlServiceURL("process")
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

func (uc *DanceUsecase) enqueueProcessingByURL(ctx context.Context, videoURL, danceID, uploaderUserID string) (string, error) {
	processingURL := mlServiceURL("process-url/")
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

func (uc *DanceUsecase) waitForProcessing(ctx context.Context, taskID string, logger *slog.Logger) (*models.ProcessingResult, error) {
	statusURL := mlServiceURL("status/") + taskID

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
				return status.Result, dance.ErrorModerationPending
			}
			return status.Result, nil
		case "failed":
			return nil, fmt.Errorf("processing failed")
		}
	}

	return nil, fmt.Errorf("processing timeout")
}

func (uc *DanceUsecase) notifyDanceUploaders(ctx context.Context, danceID, notifType, reason string, logger *slog.Logger) {
	uploaders, err := uc.danceRepo.GetDanceUploaders(ctx, danceID)
	if err != nil {
		logger.Warn("failed to get dance uploaders for notification", "dance_id", danceID, "error", err)
		return
	}
	for _, uid := range uploaders {
		if err := uc.danceRepo.CreateNotification(ctx, uid, notifType, danceID, reason); err != nil {
			logger.Warn("failed to create notification", "dance_id", danceID, "user_id", uid.String(), "error", err)
		}
	}
}

func (uc *DanceUsecase) UploadDance(
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
		return nil, dance.ErrorBadRequest
	}

	dancePath, err := uc.storageRepo.UploadDance(ctx, buffer, fileFormat, danceExtension)
	if err != nil {
		logger.Error("failed to upload dance", "error", err)
		return nil, dance.ErrorInternalServerError
	}

	danceID := uuid.NewV4().String()
	logger.Info("video uploaded to S3", "path", dancePath, "dance_id", danceID)

	if reason, err := uc.callModerate(ctx, dancePath, danceID, uploaderUserID, uploaderLogin); err != nil {
		if err == dance.ErrorModerationPending {
			if dbErr := uc.danceRepo.CreateDance(ctx, danceID, "", "pending", "medium", dancePath); dbErr != nil {
				logger.Error("failed to create pending dance record", "error", dbErr)
			}
			if reason != "" {
				if rErr := uc.danceRepo.SetDanceModerationReason(ctx, danceID, reason); rErr != nil {
					logger.Warn("failed to save moderation reason", "dance_id", danceID, "error", rErr)
				}
			}
			uc.linkUploaderIfPresent(ctx, danceID, uploaderUserID, logger)
			return &models.UploadDanceResult{
				DanceID:          danceID,
				ModerationReason: reason,
			}, dance.ErrorModerationPending
		}
		logger.Warn("pre-moderation check failed, proceeding", "error", err)
	}

	lockKey := "ml:processing:" + danceID
	if uc.mlLock != nil {
		acquired, _ := uc.mlLock.TryLock(ctx, lockKey, danceID, 600*time.Second)
		if !acquired {
			existingTaskID, _ := uc.mlLock.GetValue(ctx, lockKey)
			logger.Info("ml processing lock already held, returning existing task", "dance_id", danceID)
			return &models.UploadDanceResult{DanceID: danceID, TaskID: existingTaskID}, nil
		}
	}

	taskID, err := uc.enqueueProcessing(ctx, dancePath, danceID, uploaderUserID)
	if err != nil {
		if uc.mlLock != nil {
			_ = uc.mlLock.Unlock(ctx, lockKey)
		}
		logger.Error("failed to enqueue processing", "error", err)
		return nil, dance.ErrorInternalServerError
	}

	if dbErr := uc.danceRepo.CreateDance(ctx, danceID, "", "processing", "medium", dancePath); dbErr != nil {
		logger.Error("failed to create processing dance record", "error", dbErr)
	}
	uc.linkUploaderIfPresent(ctx, danceID, uploaderUserID, logger)

	return &models.UploadDanceResult{
		DanceID: danceID,
		TaskID:  taskID,
	}, nil
}

func (uc *DanceUsecase) FinalizeUploadTask(ctx context.Context, danceID string, result *models.ProcessingResult, uploaderUserID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	status, err := uc.danceRepo.GetDanceStatus(ctx, danceID)
	if err != nil {
		return err
	}
	if status == "private" || status == "published" {
		return nil
	}
	if result.Status == "moderation_pending" {
		if rErr := uc.danceRepo.UpdateDanceStatus(ctx, danceID, "pending"); rErr != nil {
			logger.Warn("failed to set pending status", "dance_id", danceID, "error", rErr)
		}
		if result.Reason != "" {
			if rErr := uc.danceRepo.SetDanceModerationReason(ctx, danceID, result.Reason); rErr != nil {
				logger.Warn("failed to save moderation reason", "dance_id", danceID, "error", rErr)
			}
		}
		return dance.ErrorModerationPending
	}
	if dbErr := uc.danceRepo.UpdateDanceStatus(ctx, danceID, "private"); dbErr != nil {
		logger.Error("failed to set dance private", "dance_id", danceID, "error", dbErr)
		return dbErr
	}
	if uc.cacheInvalidator != nil {
		uc.cacheInvalidator.InvalidateCache()
		logger.Info("recommendation cache invalidated after dance upload", "dance_id", danceID)
	}
	if result.DurationSec > 0 {
		if durErr := uc.danceRepo.UpdateDanceDuration(ctx, danceID, int(result.DurationSec)); durErr != nil {
			logger.Warn("failed to save duration_sec", "dance_id", danceID, "error", durErr)
		}
	}
	if uploaderUserID != "" {
		uc.linkUploaderIfPresent(ctx, danceID, uploaderUserID, logger)
		if uc.kafkaProducer != nil {
			payload, _ := json.Marshal(map[string]string{
				"user_id":  uploaderUserID,
				"dance_id": danceID,
			})
			uc.kafkaProducer.PublishAsync(ctx, kafka.TopicDanceUploaded, uploaderUserID, payload, func(err error) {
				logger.Warn("kafka publish TopicDanceUploaded failed", "dance_id", danceID, "error", err)
			})
		} else {
			if uid, parseErr := uuid.FromString(uploaderUserID); parseErr == nil {
				uc.triggerAchievementCheck(ctx, uid)
			}
		}
	}
	return nil
}

func (uc *DanceUsecase) UploadDanceByURL(
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
		return nil, dance.ErrorInternalServerError
	}

	if dbErr := uc.danceRepo.CreateDance(ctx, danceID, "", "processing", "medium", ""); dbErr != nil {
		logger.Error("failed to create processing dance record", "error", dbErr)
	}
	uc.linkUploaderIfPresent(ctx, danceID, uploaderUserID, logger)

	return &models.UploadDanceResult{
		DanceID: danceID,
		TaskID:  taskID,
	}, nil
}

func (uc *DanceUsecase) GetDanceByID(ctx context.Context, danceID string, userID *uuid.UUID) (*models.UploadDanceResult, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if status, statusErr := uc.danceRepo.GetDanceStatus(ctx, danceID); statusErr != nil || status != "published" {
		uploaders, uploadersErr := uc.danceRepo.GetDanceUploaders(ctx, danceID)
		if uploadersErr != nil {
			logger.Error("failed to get dance uploaders for access check", "error", uploadersErr)
			return nil, dance.ErrorInternalServerError
		}
		if len(uploaders) > 0 && !userInUploaders(uploaders, userID) {
			logger.Warn("access to private dance denied", "dance_id", danceID)
			return nil, dance.ErrorNotFound
		}
	}

	segmentsKey := fmt.Sprintf("results/%s/segments.json", danceID)

	data, err := uc.storageRepo.DownloadFile(ctx, segmentsKey)
	if err != nil {
		logger.Error("failed to get segments.json", "error", err)
		return nil, dance.ErrorNotFound
	}

	var segmentsDoc struct {
		DanceID     string `json:"dance_id"`
		NumSegments int    `json:"num_segments"`
		Meta        struct {
			NumFrames   int     `json:"num_frames"`
			DurationSec float64 `json:"duration_sec"`
			FPS         float64 `json:"fps"`
		} `json:"meta"`
		Segments []struct {
			Index          int    `json:"index"`
			StartFrame     int    `json:"start_frame"`
			EndFrame       int    `json:"end_frame"`
			LlmDescription string `json:"llm_description"`
		} `json:"segments"`
	}
	if err := json.Unmarshal(data, &segmentsDoc); err != nil {
		logger.Error("failed to parse segments.json", "error", err)
		return nil, dance.ErrorInternalServerError
	}

	fps := segmentsDoc.Meta.FPS
	if fps <= 0 {
		fps = 30
	}

	segInfos := make([]models.SegmentInfo, len(segmentsDoc.Segments))
	for i, s := range segmentsDoc.Segments {
		endFrame := s.EndFrame - 5
		if endFrame <= s.StartFrame {
			endFrame = s.EndFrame
		}
		segInfos[i] = models.SegmentInfo{
			Index:       s.Index,
			StartTime:   float64(s.StartFrame) / fps,
			EndTime:     float64(endFrame) / fps,
			Description: s.LlmDescription,
		}
	}

	glbKeys := make([]string, segmentsDoc.NumSegments)
	for i := 0; i < segmentsDoc.NumSegments; i++ {
		glbKeys[i] = fmt.Sprintf("results/%s/segment_%d.glb", danceID, i)
	}

	result := &models.UploadDanceResult{
		DanceID:             danceID,
		SegmentsKey:         segmentsKey,
		FullGlbKey:          fmt.Sprintf("results/%s/full_animation.glb", danceID),
		GlbKeys:             glbKeys,
		KeyframesURL:        fmt.Sprintf("results/%s/keyframes.json", danceID),
		Segments:            segInfos,
		NumFrames:           segmentsDoc.Meta.NumFrames,
		NumSegments:         segmentsDoc.NumSegments,
		NumSegmentsRendered: segmentsDoc.NumSegments,
		DurationSec:         segmentsDoc.Meta.DurationSec,
		VideoPath:           fmt.Sprintf("results/%s/video.mp4", danceID),
	}

	count, err := uc.danceRepo.GetLikesCount(ctx, danceID)
	if err == nil {
		result.LikesCount = count
	}

	if uc.viewCache != nil {
		if n, hllErr := uc.viewCache.PFCount(ctx, "hll:views:"+danceID); hllErr == nil {
			result.UniqueViewersApprox = n
		}
	}

	if author, authErr := uc.danceRepo.GetDanceAuthor(ctx, danceID); authErr != nil {
		logger.Warn("failed to get dance author", "dance_id", danceID, "error", authErr)
	} else {
		result.Author = author
	}

	if diff, byUsers, dErr := uc.danceRepo.GetEffectiveDanceDifficulty(ctx, danceID); dErr != nil {
		logger.Warn("failed to get effective difficulty", "dance_id", danceID, "error", dErr)
	} else {
		result.Difficulty = diff
		result.DifficultyByUsers = byUsers
	}

	if userID != nil {
		liked, err := uc.danceRepo.IsLikedByUser(ctx, *userID, danceID)
		if err == nil {
			result.IsLiked = liked
		}

		if err := uc.danceRepo.AddToHistory(ctx, *userID, danceID, ""); err != nil {
			logger.Warn("failed to add to history", "error", err)
		}
		if last, lastErr := uc.danceRepo.GetLastAttempt(ctx, *userID, danceID); lastErr == nil && last != nil {
			result.LastAttemptID = last.AttemptID
			score := last.Score
			result.LastAttemptScore = &score
		}
	}

	return result, nil
}

func (uc *DanceUsecase) GetMainPage(ctx context.Context) ([]models.VideoItem, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	topDances, err := uc.danceRepo.GetTopLikedDances(ctx, 14)
	logger.Info("got top liked dances", "count", len(topDances))
	if err != nil {
		logger.Error("failed to get top dances", "error", err)
		return nil, dance.ErrorInternalServerError
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
			return nil, dance.ErrorInternalServerError
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
	published, err := uc.danceRepo.GetPublishedDanceIDs(ctx, ids)
	if err != nil {
		logger.Error("failed to filter published dances", "error", err)
		return nil, dance.ErrorInternalServerError
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
		enriched, err := uc.danceRepo.GetDancesEnrichedInfo(ctx, ids)
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

func (uc *DanceUsecase) GetSegmentDescription(ctx context.Context, danceID string, segmentIdx int) (*models.SegmentDescriptionResult, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	mlURL := mlServiceURL(fmt.Sprintf("segment_description/%s/%d", danceID, segmentIdx))
	client := &http.Client{Timeout: 120 * time.Second}

	resp, err := mlGet(ctx, client, mlURL)
	if err != nil {
		logger.Error("failed to call ml service", "error", err)
		return nil, dance.ErrorInternalServerError
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, dance.ErrorNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, dance.ErrorInternalServerError
	}

	var result models.SegmentDescriptionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		logger.Error("failed to decode response", "error", err)
		return nil, dance.ErrorInternalServerError
	}

	return &result, nil
}

func (uc *DanceUsecase) AddToHistory(ctx context.Context, userID uuid.UUID, danceID string, sourceURL string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	err := uc.danceRepo.AddToHistory(ctx, userID, danceID, sourceURL)
	if err != nil {
		logger.Error("failed to add to history", "error", err)
		return err
	}

	if err := uc.danceRepo.CleanHistory(ctx, userID); err != nil {
		logger.Error("failed to clean history", "error", err)
	}

	return nil
}

func (uc *DanceUsecase) GetDanceCatalog(ctx context.Context, sort, search string, page, limit int) (*models.DanceCatalogResponse, error) {
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

	items, err := uc.danceRepo.GetDanceCatalog(ctx, sort, search, page, limit)
	if err != nil {
		logger.Error("failed to get dance catalog", "error", err)
		return nil, err
	}

	total, err := uc.danceRepo.GetDanceCatalogCount(ctx, sort, search)
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

func (uc *DanceUsecase) GetDanceTrending(ctx context.Context) (*models.TrendingResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	items, err := uc.danceRepo.GetDanceTrending(ctx)
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

func (uc *DanceUsecase) GetDanceStats(ctx context.Context, danceID string) (*models.DanceStats, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	stats, err := uc.danceRepo.GetDanceStats(ctx, danceID)
	if err != nil {
		logger.Error("failed to get dance stats", "error", err)
		return nil, err
	}

	return stats, nil
}

func (uc *DanceUsecase) GetDanceTimeline(ctx context.Context, danceID string, userID string) ([]byte, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	key := fmt.Sprintf("users/%s/%s/timeline.json", userID, danceID)
	data, err := uc.storageRepo.DownloadFile(ctx, key)
	if err != nil {
		if isS3NotFoundError(err) {
			logger.Warn("timeline not found", "key", key)
			return nil, dance.ErrorNotFound
		}
		logger.Error("failed to get timeline from storage", "error", err)
		return nil, dance.ErrorInternalServerError
	}

	return data, nil
}

func (uc *DanceUsecase) GetDanceKeyframes(ctx context.Context, danceID string) ([]byte, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	key := fmt.Sprintf("results/%s/keyframes.json", danceID)
	data, err := uc.storageRepo.DownloadFile(ctx, key)
	if err != nil {
		if isS3NotFoundError(err) {
			logger.Warn("keyframes not found", "key", key)
			return nil, dance.ErrorNotFound
		}
		logger.Error("failed to get keyframes from storage", "error", err)
		return nil, dance.ErrorInternalServerError
	}

	return data, nil
}

func (uc *DanceUsecase) UpdateDanceDuration(ctx context.Context, danceID string, durationSec int) error {
	return uc.danceRepo.UpdateDanceDuration(ctx, danceID, durationSec)
}

func (uc *DanceUsecase) UpdateDanceStatus(ctx context.Context, id, status string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	var dbStatus string
	switch status {
	case "approved":
		dbStatus = "published"
	case "rejected":
		dbStatus = "rejected"
	default:
		logger.Error("invalid dance status", "status", status)
		return dance.ErrorBadRequest
	}

	currentStatus, err := uc.danceRepo.GetDanceStatus(ctx, id)
	if err != nil {
		return err
	}

	if err := uc.danceRepo.UpdateDanceStatus(ctx, id, dbStatus); err != nil {
		logger.Error("failed to update dance status", "error", err)
		return err
	}

	if dbStatus == "rejected" {
		reason, _ := uc.danceRepo.GetDanceModerationReason(ctx, id)
		uc.notifyDanceUploaders(ctx, id, "dance_rejected", reason, logger)
		return nil
	}

	if currentStatus == "pending" && dbStatus == "published" {
		videoPath, err := uc.danceRepo.GetDanceVideoPath(ctx, id)
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

func (uc *DanceUsecase) RecordDanceView(ctx context.Context, danceID string, viewerID string) (int64, error) {
	if danceID == "" || viewerID == "" {
		return 0, dance.ErrorBadRequest
	}

	shouldRecord := true
	if uc.viewCache != nil {
		key := "view:" + viewerID + ":" + danceID
		ok, err := uc.viewCache.SetNX(ctx, key, time.Hour)
		if err == nil && !ok {
			shouldRecord = false
		}
		uc.viewCache.PFAdd(ctx, "hll:views:"+danceID, viewerID)
	}

	if shouldRecord {
		if uc.kafkaProducer != nil {
			payload, _ := json.Marshal(map[string]interface{}{
				"dance_id":  danceID,
				"viewer_id": viewerID,
				"timestamp": time.Now().UTC().Unix(),
			})
			uc.kafkaProducer.PublishAsync(ctx, kafka.TopicDanceViewed, danceID, payload, func(err error) {
				log.GetLoggerFromContext(ctx).Warn("kafka publish TopicDanceViewed failed", "dance_id", danceID, "error", err)
			})
		} else {
			if err := uc.danceRepo.RecordDanceView(ctx, danceID, viewerID); err != nil {
				return 0, err
			}
		}
	}

	count, err := uc.danceRepo.GetDanceViewCount(ctx, danceID)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (uc *DanceUsecase) ClaimDanceUploads(ctx context.Context, userID uuid.UUID, danceIDs []string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	for _, id := range danceIDs {
		if id == "" {
			continue
		}
		if err := uc.danceRepo.LinkDanceUpload(ctx, userID, id); err != nil {
			logger.Warn("failed to claim dance upload", "dance_id", id, "user_id", userID.String(), "error", err)
		}
	}
	return nil
}

func (uc *DanceUsecase) GetUploadedDances(ctx context.Context, userID uuid.UUID) ([]models.UploadedDance, error) {
	return uc.danceRepo.GetUploadedDancesByUser(ctx, userID)
}

func (uc *DanceUsecase) SetDanceName(ctx context.Context, userID uuid.UUID, danceID, title, difficulty string) error {
	uploaders, err := uc.danceRepo.GetDanceUploaders(ctx, danceID)
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
		return dance.ErrorForbidden
	}
	if err := uc.danceRepo.UpdateDanceTitle(ctx, danceID, title); err != nil {
		return err
	}
	if difficulty != "" {
		label, score := normalizeDanceDifficulty(difficulty)
		if err := uc.danceRepo.UpdateDanceDifficulty(ctx, danceID, label, score); err != nil {
			return err
		}
	}
	return nil
}

func (uc *DanceUsecase) GetDanceModerationStatus(ctx context.Context, danceID string) (string, string, error) {
	status, err := uc.danceRepo.GetDanceStatus(ctx, danceID)
	if err != nil {
		return "", "", dance.ErrorNotFound
	}
	reason, _ := uc.danceRepo.GetDanceModerationReason(ctx, danceID)
	return status, reason, nil
}

func (uc *DanceUsecase) PublishDance(ctx context.Context, userID uuid.UUID, danceID string) error {
	uploaders, err := uc.danceRepo.GetDanceUploaders(ctx, danceID)
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
		return dance.ErrorForbidden
	}
	return uc.danceRepo.UpdateDanceStatus(ctx, danceID, "published")
}

func (uc *DanceUsecase) UnpublishDance(ctx context.Context, userID uuid.UUID, danceID string) error {
	uploaders, err := uc.danceRepo.GetDanceUploaders(ctx, danceID)
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
		return dance.ErrorForbidden
	}
	return uc.danceRepo.UpdateDanceStatus(ctx, danceID, "private")
}

func (uc *DanceUsecase) DeleteDance(ctx context.Context, userID uuid.UUID, danceID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	uploaders, err := uc.danceRepo.GetDanceUploaders(ctx, danceID)
	if err != nil {
		return err
	}
	if !userInUploaders(uploaders, &userID) {
		return dance.ErrorForbidden
	}

	if dbErr := uc.danceRepo.DeleteDanceAndRelated(ctx, danceID); dbErr != nil {
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

const topDancesCacheTTL = 10 * time.Minute

type topDancesCache struct {
	mu        sync.Mutex
	resp      *models.TrendingResponse
	updatedAt time.Time
}

var topDancesCacheInstance topDancesCache

func (uc *DanceUsecase) GetTopDances(ctx context.Context) (*models.TrendingResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	topDancesCacheInstance.mu.Lock()
	defer topDancesCacheInstance.mu.Unlock()

	if topDancesCacheInstance.resp != nil &&
		time.Since(topDancesCacheInstance.updatedAt) < topDancesCacheTTL {
		return topDancesCacheInstance.resp, nil
	}

	items, err := uc.danceRepo.GetTopDances(ctx)
	if err != nil {
		logger.Error("failed to get top dances", "error", err)
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

	resp := &models.TrendingResponse{
		Count:  len(videos),
		Videos: videos,
	}
	topDancesCacheInstance.resp = resp
	topDancesCacheInstance.updatedAt = time.Now()
	return resp, nil
}

func (uc *DanceUsecase) GetDanceChoreographerDescriptions(ctx context.Context, danceID string) (map[int]string, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	descs, err := uc.danceRepo.GetDanceSegmentDescriptions(ctx, danceID)
	if err != nil {
		logger.Error("failed to get dance segment descriptions", "error", err)
		return nil, err
	}
	return descs, nil
}

func (uc *DanceUsecase) UpdateSegmentDescription(ctx context.Context, userID uuid.UUID, danceID string, segmentIndex int, description string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	uploaders, err := uc.danceRepo.GetDanceUploaders(ctx, danceID)
	if err != nil {
		logger.Warn("failed to get dance uploaders", "dance_id", danceID, "error", err)
		return err
	}
	if !userInUploaders(uploaders, &userID) {
		return dance.ErrorForbidden
	}

	segmentsKey := fmt.Sprintf("results/%s/segments.json", danceID)
	data, err := uc.storageRepo.DownloadFile(ctx, segmentsKey)
	if err != nil {
		logger.Warn("segments.json not found", "dance_id", danceID, "error", err)
		return dance.ErrorNotFound
	}

	var meta struct {
		NumSegments int `json:"num_segments"`
	}
	if jsonErr := json.Unmarshal(data, &meta); jsonErr != nil {
		logger.Error("failed to parse segments.json", "error", jsonErr)
		return dance.ErrorInternalServerError
	}

	if segmentIndex < 0 || (meta.NumSegments > 0 && segmentIndex >= meta.NumSegments) {
		return dance.ErrorBadRequest
	}

	if err := uc.danceRepo.UpsertSegmentDescription(ctx, danceID, segmentIndex, description); err != nil {
		logger.Error("failed to upsert segment description", "error", err)
		return err
	}
	return nil
}
