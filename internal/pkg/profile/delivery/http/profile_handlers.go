package http

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/helpers"
	"DDDance/internal/pkg/profile"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
)

const (
	cookieName     = "DDDanceJWT"
	csrfCookieName = "DDDanceCSRF"
)

type ProfileHandler struct {
	uc             profile.ProfileUsecase
	cookieSecure   bool
	cookieSamesite http.SameSite
}

func NewProfileHandler(uc profile.ProfileUsecase) *ProfileHandler {
	secure := false
	if os.Getenv("COOKIE_SECURE") == "true" {
		secure = true
	}
	samesite := http.SameSiteLaxMode
	if os.Getenv("COOKIE_SAMESITE") == "Strict" {
		samesite = http.SameSiteStrictMode
	}
	return &ProfileHandler{
		uc:             uc,
		cookieSecure:   secure,
		cookieSamesite: samesite,
	}
}

func getUserFromContext(ctx context.Context) *models.User {
	user, ok := ctx.Value(users.UserKey).(models.User)
	if !ok {
		return nil
	}
	return &user
}

func (h *ProfileHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	neededUser, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		log.LogHandlerError(logger, errors.New("user unauthorized"), http.StatusUnauthorized)
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	const maxSize = 5 * 1024 * 1024
	limitedReader := http.MaxBytesReader(w, r.Body, maxSize)
	defer func() { _ = limitedReader.Close() }()
	newReq := *r
	newReq.Body = limitedReader

	if err := newReq.ParseMultipartForm(maxSize); err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			helpers.WriteError(w, http.StatusRequestEntityTooLarge)
			return
		}
		log.LogHandlerError(logger, err, http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	defer func() {
		if newReq.MultipartForm != nil {
			_ = newReq.MultipartForm.RemoveAll()
		}
	}()

	var newLogin *string
	if _, present := newReq.MultipartForm.Value["login"]; present {
		v := newReq.FormValue("login")
		newLogin = &v
	}

	var avatarBuffer []byte
	var avatarContentType string
	if files, has := newReq.MultipartForm.File["avatar"]; has && len(files) > 0 {
		fileHeader := files[0]
		f, err := fileHeader.Open()
		if err != nil {
			log.LogHandlerError(logger, fmt.Errorf("failed to open avatar: %w", err), http.StatusBadRequest)
			helpers.WriteError(w, http.StatusBadRequest)
			return
		}
		defer f.Close()

		sniff := make([]byte, 512)
		n, _ := f.Read(sniff)
		avatarContentType = http.DetectContentType(sniff[:n])
		if _, err = f.Seek(0, 0); err != nil {
			log.LogHandlerError(logger, fmt.Errorf("failed to seek avatar: %w", err), http.StatusInternalServerError)
			helpers.WriteError(w, http.StatusInternalServerError)
			return
		}
		avatarBuffer, err = io.ReadAll(f)
		if err != nil {
			log.LogHandlerError(logger, err, http.StatusBadRequest)
			helpers.WriteError(w, http.StatusBadRequest)
			return
		}
	}

	if newLogin == nil && len(avatarBuffer) == 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	updated, newToken, err := h.uc.UpdateProfile(r.Context(), neededUser.ID, newLogin, avatarBuffer, avatarContentType)
	if err != nil {
		switch err {
		case users.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		case users.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	if newToken != "" {
		http.SetCookie(w, &http.Cookie{
			Name:     cookieName,
			Value:    newToken,
			HttpOnly: true,
			Secure:   h.cookieSecure,
			SameSite: h.cookieSamesite,
			Expires:  time.Now().Add(12 * time.Hour),
			Path:     "/",
		})
	}

	updated.Sanitize()
	helpers.WriteJSON(w, updated)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ProfileHandler) GetPublicProfile(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	profileID, err := uuid.FromString(mux.Vars(r)["id"])
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var viewerID *uuid.UUID
	if viewer := getUserFromContext(r.Context()); viewer != nil {
		viewerID = &viewer.ID
	}

	prof, err := h.uc.GetPublicProfile(r.Context(), profileID, viewerID)
	if err != nil {
		switch err {
		case users.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, prof)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// SearchUsers godoc
// @Summary      Search users by login
// @Tags         profile
// @Produce      json
// @Param        q  query  string  true  "Search query (min 2 chars)"
// @Success      200  {array}  models.UserSearchItem
// @Router       /users/search [get]
func (h *ProfileHandler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) < 2 {
		helpers.WriteJSON(w, []models.UserSearchItem{})
		return
	}

	results, err := h.uc.SearchUsers(r.Context(), query)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, results)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ProfileHandler) GetSearchHistory(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	items, err := h.uc.GetHistory(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(items)
}

func (h *ProfileHandler) DeleteFromHistory(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	historyID := uuid.FromStringOrNil(mux.Vars(r)["history_id"])
	if historyID == uuid.Nil {
		http.Error(w, "invalid history_id", http.StatusBadRequest)
		return
	}
	err := h.uc.DeleteFromHistory(r.Context(), historyID, user.ID)
	if err != nil {
		if err == users.ErrorNotFound {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ProfileHandler) UpdateHistoryName(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r.Context())
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	historyID := uuid.FromStringOrNil(mux.Vars(r)["history_id"])
	if historyID == uuid.Nil {
		http.Error(w, "invalid history_id", http.StatusBadRequest)
		return
	}
	var input models.UpdateHistoryNameInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.Name == "" {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	err := h.uc.UpdateHistoryName(r.Context(), historyID, user.ID, input.Name)
	if err != nil {
		if err == users.ErrorNotFound {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ProfileHandler) SendFriendRequest(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	sender, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	receiverID, err := uuid.FromString(mux.Vars(r)["id"])
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := h.uc.SendFriendRequest(r.Context(), sender.ID, receiverID); err != nil {
		switch err {
		case users.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		case users.ErrorAlreadyExists:
			helpers.WriteError(w, http.StatusConflict)
		case users.ErrorForbidden:
			helpers.WriteError(w, http.StatusForbidden)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, map[string]string{"status": "ok"})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ProfileHandler) RespondFriendRequest(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	friendshipID, err := strconv.ParseInt(mux.Vars(r)["id"], 10, 64)
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var body struct {
		Accept bool `json:"accept"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := h.uc.RespondFriendRequest(r.Context(), user.ID, friendshipID, body.Accept); err != nil {
		switch err {
		case users.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		case users.ErrorForbidden:
			helpers.WriteError(w, http.StatusForbidden)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, map[string]string{"status": "ok"})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ProfileHandler) GetFriends(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	friends, err := h.uc.GetFriends(r.Context(), user.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, friends)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ProfileHandler) GetPublicFriends(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	profileUserID, err := uuid.FromString(mux.Vars(r)["id"])
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	friends, err := h.uc.GetFriends(r.Context(), profileUserID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, friends)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ProfileHandler) RemoveFriend(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	friendID, err := uuid.FromString(mux.Vars(r)["friend_id"])
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := h.uc.RemoveFriend(r.Context(), user.ID, friendID); err != nil {
		switch err {
		case users.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		case users.ErrorForbidden:
			helpers.WriteError(w, http.StatusForbidden)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, map[string]string{"status": "ok"})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ProfileHandler) GetUserActivity(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	profileID, err := uuid.FromString(mux.Vars(r)["id"])
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	items, err := h.uc.GetUserActivity(r.Context(), profileID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, items)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ProfileHandler) GetCreatorAnalytics(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	result, err := h.uc.GetCreatorAnalytics(r.Context(), user.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (h *ProfileHandler) GetMostImprovedDance(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	result, err := h.uc.GetMostImprovedDance(r.Context(), user.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	if result == nil {
		helpers.WriteJSON(w, nil)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}
