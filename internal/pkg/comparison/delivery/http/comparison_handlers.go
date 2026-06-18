package http

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/comparison"
	"DDDance/internal/pkg/helpers"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
)

type ComparisonHandler struct {
	uc comparison.ComparisonUsecase
}

func NewComparisonHandler(uc comparison.ComparisonUsecase) *ComparisonHandler {
	return &ComparisonHandler{uc: uc}
}

func getUserFromContext(ctx context.Context) *models.User {
	user, ok := ctx.Value(users.UserKey).(models.User)
	if !ok {
		return nil
	}
	return &user
}

func (h *ComparisonHandler) SaveAttempt(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}
	attemptID := mux.Vars(r)["user_dance_id"]
	if attemptID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var input models.SaveAttemptInput
	if r.Body != nil && r.ContentLength != 0 {
		_ = json.NewDecoder(r.Body).Decode(&input)
	}
	if input.DanceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	input.UserName = sanitizeDanceTitle(input.UserName)
	if containsBannedWords(input.UserName) {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var duelID *uuid.UUID
	if input.DuelID != nil && *input.DuelID != "" {
		if parsed, err := uuid.FromString(*input.DuelID); err == nil {
			duelID = &parsed
		}
	}

	derefF := func(p *float64) float64 {
		if p != nil {
			return *p
		}
		return 0
	}
	if err := h.uc.SaveAttempt(r.Context(), user.ID, attemptID, input.DanceID, input.IncludeVideo, input.Score, input.UserName, input.IsPrivate, duelID, input.PublicConsent, input.SubmitToDuels, derefF(input.TimingScore), derefF(input.AmplitudeScore), derefF(input.PoseScore)); err != nil {
		switch err {
		case comparison.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		case comparison.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

func (h *ComparisonHandler) UnsaveAttempt(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}
	attemptID := mux.Vars(r)["user_dance_id"]
	if attemptID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := h.uc.UnsaveAttempt(r.Context(), user.ID, attemptID); err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

func (h *ComparisonHandler) GetSavedAttempts(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	limit, offset := parsePaginationParams(r, 50, 100)

	items, err := h.uc.GetSavedAttempts(r.Context(), user.ID, limit, offset)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, items)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ComparisonHandler) GetUserAttempts(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	limit, offset := parsePaginationParams(r, 50, 100)

	items, err := h.uc.GetUserAttempts(r.Context(), user.ID, limit, offset)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, items)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func parsePaginationParams(r *http.Request, defaultLimit, maxLimit int) (limit, offset int) {
	limit = defaultLimit
	offset = 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

func (h *ComparisonHandler) GetCompareResult(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	userDanceID := mux.Vars(r)["user_dance_id"]
	if userDanceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	result, err := h.uc.GetCompareResult(r.Context(), user.ID, userDanceID)
	if err != nil {
		switch err {
		case comparison.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		case comparison.ErrorForbidden:
			helpers.WriteError(w, http.StatusForbidden)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ComparisonHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
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

	result, err := h.uc.GetLeaderboard(r.Context(), danceID, userID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ComparisonHandler) GetTopDancers(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	entries, err := h.uc.GetTopDancers(r.Context())
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, entries)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ComparisonHandler) GetMyDanceProgress(w http.ResponseWriter, r *http.Request) {
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

	entries, err := h.uc.GetMyDanceProgress(r.Context(), user.ID, danceID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, entries)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ComparisonHandler) CompareDanceWithFile(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	var userID *uuid.UUID
	if user := getUserFromContext(r.Context()); user != nil {
		id := user.ID
		userID = &id
	}

	const maxSize = 60 * 1024 * 1024
	limitedReader := http.MaxBytesReader(w, r.Body, maxSize)
	defer limitedReader.Close()
	newReq := *r
	newReq.Body = limitedReader

	if err := newReq.ParseMultipartForm(maxSize); err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	defer func() {
		if newReq.MultipartForm != nil {
			_ = newReq.MultipartForm.RemoveAll()
		}
	}()

	referenceDanceID := newReq.FormValue("reference_dance_id")
	if referenceDanceID == "" {
		log.LogHandlerError(logger, fmt.Errorf("reference_dance_id is required"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	file, _, err := newReq.FormFile("dance")
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	defer file.Close()

	buffer, err := io.ReadAll(file)
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	startStr := newReq.FormValue("start_sec")
	endStr := newReq.FormValue("end_sec")

	logger.Info("CompareDanceWithFile called",
		"bufferSize", len(buffer),
		"referenceDanceID", referenceDanceID,
		"authenticated", userID != nil,
		"startSec", startStr,
		"endSec", endStr,
	)

	if startStr != "" && endStr != "" {
		startSec, errStart := strconv.ParseFloat(startStr, 64)
		endSec, errEnd := strconv.ParseFloat(endStr, 64)
		if errStart == nil && errEnd == nil && endSec > startSec && startSec >= 0 {
			buffer, err = helpers.TrimAndConvertVideo(buffer, startSec, endSec)
			if err != nil {
				log.LogHandlerError(logger, fmt.Errorf("failed to trim: %w", err), http.StatusBadRequest)
				helpers.WriteError(w, http.StatusBadRequest)
				return
			}
		} else {
			buffer, err = helpers.ConvertToH264(buffer)
		}
	} else {
		buffer, err = helpers.ConvertToH264(buffer)
	}
	if err != nil {
		log.LogHandlerError(logger, fmt.Errorf("failed to convert: %w", err), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	result, err := h.uc.CompareDanceFromBuffer(r.Context(), buffer, "video/mp4", referenceDanceID, userID)
	if err != nil {
		logger.Error("CompareDanceFromBuffer failed", "error", err)
		switch err {
		case comparison.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(models.AsyncEnqueueResult{
		TaskID:           result.TaskID,
		UserDanceID:      result.UserDanceID,
		ReferenceDanceID: result.DanceID,
		Status:           "queued",
	})
	log.LogHandlerInfo(logger, "queued", http.StatusAccepted)
}

func (h *ComparisonHandler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	taskID := mux.Vars(r)["task_id"]
	if taskID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	taskType := r.URL.Query().Get("type")
	danceID := r.URL.Query().Get("dance_id")
	userDanceID := r.URL.Query().Get("user_dance_id")
	videoKey := r.URL.Query().Get("video_key")

	var userID *uuid.UUID
	var uploaderUserID string
	if user := getUserFromContext(r.Context()); user != nil {
		id := user.ID
		userID = &id
		uploaderUserID = user.ID.String()
	}

	result, err := h.uc.GetTaskStatus(r.Context(), taskID, taskType, danceID, userDanceID, videoKey, uploaderUserID, userID)
	if err != nil {
		logger.Error("GetTaskStatus failed", "error", err)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ComparisonHandler) GetReelsAttempts(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	limit := 20
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	items, err := h.uc.GetReelsAttempts(r.Context(), limit, offset)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, items)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ComparisonHandler) GetFriendsDanceScores(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user := getUserFromContext(r.Context())
	if user == nil {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	danceID := mux.Vars(r)["dance_id"]
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	scores, err := h.uc.GetFriendsDanceScores(r.Context(), user.ID, danceID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, scores)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ComparisonHandler) GetWeakSpots(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user := getUserFromContext(r.Context())
	if user == nil {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	ws, err := h.uc.GetUserWeakSpots(r.Context(), user.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, ws)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}
