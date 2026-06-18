package http

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/helpers"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
)

func BotSecretMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secret := os.Getenv("BOT_SECRET")
		if secret == "" || r.Header.Get("X-Bot-Secret") != secret {
			helpers.WriteError(w, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type botLinkRequest struct {
	Code       string `json:"code"`
	TelegramID int64  `json:"telegram_id"`
}

func (u *UserHandler) BotLink(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	var req botLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Code == "" || req.TelegramID == 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	resp, err := u.uc.LinkTelegramAccount(r.Context(), req.Code, req.TelegramID)
	if err != nil {
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	if !resp.OK {
		w.WriteHeader(http.StatusNotFound)
	}
	helpers.WriteJSON(w, resp)
}

func (u *UserHandler) GetTelegramLinkCode(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	code, err := u.uc.GenerateTelegramLinkCode(r.Context(), user.ID)
	if err != nil {
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	deepLink := ""
	if botUsername := os.Getenv("TELEGRAM_BOT_USERNAME"); botUsername != "" {
		deepLink = "https://t.me/" + botUsername + "?start=" + code
	}

	helpers.WriteJSON(w, map[string]any{
		"code":       code,
		"deep_link":  deepLink,
		"expires_in": 600,
	})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

type botNotifyRequest struct {
	TelegramID int64          `json:"telegram_id"`
	Type       string         `json:"type"`
	Payload    map[string]any `json:"payload"`
}

func (u *UserHandler) BotNotify(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	var req botNotifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.TelegramID == 0 || req.Type == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := u.uc.BotPushNotification(r.Context(), req.TelegramID, req.Type, req.Payload); err != nil {
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (u *UserHandler) BotUpload(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	const maxSize = 50 * 1024 * 1024
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

	telegramIDStr := newReq.FormValue("telegram_id")
	if telegramIDStr == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil || telegramID == 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	danceID := newReq.FormValue("dance_id")
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	botUser, err := u.uc.BotGetUserByTelegramID(r.Context(), telegramID)
	if err != nil {
		if errors.Is(err, users.ErrorNotFound) {
			helpers.WriteError(w, http.StatusNotFound)
			return
		}
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	file, _, err := newReq.FormFile("video")
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

	buffer, err = helpers.ConvertToH264(buffer)
	if err != nil {
		log.LogHandlerError(logger, err, http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	userID := botUser.ID
	result, err := u.compUC.CompareDanceFromBuffer(r.Context(), buffer, "video/mp4", danceID, &userID)
	if err != nil {
		switch {
		case errors.Is(err, users.ErrorNotFound):
			helpers.WriteError(w, http.StatusNotFound)
		default:
			log.LogHandlerError(logger, err, http.StatusInternalServerError)
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(models.AsyncEnqueueResult{
		TaskID:           result.TaskID,
		UserDanceID:      result.UserDanceID,
		ReferenceDanceID: danceID,
		Status:           "processing",
	})
	log.LogHandlerInfo(logger, "bot upload queued", http.StatusAccepted)
}

func (u *UserHandler) BotTaskStatus(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	taskID := mux.Vars(r)["task_id"]
	if taskID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	telegramIDStr := r.URL.Query().Get("telegram_id")
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil || telegramID == 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	botUser, err := u.uc.BotGetUserByTelegramID(r.Context(), telegramID)
	if err != nil {
		if errors.Is(err, users.ErrorNotFound) {
			helpers.WriteError(w, http.StatusNotFound)
			return
		}
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	userID := botUser.ID
	status, err := u.compUC.GetTaskStatus(r.Context(), taskID, "compare", "", "", "", "", &userID)
	if err != nil {
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, status)
	log.LogHandlerInfo(logger, "bot task status", http.StatusOK)
}

func (u *UserHandler) BotGetAttempts(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	telegramIDStr := r.URL.Query().Get("telegram_id")
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil || telegramID == 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	botUser, err := u.uc.BotGetUserByTelegramID(r.Context(), telegramID)
	if err != nil {
		if errors.Is(err, users.ErrorNotFound) {
			helpers.WriteError(w, http.StatusNotFound)
			return
		}
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	attempts, err := u.compUC.GetUserAttempts(r.Context(), botUser.ID, 5, 0)
	if err != nil {
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	if attempts == nil {
		attempts = []models.UserAttemptItem{}
	}
	helpers.WriteJSON(w, attempts)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (u *UserHandler) BotGetAchievements(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	telegramIDStr := r.URL.Query().Get("telegram_id")
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil || telegramID == 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	botUser, err := u.uc.BotGetUserByTelegramID(r.Context(), telegramID)
	if err != nil {
		if errors.Is(err, users.ErrorNotFound) {
			helpers.WriteError(w, http.StatusNotFound)
			return
		}
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	result, err := u.uc.GetUserAchievements(r.Context(), botUser.ID)
	if err != nil {
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (u *UserHandler) BotGetStats(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	telegramIDStr := r.URL.Query().Get("telegram_id")
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil || telegramID == 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	botUser, err := u.uc.BotGetUserByTelegramID(r.Context(), telegramID)
	if err != nil {
		if errors.Is(err, users.ErrorNotFound) {
			helpers.WriteError(w, http.StatusNotFound)
			return
		}
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	stats, err := u.uc.GetUserBotStats(r.Context(), botUser.ID)
	if err != nil {
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, stats)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

var _activeDuelStatuses = map[string]bool{
	models.DuelStatusPending:        true,
	models.DuelStatusActive:         true,
	models.DuelStatusChallengerDone: true,
	models.DuelStatusOpponentDone:   true,
}

func (u *UserHandler) BotGetDuels(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	telegramIDStr := r.URL.Query().Get("telegram_id")
	telegramID, err := strconv.ParseInt(telegramIDStr, 10, 64)
	if err != nil || telegramID == 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	botUser, err := u.uc.BotGetUserByTelegramID(r.Context(), telegramID)
	if err != nil {
		if errors.Is(err, users.ErrorNotFound) {
			helpers.WriteError(w, http.StatusNotFound)
			return
		}
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	history, err := u.uc.GetDuelHistory(r.Context(), botUser.ID, 20, 0)
	if err != nil {
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	active := make([]models.DuelWithUsers, 0)
	for _, d := range history.Duels {
		if _activeDuelStatuses[d.Status] {
			active = append(active, d)
		}
	}

	helpers.WriteJSON(w, active)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (u *UserHandler) BotAcceptDuel(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	duelIDStr := mux.Vars(r)["duel_id"]
	duelID, err := uuid.FromString(duelIDStr)
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var body struct {
		TelegramID int64 `json:"telegram_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TelegramID == 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	botUser, err := u.uc.BotGetUserByTelegramID(r.Context(), body.TelegramID)
	if err != nil {
		if errors.Is(err, users.ErrorNotFound) {
			helpers.WriteError(w, http.StatusNotFound)
			return
		}
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	duel, err := u.uc.AcceptDuel(r.Context(), botUser.ID, duelID)
	if err != nil {
		switch {
		case errors.Is(err, users.ErrorNotFound):
			helpers.WriteError(w, http.StatusNotFound)
		case errors.Is(err, users.ErrorForbidden):
			helpers.WriteError(w, http.StatusForbidden)
		default:
			log.LogHandlerError(logger, err, http.StatusInternalServerError)
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, duel)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (u *UserHandler) BotDeclineDuel(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	duelIDStr := mux.Vars(r)["duel_id"]
	duelID, err := uuid.FromString(duelIDStr)
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	var body struct {
		TelegramID int64 `json:"telegram_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TelegramID == 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	botUser, err := u.uc.BotGetUserByTelegramID(r.Context(), body.TelegramID)
	if err != nil {
		if errors.Is(err, users.ErrorNotFound) {
			helpers.WriteError(w, http.StatusNotFound)
			return
		}
		log.LogHandlerError(logger, err, http.StatusInternalServerError)
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	if err := u.uc.DeclineDuel(r.Context(), botUser.ID, duelID); err != nil {
		switch {
		case errors.Is(err, users.ErrorNotFound):
			helpers.WriteError(w, http.StatusNotFound)
		case errors.Is(err, users.ErrorForbidden):
			helpers.WriteError(w, http.StatusForbidden)
		default:
			log.LogHandlerError(logger, err, http.StatusInternalServerError)
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}
