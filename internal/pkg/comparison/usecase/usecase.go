package usecase

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/comparison"
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

const topDancersCacheTTL = 10 * time.Minute

type topDancersCache struct {
	mu        sync.Mutex
	entries   []models.TopDancerEntry
	updatedAt time.Time
}

var topDancersCacheInstance topDancersCache

const mlInternalTokenHeader = "X-Internal-Token"

type ComparisonUsecase struct {
	compRepo      comparison.ComparisonRepo
	storageRepo   comparison.ComparisonStorageRepo
	achTrigger    comparison.AchievementTrigger
	duelSub       comparison.DuelSubmitter
	uploadFin     comparison.UploadFinalizer
	kafkaProducer comparison.KafkaPublisher
	top10Cache    comparison.Top10Cache
	botNotifier   comparison.BotNotifier
	leaderboard   comparison.Leaderboard
	ssePublisher  comparison.SSEPublisher
}

// NewComparisonUsecase wires the comparison usecase. The achievement trigger,
// duel submitter and upload finalizer are required collaborators and are passed
// here so a fully-constructed usecase is guaranteed to have them (no post-hoc
// setters, no nil-collaborator panics). Optional infrastructure (Redis caches,
// Kafka, SSE) is still attached via Set* and degrades gracefully when absent.
func NewComparisonUsecase(
	compRepo comparison.ComparisonRepo,
	storageRepo comparison.ComparisonStorageRepo,
	achTrigger comparison.AchievementTrigger,
	duelSub comparison.DuelSubmitter,
	uploadFin comparison.UploadFinalizer,
) *ComparisonUsecase {
	return &ComparisonUsecase{
		compRepo:    compRepo,
		storageRepo: storageRepo,
		achTrigger:  achTrigger,
		duelSub:     duelSub,
		uploadFin:   uploadFin,
	}
}

func (uc *ComparisonUsecase) SetKafkaProducer(kp comparison.KafkaPublisher) {
	uc.kafkaProducer = kp
}

func (uc *ComparisonUsecase) SetTop10Cache(c comparison.Top10Cache) {
	uc.top10Cache = c
}

func (uc *ComparisonUsecase) SetBotNotifier(bn comparison.BotNotifier) {
	uc.botNotifier = bn
}

func (uc *ComparisonUsecase) SetLeaderboard(lb comparison.Leaderboard) {
	uc.leaderboard = lb
}

func (uc *ComparisonUsecase) SetSSEPublisher(sp comparison.SSEPublisher) {
	uc.ssePublisher = sp
}

func mlServiceURL(path string) string {
	return strings.TrimRight(os.Getenv("ML_SERVICE_URL"), "/") + "/ml/" + strings.TrimLeft(path, "/")
}

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

func (uc *ComparisonUsecase) triggerAchievementCheck(ctx context.Context, userID uuid.UUID) {
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

func (uc *ComparisonUsecase) maybeNotifyTop10(ctx context.Context, userID uuid.UUID, logger *slog.Logger) {
	if uc.top10Cache == nil || uc.kafkaProducer == nil {
		return
	}
	detached := context.WithoutCancel(ctx)
	go func() {
		rank, err := uc.compRepo.GetUserGlobalRank(detached, userID)
		if err != nil || rank > 10 {
			return
		}
		cacheKey := fmt.Sprintf("top10_notified:%s", userID.String())
		fired, _ := uc.top10Cache.SetNX(detached, cacheKey, 24*time.Hour)
		if !fired {
			return
		}
		payload, _ := json.Marshal(map[string]interface{}{
			"to_user_id":       userID.String(),
			"type":             "top_entry",
			"telegram_payload": map[string]interface{}{},
		})
		uc.kafkaProducer.PublishAsync(detached, kafka.TopicNotificationSend, userID.String(), payload, func(pubErr error) {
			logger.Warn("kafka publish top_entry notification failed", "user_id", userID, "error", pubErr)
		})
	}()
}

func (uc *ComparisonUsecase) maybeNotifyAttemptResult(ctx context.Context, userID uuid.UUID, attemptID, danceID string, score float64, logger *slog.Logger) {
	if uc.botNotifier == nil {
		return
	}
	detached := context.WithoutCancel(ctx)
	go func() {
		tid, err := uc.compRepo.GetUserTelegramID(detached, userID)
		if err != nil || tid == nil {
			return
		}
		title, _ := uc.compRepo.GetDanceTitleByID(detached, danceID)
		if title == "" {
			title = danceID
		}
		msg, _ := json.Marshal(map[string]interface{}{
			"telegram_id": *tid,
			"type":        "attempt_result",
			"payload": map[string]interface{}{
				"dance_title": title,
				"score":       score,
				"attempt_id":  attemptID,
			},
		})
		if pushErr := uc.botNotifier.Push(detached, msg); pushErr != nil {
			logger.Warn("failed to push attempt_result bot notification", "user_id", userID, "error", pushErr)
		}
	}()
}

func isS3NotFoundError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "NoSuchKey") || strings.Contains(msg, "StatusCode: 404")
}

// loadAuthoritativeResult fetches the ML-produced comparison_result.json for an
// attempt from S3, returning nil when it cannot be read or parsed. It is the
// source of truth for an attempt's score (see SaveAttempt). The candidate keys
// mirror the path resolution in GetCompareResult: owner-based first, then the
// anonymous layout (attempt_id/attempt_id) used before an anon→register move.
func (uc *ComparisonUsecase) loadAuthoritativeResult(ctx context.Context, userID uuid.UUID, attemptID string) *models.CompareStatusResult {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	candidates := []string{
		fmt.Sprintf("users/%s/%s/comparison_result.json", userID.String(), attemptID),
		fmt.Sprintf("users/%s/%s/comparison_result.json", attemptID, attemptID),
	}
	for _, key := range candidates {
		data, err := uc.storageRepo.DownloadFile(ctx, key)
		if err != nil {
			if !isS3NotFoundError(err) {
				logger.Warn("failed to download comparison_result.json for score verification", "error", err, "key", key)
			}
			continue
		}
		var stored models.CompareStatusResult
		if err := json.Unmarshal(data, &stored); err != nil {
			logger.Warn("failed to unmarshal comparison_result.json for score verification", "error", err, "key", key)
			return nil
		}
		return &stored
	}
	return nil
}

// meanSegmentScores returns the mean timing / amplitude / pose-accuracy across
// segments. ok is false when there are no segments to average, so the caller
// keeps whatever values it already had.
func meanSegmentScores(segments []models.SegmentDiagnostic) (timing, amplitude, pose float64, ok bool) {
	if len(segments) == 0 {
		return 0, 0, 0, false
	}
	for _, s := range segments {
		timing += s.TimingScore
		amplitude += s.AmplitudeScore
		pose += s.PoseAccuracyScore
	}
	n := float64(len(segments))
	return timing / n, amplitude / n, pose / n, true
}

func (uc *ComparisonUsecase) SaveAttempt(ctx context.Context, userID uuid.UUID, attemptID, danceID string, includeVideo bool, fallbackScore *float64, userName string, isPrivate bool, duelID *uuid.UUID, publicConsent bool, submitToDuels bool, timingScore, amplitudeScore, poseScore float64) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	if attemptID == "" || danceID == "" {
		return comparison.ErrorBadRequest
	}

	prevRank, _ := uc.compRepo.GetUserDanceRank(ctx, danceID, userID)
	prevBest := 0.0
	if prevRank != nil {
		prevBest = prevRank.Score
	}

	// SECURITY: the score persisted to the DB and pushed to the global/per-dance
	// leaderboard MUST be the ML-produced value, never the number in the client
	// request body. The authoritative comparison_result.json (written by the ML
	// service) is read from S3 here. The request-supplied scores are accepted
	// only as a fallback when that artifact cannot be read — legacy attempts or
	// the anonymous→register handoff before the S3 objects are moved. Without
	// this, any client could POST an arbitrary score and top the leaderboard.
	var (
		score        float64
		haveNewScore bool
	)
	if result := uc.loadAuthoritativeResult(ctx, userID, attemptID); result != nil {
		score = result.ComparisonScore
		haveNewScore = true
		if t, a, p, ok := meanSegmentScores(result.Segments); ok {
			timingScore, amplitudeScore, poseScore = t, a, p
		}
		if fallbackScore != nil && *fallbackScore != score {
			logger.Warn("client-reported score differs from ML result; using ML result",
				"attempt_id", attemptID, "client_score", *fallbackScore, "ml_score", score)
		}
	} else if fallbackScore != nil {
		score = *fallbackScore
		haveNewScore = true
		logger.Warn("authoritative comparison result unavailable; using client-reported score",
			"attempt_id", attemptID)
	} else if prevRank != nil {
		score = prevRank.Score
	}

	if haveNewScore {
		if recErr := uc.compRepo.RecordDanceAttempt(ctx, danceID, &userID, attemptID, score); recErr != nil {
			logger.Warn("failed to record attempt (may already exist)", "error", recErr)
		}
	}

	alreadySaved, _ := uc.compRepo.SavedAttemptExists(ctx, userID, attemptID)

	if err := uc.compRepo.SaveAttempt(ctx, userID, attemptID, danceID, score, includeVideo, userName, isPrivate, timingScore, amplitudeScore, poseScore); err != nil {
		return err
	}

	if !includeVideo {
		videoKey := fmt.Sprintf("users/%s/%s/video.mp4", userID.String(), attemptID)
		if delErr := uc.storageRepo.DeleteFile(ctx, videoKey); delErr != nil {
			logger.Warn("failed to delete user video on save-without-video", "error", delErr, "key", videoKey)
		}
	}

	if duelID != nil && uc.duelSub != nil {
		attemptUUID, parseErr := uuid.FromString(attemptID)
		if parseErr == nil {
			if submitErr := uc.duelSub.SubmitDuelAttempt(ctx, userID, *duelID, attemptUUID, publicConsent); submitErr != nil {
				logger.Warn("failed to auto-submit duel attempt", "error", submitErr, "duel_id", duelID.String())
			}
		}
	}

	if submitToDuels && uc.duelSub != nil {
		if attemptUUID, parseErr := uuid.FromString(attemptID); parseErr == nil {
			if subErr := uc.duelSub.SubmitAttemptToActiveDuels(ctx, userID, danceID, attemptUUID, publicConsent); subErr != nil {
				logger.Warn("failed to fan-out duel attempt", "error", subErr, "dance_id", danceID)
			}
		}
	}

	if uc.kafkaProducer != nil {
		payload, _ := json.Marshal(map[string]interface{}{
			"user_id":    userID.String(),
			"attempt_id": attemptID,
			"dance_id":   danceID,
			"score":      score,
			"is_private": isPrivate,
		})
		logger.Info("publishing TopicAttemptSaved to Kafka")
		uc.kafkaProducer.PublishAsync(ctx, kafka.TopicAttemptSaved, userID.String(), payload, func(err error) {
			log.GetLoggerFromContext(ctx).Warn("kafka publish TopicAttemptSaved failed", "user_id", userID, "error", err)
		})
	} else {
		uc.triggerAchievementCheck(ctx, userID)
	}

	if uc.kafkaProducer != nil && score > 0 && score > prevBest {
		danceTitle, _ := uc.compRepo.GetDanceTitleByID(ctx, danceID)
		if danceTitle == "" {
			danceTitle = danceID
		}
		improvedPayload, _ := json.Marshal(map[string]interface{}{
			"user_id":     userID.String(),
			"dance_id":    danceID,
			"dance_title": danceTitle,
			"new_score":   score,
			"delta":       score - prevBest,
		})
		uc.kafkaProducer.PublishAsync(ctx, kafka.TopicAttemptImproved, userID.String(), improvedPayload, func(err error) {
			log.GetLoggerFromContext(ctx).Warn("kafka publish TopicAttemptImproved failed", "user_id", userID, "error", err)
		})
	}

	if uc.kafkaProducer != nil {
		warmupPayload, _ := json.Marshal(map[string]interface{}{"user_id": userID.String()})
		uc.kafkaProducer.PublishAsync(ctx, kafka.TopicRecommendationWarmup, userID.String(), warmupPayload, func(err error) {
			log.GetLoggerFromContext(ctx).Warn("kafka publish TopicRecommendationWarmup failed", "user_id", userID, "error", err)
		})
	}

	uc.maybeNotifyTop10(ctx, userID, logger)
	if !alreadySaved {
		uc.maybeNotifyAttemptResult(ctx, userID, attemptID, danceID, score, logger)
	}
	if uc.achTrigger != nil {
		uc.achTrigger.TriggerNightDancerAchievement(ctx, userID)
		uc.achTrigger.TriggerSpeedLearnerAchievement(ctx, userID, danceID)
		uc.achTrigger.TriggerTopDancerAchievement(ctx, userID)
	}
	if uc.leaderboard != nil && score > 0 {
		detached := context.WithoutCancel(ctx)
		uid := userID.String()
		did := danceID
		sc := score
		lb := uc.leaderboard
		go func() {
			if err := lb.ZAdd(detached, "global", sc, uid); err != nil {
				log.GetLoggerFromContext(detached).Warn("leaderboard ZAdd global failed", "error", err)
			}
			if err := lb.ZAdd(detached, "dance:"+did, sc, uid); err != nil {
				log.GetLoggerFromContext(detached).Warn("leaderboard ZAdd dance failed", "error", err, "dance_id", did)
			}
		}()
	}
	if !alreadySaved && uc.ssePublisher != nil && score > 0 {
		detached := context.WithoutCancel(ctx)
		uid := userID.String()
		aid := attemptID
		did := danceID
		sc := score
		pub := uc.ssePublisher
		go func() {
			title, _ := uc.compRepo.GetDanceTitleByID(detached, did)
			if title == "" {
				title = did
			}
			evt, _ := json.Marshal(map[string]interface{}{
				"type":        "attempt_result",
				"attempt_id":  aid,
				"dance_title": title,
				"score":       sc,
			})
			if err := pub.Publish(detached, uid, evt); err != nil {
				log.GetLoggerFromContext(detached).Warn("sse publish attempt_result failed", "user_id", uid, "error", err)
			}
		}()
	}
	return nil
}

func (uc *ComparisonUsecase) UnsaveAttempt(ctx context.Context, userID uuid.UUID, attemptID string) error {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))
	if attemptID == "" {
		return comparison.ErrorBadRequest
	}

	hadVideo, err := uc.compRepo.IsSavedAttemptWithVideo(ctx, userID, attemptID)
	if err != nil {
		logger.Warn("failed to read has_video before unsave", "error", err)
		hadVideo = false
	}

	if err := uc.compRepo.UnsaveAttempt(ctx, userID, attemptID); err != nil {
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

func (uc *ComparisonUsecase) GetSavedAttempts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.SavedAttemptItem, error) {
	return uc.compRepo.GetSavedAttempts(ctx, userID, limit, offset)
}

func (uc *ComparisonUsecase) GetUserAttempts(ctx context.Context, userID uuid.UUID, limit, offset int) ([]models.UserAttemptItem, error) {
	return uc.compRepo.GetUserAttempts(ctx, userID, limit, offset)
}

func (uc *ComparisonUsecase) GetCompareResult(ctx context.Context, userID uuid.UUID, userDanceID string) (*models.CompareResult, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	ownerID, ownerErr := uc.compRepo.GetAttemptOwner(ctx, userDanceID)
	if ownerErr != nil {
		logger.Warn("failed to lookup attempt owner", "error", ownerErr, "attempt_id", userDanceID)
	}

	if ownerID != nil && *ownerID != userID {
		if isPrivate, _ := uc.compRepo.IsAttemptPrivate(ctx, userDanceID); isPrivate {
			return nil, comparison.ErrorForbidden
		}
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
			return nil, comparison.ErrorInternalServerError
		}
	}
	if data == nil {
		return nil, comparison.ErrorNotFound
	}

	var stored models.CompareStatusResult
	if err := json.Unmarshal(data, &stored); err != nil {
		logger.Error("failed to unmarshal comparison_result.json", "error", err)
		return nil, comparison.ErrorInternalServerError
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
		FrameLabels:          stored.FrameLabels,
		UserSkeletonKey:      fmt.Sprintf("users/%s/%s/skeleton.json", s3UserID, userDanceID),
		ReferenceSkeletonKey: fmt.Sprintf("results/%s/skeleton.json", danceID),
	}

	userVideoKey := fmt.Sprintf("users/%s/%s/video.mp4", s3UserID, userDanceID)
	if uc.storageRepo.FileExists(ctx, userVideoKey) {
		out.UserVideoKey = userVideoKey
	}

	if stats, err := uc.compRepo.GetDanceStats(ctx, danceID); err == nil {
		out.DanceStats = &models.CompareDanceStats{
			AttemptCount: stats.AttemptCount,
			BestScore:    stats.TopScore,
		}
	}

	if ownerID != nil && *ownerID != userID {
		if owner, oErr := uc.compRepo.GetUserByID(ctx, *ownerID); oErr == nil {
			out.Owner = &models.CompareAttemptOwner{
				UserID: owner.ID.String(),
				Login:  owner.Login,
			}
		}
	}

	return out, nil
}

func (uc *ComparisonUsecase) GetLeaderboard(ctx context.Context, danceID string, userID *uuid.UUID) (*models.LeaderboardResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	entries, err := uc.compRepo.GetDanceLeaderboard(ctx, danceID)
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
			rank, err := uc.compRepo.GetUserDanceRank(ctx, danceID, *userID)
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

func (uc *ComparisonUsecase) GetTopDancers(ctx context.Context) ([]models.TopDancerEntry, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if uc.leaderboard != nil {
		userIDs, err := uc.leaderboard.ZTopMembers(ctx, "global", 50)
		if err == nil && len(userIDs) > 0 {
			entries, dbErr := uc.compRepo.GetTopDancersByIDs(ctx, userIDs)
			if dbErr == nil {
				return entries, nil
			}
			logger.Warn("GetTopDancersByIDs failed, falling back to DB scan", "error", dbErr)
		}
	}

	topDancersCacheInstance.mu.Lock()
	defer topDancersCacheInstance.mu.Unlock()

	if topDancersCacheInstance.entries != nil &&
		time.Since(topDancersCacheInstance.updatedAt) < topDancersCacheTTL {
		return topDancersCacheInstance.entries, nil
	}

	entries, err := uc.compRepo.GetTopDancers(ctx)
	if err != nil {
		logger.Error("failed to get top dancers from db", "error", err)
		return nil, err
	}

	topDancersCacheInstance.entries = entries
	topDancersCacheInstance.updatedAt = time.Now()
	return entries, nil
}

func (uc *ComparisonUsecase) CompareDanceFromBuffer(ctx context.Context, buffer []byte, fileFormat string, referenceDanceID string, userID *uuid.UUID) (*models.CompareResult, error) {
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
		return nil, comparison.ErrorInternalServerError
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.Write(buffer); err != nil {
		return nil, comparison.ErrorInternalServerError
	}
	tmpFile.Close()

	if err := uc.storageRepo.UploadFileRaw(ctx, tmpFile.Name(), videoKey); err != nil {
		logger.Error("failed to upload user video", "error", err)
		return nil, comparison.ErrorInternalServerError
	}

	mlURL := strings.TrimRight(os.Getenv("ML_SERVICE_URL"), "/") + "/ml/dance_compare"
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
		return nil, comparison.ErrorInternalServerError
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
		return nil, comparison.ErrorInternalServerError
	}

	if taskResp.TaskID != "" {
		if err := uc.compRepo.CreateCompareTask(ctx, taskResp.TaskID, referenceDanceID, userDanceID, videoKey); err != nil {
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

func (uc *ComparisonUsecase) finalizeCompareTask(ctx context.Context, mlResult *models.CompareStatusResult, referenceDanceID, userDanceID, videoKey string, userID *uuid.UUID, recordAttempt bool) (*models.CompareResult, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	if recordAttempt && userID != nil {
		if recErr := uc.compRepo.RecordDanceAttempt(ctx, referenceDanceID, userID, userDanceID, mlResult.ComparisonScore); recErr != nil {
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

	if stats, err := uc.compRepo.GetDanceStats(ctx, referenceDanceID); err == nil {
		out.DanceStats = &models.CompareDanceStats{
			AttemptCount: stats.AttemptCount,
			BestScore:    stats.TopScore,
		}
	}

	return out, nil
}

func (uc *ComparisonUsecase) waitForCompareResult(ctx context.Context, taskID string, logger *slog.Logger) (*models.CompareStatusResult, error) {
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
				return nil, comparison.ErrorInternalServerError
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

func (uc *ComparisonUsecase) generateCompareTips(ctx context.Context, score float64, segments []models.SegmentDiagnostic) ([]models.CompareTip, error) {
	mlURL := mlServiceURL("compare_tips")

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

func (uc *ComparisonUsecase) GetTaskStatus(ctx context.Context, taskID, taskType, danceID, userDanceID, videoKey, uploaderUserID string, userID *uuid.UUID) (*models.TaskStatusResponse, error) {
	logger := log.GetLoggerFromContext(ctx).With(slog.String("func", log.GetFuncName()))

	mlURL := mlServiceURL("status/") + taskID
	client := &http.Client{Timeout: 10 * time.Second}

	resp, err := mlGet(ctx, client, mlURL)
	if err != nil {
		logger.Error("failed to get task status from ML", "error", err)
		return nil, comparison.ErrorInternalServerError
	}
	defer resp.Body.Close()

	var mlStatus models.MlStatusResp
	if err := json.NewDecoder(resp.Body).Decode(&mlStatus); err != nil {
		return nil, comparison.ErrorInternalServerError
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
			return nil, comparison.ErrorInternalServerError
		}

		var finErr error
		if uc.uploadFin != nil {
			finErr = uc.uploadFin.FinalizeUploadTask(ctx, danceID, &mlResult, uploaderUserID)
		}
		if finErr != nil && finErr.Error() != "moderation_pending" {
			logger.Warn("FinalizeUploadTask error", "error", finErr)
		}

		if mlResult.Status == "moderation_pending" {
			out.Status = "failed"
			out.ModerationFailed = true
			out.ModerationReason = mlResult.Reason
			out.Error = "moderation_failed"
			return out, nil
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
			return nil, comparison.ErrorInternalServerError
		}

		finDanceID, finUserDanceID, finVideoKey := danceID, userDanceID, videoKey
		recordAttempt := false
		if task, taskErr := uc.compRepo.GetCompareTask(ctx, taskID); taskErr == nil {
			finDanceID = task.DanceID
			finUserDanceID = task.UserDanceID
			finVideoKey = task.VideoKey
			claimed, claimErr := uc.compRepo.MarkCompareTaskFinalized(ctx, taskID)
			if claimErr != nil {
				logger.Warn("failed to claim compare task finalization", "error", claimErr)
			}
			recordAttempt = claimed
		} else {
			logger.Warn("compare task metadata not found, finalizing without recording attempt", "task_id", taskID)
		}

		compareResult, finErr := uc.finalizeCompareTask(ctx, &mlResult, finDanceID, finUserDanceID, finVideoKey, userID, recordAttempt)
		if finErr != nil {
			logger.Warn("finalizeCompareTask error", "error", finErr)
		}
		if compareResult != nil {
			resultBytes, _ := json.Marshal(compareResult)
			out.Result = resultBytes
		}
	}

	return out, nil
}

func (uc *ComparisonUsecase) CleanupExpiredUserVideos(ctx context.Context, ttl time.Duration) (int, error) {
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
			protected, err = uc.compRepo.IsSavedAttemptWithVideo(ctx, userUUID, v.DanceID)
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

func (uc *ComparisonUsecase) GetMyDanceProgress(ctx context.Context, userID uuid.UUID, danceID string) ([]models.DanceProgressEntry, error) {
	return uc.compRepo.GetDanceProgress(ctx, userID, danceID)
}

func (uc *ComparisonUsecase) GetReelsAttempts(ctx context.Context, limit, offset int) ([]models.ReelsAttemptItem, error) {
	return uc.compRepo.GetReelsAttempts(ctx, limit, offset)
}

func (uc *ComparisonUsecase) GetFriendsDanceScores(ctx context.Context, userID uuid.UUID, danceID string) ([]models.FriendScore, error) {
	return uc.compRepo.GetFriendsDanceScores(ctx, userID, danceID)
}

func (uc *ComparisonUsecase) GetUserWeakSpots(ctx context.Context, userID uuid.UUID) (*models.WeakSpots, error) {
	return uc.compRepo.GetUserWeakSpots(ctx, userID)
}
