package http

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/helpers"
	"DDDance/internal/pkg/social"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
)

type SocialHandler struct {
	uc social.SocialUsecase
}

func NewSocialHandler(uc social.SocialUsecase) *SocialHandler {
	return &SocialHandler{uc: uc}
}

func getUserFromContext(ctx context.Context) *models.User {
	user, ok := ctx.Value(users.UserKey).(models.User)
	if !ok {
		return nil
	}
	return &user
}

func (h *SocialHandler) ToggleLike(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user := getUserFromContext(r.Context())
	if user == nil {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	danceID := mux.Vars(r)["id"]
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	result, err := h.uc.ToggleLike(r.Context(), user.ID, danceID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *SocialHandler) GetUserLikedDances(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		log.LogHandlerError(logger, errors.New("user unauthorized"), http.StatusUnauthorized)
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	likes, err := h.uc.GetUserLikedDances(r.Context(), user.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	response := models.UserLikedDancesResponse{Likes: likes}
	if response.Likes == nil {
		response.Likes = []models.DanceLike{}
	}

	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *SocialHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	resp, err := h.uc.GetNotifications(r.Context(), user.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, resp)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *SocialHandler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	id, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil || id <= 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := h.uc.MarkNotificationRead(r.Context(), id, user.ID); err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

func (h *SocialHandler) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	if err := h.uc.MarkAllNotificationsRead(r.Context(), user.ID); err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

func (h *SocialHandler) ClearNotifications(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	if err := h.uc.ClearNotifications(r.Context(), user.ID); err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

func (h *SocialHandler) SaveRating(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	var req models.SaveRatingInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.LogHandlerError(logger, errors.New("invalid request"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		log.LogHandlerError(logger, err, http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	result, err := h.uc.SaveRating(r.Context(), user.ID, req)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *SocialHandler) GetRating(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	danceID := r.URL.Query().Get("dance_id")
	if danceID == "" {
		log.LogHandlerError(logger, errors.New("dance_id is required"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	rating, err := h.uc.GetRating(r.Context(), danceID)
	if err != nil {
		switch err {
		case social.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, rating)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *SocialHandler) GetFriendsFeed(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 50 {
			limit = v
		}
	}

	var cursor time.Time
	if c := r.URL.Query().Get("cursor"); c != "" {
		if t, err := time.Parse(time.RFC3339Nano, c); err == nil {
			cursor = t
		}
	}

	resp, err := h.uc.GetFriendsFeed(r.Context(), user.ID, limit, cursor)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, resp)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}
