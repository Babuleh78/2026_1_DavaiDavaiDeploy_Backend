package http

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/dance"
	"DDDance/internal/pkg/helpers"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"unicode/utf8"

	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
)

const (
	cookieName     = "DDDanceJWT"
	deviceIDCookie = "DDDanceDeviceID"
	deviceIDMaxAge = 60 * 60 * 24 * 365
)

type DanceHandler struct {
	uc              dance.DanceUsecase
	cookieSecure    bool
	cookieSamesite  http.SameSite
	mlInternalToken string
}

func NewDanceHandler(uc dance.DanceUsecase, cookieSecure bool, cookieSameSite, mlInternalToken string) *DanceHandler {
	samesite := http.SameSiteLaxMode
	if cookieSameSite == "Strict" {
		samesite = http.SameSiteStrictMode
	}
	return &DanceHandler{
		uc:              uc,
		cookieSecure:    cookieSecure,
		cookieSamesite:  samesite,
		mlInternalToken: mlInternalToken,
	}
}

func getUserFromContext(ctx context.Context) *models.User {
	user, ok := ctx.Value(users.UserKey).(models.User)
	if !ok {
		return nil
	}
	return &user
}

func (d *DanceHandler) ensureDeviceID(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(deviceIDCookie); err == nil && c.Value != "" {
		return c.Value
	}
	id := uuid.NewV4().String()
	http.SetCookie(w, &http.Cookie{
		Name:     deviceIDCookie,
		Value:    id,
		HttpOnly: true,
		Secure:   d.cookieSecure,
		SameSite: d.cookieSamesite,
		MaxAge:   deviceIDMaxAge,
		Path:     "/",
	})
	return id
}

func writeModerationPending(w http.ResponseWriter, danceResult *models.UploadDanceResult) {
	danceID, reason := "", ""
	if danceResult != nil {
		danceID = danceResult.DanceID
		reason = danceResult.ModerationReason
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"error_code": "MODERATION_PENDING",
		"reason":     reason,
		"message":    "Видео не прошло модерацию",
		"dance_id":   danceID,
	})
}

func convertToH264(input []byte) ([]byte, error) {
	tmpDir := "/dddance-back/tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		tmpDir = "."
	}

	tmpIn, err := os.CreateTemp(tmpDir, "dance-input-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp input file: %w", err)
	}
	defer os.Remove(tmpIn.Name())
	defer tmpIn.Close()

	if _, err := tmpIn.Write(input); err != nil {
		return nil, fmt.Errorf("failed to write temp input file: %w", err)
	}
	tmpIn.Close()

	tmpOut, err := os.CreateTemp(tmpDir, "dance-output-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp output file: %w", err)
	}
	defer os.Remove(tmpOut.Name())
	tmpOut.Close()

	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", tmpIn.Name(),
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "23",
		"-r", "30",
		"-vsync", "cfr",
		"-c:a", "aac",
		"-movflags", "+faststart",
		"-f", "mp4",
		tmpOut.Name(),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w, stderr: %s", err, stderr.String())
	}

	result, err := os.ReadFile(tmpOut.Name())
	if err != nil {
		return nil, fmt.Errorf("failed to read output file: %w", err)
	}

	return result, nil
}

func trimAndConvertVideo(input []byte, startSec, endSec float64) ([]byte, error) {
	tmpDir := "/dddance-back/tmp"
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		tmpDir = "."
	}

	tmpIn, err := os.CreateTemp(tmpDir, "dance-input-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp input: %w", err)
	}
	defer os.Remove(tmpIn.Name())
	defer tmpIn.Close()

	if _, err := tmpIn.Write(input); err != nil {
		return nil, fmt.Errorf("failed to write input: %w", err)
	}
	tmpIn.Close()

	tmpOut, err := os.CreateTemp(tmpDir, "dance-output-*.mp4")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp output: %w", err)
	}
	defer os.Remove(tmpOut.Name())
	tmpOut.Close()

	cmd := exec.Command("ffmpeg",
		"-y",
		"-i", tmpIn.Name(),
		"-ss", fmt.Sprintf("%.3f", startSec),
		"-to", fmt.Sprintf("%.3f", endSec),
		"-c:v", "libx264",
		"-preset", "fast",
		"-crf", "23",
		"-r", "30",
		"-vsync", "cfr",
		"-c:a", "aac",
		"-movflags", "+faststart",
		"-f", "mp4",
		tmpOut.Name(),
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg failed: %w, stderr: %s", err, stderr.String())
	}

	return os.ReadFile(tmpOut.Name())
}

func (d *DanceHandler) LoadDance(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	const maxRequestBodySize = 60 * 1024 * 1024
	limitedReader := http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	defer func() { _ = limitedReader.Close() }()
	newReq := *r
	newReq.Body = limitedReader

	err := newReq.ParseMultipartForm(maxRequestBodySize)
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			log.LogHandlerError(logger, errors.New("file is too large"), http.StatusRequestEntityTooLarge)
			helpers.WriteError(w, http.StatusRequestEntityTooLarge)
			return
		}
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	defer func() {
		if newReq.MultipartForm != nil {
			_ = newReq.MultipartForm.RemoveAll()
		}
	}()

	file, _, err := newReq.FormFile("dance")
	if err != nil {
		log.LogHandlerError(logger, errors.New("failed to read file"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	defer func() { _ = file.Close() }()

	buffer, err := io.ReadAll(file)
	if err != nil {
		log.LogHandlerError(logger, errors.New("failed to read file"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	buffer, err = convertToH264(buffer)
	if err != nil {
		log.LogHandlerError(logger, fmt.Errorf("failed to convert video: %w", err), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var uploaderUserID, uploaderLogin string
	if user := getUserFromContext(r.Context()); user != nil {
		uploaderUserID = user.ID.String()
		uploaderLogin = user.Login
	}

	danceResult, err := d.uc.UploadDance(r.Context(), buffer, "video/mp4", uploaderUserID, uploaderLogin)
	if err != nil {
		switch err {
		case dance.ErrorModerationPending:
			writeModerationPending(w, danceResult)
		case dance.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		case dance.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	if user := getUserFromContext(r.Context()); user != nil {
		if histErr := d.uc.AddToHistory(r.Context(), user.ID, danceResult.DanceID, ""); histErr != nil {
			logger.Warn("failed to add dance to history", "error", histErr)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(models.AsyncEnqueueResult{
		TaskID:  danceResult.TaskID,
		DanceID: danceResult.DanceID,
		Status:  "queued",
	})
	log.LogHandlerInfo(logger, "queued", http.StatusAccepted)
}

func (d *DanceHandler) LoadDanceByURL(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	var req models.LoadDanceByURLInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.LogHandlerError(logger, fmt.Errorf("invalid request body: %w", err), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	req.Sanitize()

	if req.URL == "" {
		log.LogHandlerError(logger, errors.New("url is required"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var uploaderUserID, uploaderLogin string
	if user := getUserFromContext(r.Context()); user != nil {
		uploaderUserID = user.ID.String()
		uploaderLogin = user.Login
	}

	danceResult, err := d.uc.UploadDanceByURL(r.Context(), req.URL, uploaderUserID, uploaderLogin)
	if err != nil {
		switch err {
		case dance.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		case dance.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	if user := getUserFromContext(r.Context()); user != nil {
		if histErr := d.uc.AddToHistory(r.Context(), user.ID, danceResult.DanceID, req.URL); histErr != nil {
			logger.Warn("failed to add dance to history", "error", histErr)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(models.AsyncEnqueueResult{
		TaskID:  danceResult.TaskID,
		DanceID: danceResult.DanceID,
		Status:  "queued",
	})
	log.LogHandlerInfo(logger, "queued", http.StatusAccepted)
}

func (d *DanceHandler) TrimAndLoadDance(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	const maxRequestBodySize = 60 * 1024 * 1024
	limitedReader := http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	defer limitedReader.Close()
	newReq := *r
	newReq.Body = limitedReader

	if err := newReq.ParseMultipartForm(maxRequestBodySize); err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			helpers.WriteError(w, http.StatusRequestEntityTooLarge)
			return
		}
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	defer func() {
		if newReq.MultipartForm != nil {
			_ = newReq.MultipartForm.RemoveAll()
		}
	}()

	startStr := newReq.FormValue("start_sec")
	endStr := newReq.FormValue("end_sec")
	startSec, err := strconv.ParseFloat(startStr, 64)
	if err != nil || startSec < 0 {
		log.LogHandlerError(logger, errors.New("invalid start_sec"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	endSec, err := strconv.ParseFloat(endStr, 64)
	if err != nil || endSec <= startSec {
		log.LogHandlerError(logger, errors.New("invalid end_sec"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	file, _, err := newReq.FormFile("dance")
	if err != nil {
		log.LogHandlerError(logger, errors.New("failed to read file"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	defer file.Close()

	buffer, err := io.ReadAll(file)
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	buffer, err = trimAndConvertVideo(buffer, startSec, endSec)
	if err != nil {
		log.LogHandlerError(logger, fmt.Errorf("failed to trim video: %w", err), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var uploaderUserID, uploaderLogin string
	if user := getUserFromContext(r.Context()); user != nil {
		uploaderUserID = user.ID.String()
		uploaderLogin = user.Login
	}

	danceResult, err := d.uc.UploadDance(r.Context(), buffer, "video/mp4", uploaderUserID, uploaderLogin)
	if err != nil {
		switch err {
		case dance.ErrorModerationPending:
			writeModerationPending(w, danceResult)
		case dance.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		case dance.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	if user := getUserFromContext(r.Context()); user != nil {
		if histErr := d.uc.AddToHistory(r.Context(), user.ID, danceResult.DanceID, ""); histErr != nil {
			logger.Warn("failed to add dance to history", "error", histErr)
		}
	}

	response := models.LoadDanceResponse{
		DanceID:             danceResult.DanceID,
		FullGlbKey:          danceResult.FullGlbKey,
		GlbKeys:             danceResult.GlbKeys,
		SegmentsKey:         danceResult.SegmentsKey,
		NumFrames:           danceResult.NumFrames,
		NumSegments:         danceResult.NumSegments,
		DurationSec:         danceResult.DurationSec,
		NumSegmentsRendered: danceResult.NumSegmentsRendered,
		VideoPath:           danceResult.VideoPath,
	}
	response.Sanitize()

	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) GetDanceByID(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	danceID := mux.Vars(r)["id"]
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var userID *uuid.UUID
	if user := getUserFromContext(r.Context()); user != nil {
		userID = &user.ID
	}

	result, err := d.uc.GetDanceByID(r.Context(), danceID, userID)
	if err != nil {
		switch err {
		case dance.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	response := models.LoadDanceResponse{
		DanceID:             result.DanceID,
		FullGlbKey:          result.FullGlbKey,
		GlbKeys:             result.GlbKeys,
		SegmentsKey:         result.SegmentsKey,
		KeyframesURL:        result.KeyframesURL,
		Segments:            result.Segments,
		NumFrames:           result.NumFrames,
		NumSegments:         result.NumSegments,
		DurationSec:         result.DurationSec,
		NumSegmentsRendered: result.NumSegmentsRendered,
		VideoPath:           result.VideoPath,
		LikesCount:          result.LikesCount,
		IsLiked:             result.IsLiked,
		Author:              result.Author,
		Difficulty:          result.Difficulty,
		DifficultyByUsers:   result.DifficultyByUsers,
		LastAttemptID:       result.LastAttemptID,
		LastAttemptScore:    result.LastAttemptScore,
		UniqueViewersApprox: result.UniqueViewersApprox,
	}

	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) GetMainPage(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	videos, err := d.uc.GetMainPage(r.Context())
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	response := models.MainPageResponse{
		Count:  len(videos),
		Videos: videos,
	}

	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) GetSegmentDescription(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	vars := mux.Vars(r)
	danceID := vars["dance_id"]
	segmentIdxStr := vars["segment_idx"]

	if danceID == "" || segmentIdxStr == "" {
		log.LogHandlerError(logger, errors.New("dance_id and segment_idx are required"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	segmentIdx, err := strconv.Atoi(segmentIdxStr)
	if err != nil || segmentIdx < 0 {
		log.LogHandlerError(logger, errors.New("invalid segment_idx"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	result, err := d.uc.GetSegmentDescription(r.Context(), danceID, segmentIdx)
	if err != nil {
		switch err {
		case dance.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	response := models.SegmentDescriptionResponse{
		DanceID:     result.DanceID,
		SegmentIdx:  result.SegmentIdx,
		Description: result.Description,
	}

	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) GetDanceCatalog(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	sort := r.URL.Query().Get("sort")
	if sort == "" {
		sort = "popular"
	}
	search := r.URL.Query().Get("search")

	page := 1
	if v := r.URL.Query().Get("page"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			page = n
		}
	}

	limit := 12
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}

	result, err := d.uc.GetDanceCatalog(r.Context(), sort, search, page, limit)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) GetDanceTrending(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	result, err := d.uc.GetDanceTrending(r.Context())
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) GetDanceStatsHandler(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	id := mux.Vars(r)["id"]
	if id == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	stats, err := d.uc.GetDanceStats(r.Context(), id)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, stats)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) GetDanceModerationStatus(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	id := mux.Vars(r)["id"]
	if id == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	status, reason, err := d.uc.GetDanceModerationStatus(r.Context(), id)
	if err != nil {
		helpers.WriteError(w, http.StatusNotFound)
		return
	}

	helpers.WriteJSON(w, map[string]string{
		"dance_id":          id,
		"status":            status,
		"moderation_reason": reason,
	})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) GetDanceTimeline(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	vars := mux.Vars(r)
	danceID := vars["dance_id"]
	userID := vars["user_id"]

	if danceID == "" || userID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	data, err := d.uc.GetDanceTimeline(r.Context(), danceID, userID)
	if err != nil {
		switch err {
		case dance.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			log.LogHandlerError(logger, err, http.StatusInternalServerError)
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) GetDanceKeyframes(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	danceID := mux.Vars(r)["dance_id"]
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	data, err := d.uc.GetDanceKeyframes(r.Context(), danceID)
	if err != nil {
		switch err {
		case dance.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			log.LogHandlerError(logger, err, http.StatusInternalServerError)
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) UpdateDanceStatus(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	id := mux.Vars(r)["id"]
	if id == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var input models.DanceStatusInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if input.Status != "approved" && input.Status != "rejected" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := d.uc.UpdateDanceStatus(r.Context(), id, input.Status); err != nil {
		switch err {
		case dance.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

func (d *DanceHandler) RecordDanceView(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	danceID := mux.Vars(r)["id"]
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var viewerID string
	if user := getUserFromContext(r.Context()); user != nil {
		viewerID = user.ID.String()
	} else {
		viewerID = d.ensureDeviceID(w, r)
	}

	viewCount, err := d.uc.RecordDanceView(r.Context(), danceID, viewerID)
	if err != nil {
		switch err {
		case dance.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, map[string]int64{"view_count": viewCount})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) ClaimUploads(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	var input models.ClaimUploadsInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	if len(input.DanceIDs) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if err := d.uc.ClaimDanceUploads(r.Context(), user.ID, input.DanceIDs); err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

func (d *DanceHandler) GetUploadedDances(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	dances, err := d.uc.GetUploadedDances(r.Context(), user.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, dances)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) SetDanceName(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	danceID := mux.Vars(r)["dance_id"]
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var body struct {
		Title      string `json:"title"`
		Publish    bool   `json:"publish"`
		Difficulty string `json:"difficulty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	title := sanitizeDanceTitle(body.Title)
	if title == "" || containsBannedWords(title) {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := d.uc.SetDanceName(r.Context(), user.ID, danceID, title, body.Difficulty); err != nil {
		switch err {
		case dance.ErrorForbidden:
			helpers.WriteError(w, http.StatusForbidden)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	if body.Publish {
		if err := d.uc.PublishDance(r.Context(), user.ID, danceID); err != nil {
			helpers.WriteError(w, http.StatusInternalServerError)
			return
		}
	}

	helpers.WriteJSON(w, map[string]string{"status": "ok"})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) PublishDance(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	danceID := mux.Vars(r)["dance_id"]
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := d.uc.PublishDance(r.Context(), user.ID, danceID); err != nil {
		switch err {
		case dance.ErrorForbidden:
			helpers.WriteError(w, http.StatusForbidden)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, map[string]string{"status": "ok"})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) UnpublishDance(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	danceID := mux.Vars(r)["dance_id"]
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := d.uc.UnpublishDance(r.Context(), user.ID, danceID); err != nil {
		switch err {
		case dance.ErrorForbidden:
			helpers.WriteError(w, http.StatusForbidden)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, map[string]string{"status": "ok"})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) DeleteDance(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	danceID := mux.Vars(r)["dance_id"]
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := d.uc.DeleteDance(r.Context(), user.ID, danceID); err != nil {
		switch err {
		case dance.ErrorForbidden:
			helpers.WriteError(w, http.StatusForbidden)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, map[string]string{"status": "deleted"})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) GetTopDances(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	resp, err := d.uc.GetTopDances(r.Context())
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, resp)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (d *DanceHandler) UpdateSegmentDescription(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(r)
	danceID := vars["dance_id"]
	segmentIndexStr := vars["segment_index"]

	if danceID == "" || segmentIndexStr == "" {
		log.LogHandlerError(logger, errors.New("dance_id and segment_index are required"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	segmentIndex, err := strconv.Atoi(segmentIndexStr)
	if err != nil || segmentIndex < 0 {
		log.LogHandlerError(logger, errors.New("invalid segment_index"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var body struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.LogHandlerError(logger, err, http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if utf8.RuneCountInString(body.Description) > 500 {
		log.LogHandlerError(logger, errors.New("description exceeds 500 characters"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := d.uc.UpdateSegmentDescription(r.Context(), user.ID, danceID, segmentIndex, body.Description); err != nil {
		switch err {
		case dance.ErrorForbidden:
			helpers.WriteError(w, http.StatusForbidden)
		case dance.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		case dance.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

func (d *DanceHandler) PatchDanceDuration(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	expected := d.mlInternalToken
	if got := r.Header.Get("X-Internal-Token"); expected == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expected)) != 1 {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	danceID := mux.Vars(r)["dance_id"]
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var body struct {
		DurationSec int `json:"duration_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := d.uc.UpdateDanceDuration(r.Context(), danceID, body.DurationSec); err != nil {
		logger.Error("failed to update dance duration", "dance_id", danceID, "error", err)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

func (d *DanceHandler) GetDanceSegmentDescriptions(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	vars := mux.Vars(r)
	danceID := vars["dance_id"]

	if danceID == "" {
		log.LogHandlerError(logger, errors.New("dance_id is required"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	descs, err := d.uc.GetDanceChoreographerDescriptions(r.Context(), danceID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, descs)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}
