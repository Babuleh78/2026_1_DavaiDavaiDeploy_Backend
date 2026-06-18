package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"DDDance/internal/models"
	"DDDance/internal/pkg/helpers"
	"DDDance/internal/pkg/recommendation"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"

	uuid "github.com/satori/go.uuid"
)

type RecommendHandler struct {
	uc recommendation.RecommendationUsecase
}

func NewRecommendHandler(uc recommendation.RecommendationUsecase) *RecommendHandler {
	return &RecommendHandler{uc: uc}
}

func (h *RecommendHandler) Recommend(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	body.Query = strings.TrimSpace(body.Query)
	if body.Query == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	resp, err := h.uc.GetRecommendations(r.Context(), body.Query)
	if err != nil {
		logger.Error("recommend failed", "error", err)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, resp)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *RecommendHandler) GetReelsFeed(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	limit := 5
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 20 {
				n = 20
			}
			limit = n
		}
	}

	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	var excludeIDs []string
	if v := r.URL.Query().Get("exclude_ids"); v != "" {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				excludeIDs = append(excludeIDs, part)
			}
		}
	}

	var userID *uuid.UUID
	if user, ok := r.Context().Value(users.UserKey).(models.User); ok {
		id := user.ID
		userID = &id
	}

	var behaviorLog []models.BehaviorLogEntry
	if v := r.URL.Query().Get("behavior_log"); v != "" {
		if err := json.Unmarshal([]byte(v), &behaviorLog); err != nil {
			logger.Warn("failed to parse behavior_log query param", "error", err)
			behaviorLog = nil
		}
	}

	resp, err := h.uc.GetReelsFeed(r.Context(), limit, offset, excludeIDs, userID, behaviorLog)
	if err != nil {
		logger.Error("get reels feed failed", "error", err)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, resp)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *RecommendHandler) GetSimilarDances(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	danceID := strings.TrimSpace(r.URL.Query().Get("dance_id"))
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	dances, err := h.uc.GetSimilarDances(r.Context(), danceID)
	if err != nil {
		logger.Error("get similar dances failed", "error", err)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	if dances == nil {
		helpers.WriteJSON(w, []interface{}{})
		return
	}

	helpers.WriteJSON(w, dances)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}
