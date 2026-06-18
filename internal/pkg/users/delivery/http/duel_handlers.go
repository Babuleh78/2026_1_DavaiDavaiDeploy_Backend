package http

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/helpers"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
)

// CreateDuel godoc
// @Summary      Создать дуэль
// @Description  Бросить вызов другому пользователю. mode: single_dance (dance_id обязателен) или random_dance.
// @Tags         duels
// @Accept       json
// @Produce      json
// @Param        body  body  models.CreateDuelInput  true  "Параметры дуэли"
// @Success      201   {object}  models.DuelWithUsers
// @Failure      400
// @Failure      401
// @Failure      404
// @Failure      500
// @Router       /duels [post]
func (u *UserHandler) CreateDuel(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	caller, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		log.LogHandlerError(logger, errors.New("user unauthorized"), http.StatusUnauthorized)
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	var req models.CreateDuelInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.LogHandlerError(logger, err, http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	opponentID, err := uuid.FromString(req.OpponentID)
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	duel, err := u.uc.CreateDuel(r.Context(), caller.ID, opponentID, req.Mode, req.DanceID)
	if err != nil {
		switch {
		case errors.Is(err, users.ErrorBadRequest):
			helpers.WriteError(w, http.StatusBadRequest)
		case errors.Is(err, users.ErrorConflict):
			helpers.WriteError(w, http.StatusConflict)
		case errors.Is(err, users.ErrorNotFound):
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusCreated)
	helpers.WriteJSON(w, duel)
	log.LogHandlerInfo(logger, "success", http.StatusCreated)
}

// AcceptDuel godoc
// @Summary      Принять дуэль
// @Description  Принять входящий вызов. Только opponent может принять.
// @Tags         duels
// @Produce      json
// @Param        duel_id  path  string  true  "UUID дуэли"
// @Success      200   {object}  models.DuelWithUsers
// @Failure      400
// @Failure      401
// @Failure      403
// @Failure      404
// @Failure      500
// @Router       /duels/{duel_id}/accept [post]
// GetActiveDuelsForDance godoc
// @Summary      Активные дуэли пользователя по танцу
// @Description  Дуэли, где сейчас очередь пользователя сдать попытку на этом танце.
// @Tags         duels
// @Produce      json
// @Param        dance_id  query  string  true  "ID танца"
// @Success      200  {array}  models.ActiveDuelForDance
// @Router       /duels/active [get]
func (u *UserHandler) GetActiveDuelsForDance(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	caller, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	danceID := strings.TrimSpace(r.URL.Query().Get("dance_id"))
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	duels, err := u.uc.GetActiveDuelsForUserDance(r.Context(), caller.ID, danceID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, duels)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (u *UserHandler) SubmitAttemptToDuels(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	caller, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	var input struct {
		AttemptID string `json:"attempt_id"`
		DanceID   string `json:"dance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	danceID := strings.TrimSpace(input.DanceID)
	attemptID, err := uuid.FromString(strings.TrimSpace(input.AttemptID))
	if err != nil || danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := u.uc.SubmitAttemptToActiveDuels(r.Context(), caller.ID, danceID, attemptID, false); err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

func (u *UserHandler) AcceptDuel(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	caller, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		log.LogHandlerError(logger, errors.New("user unauthorized"), http.StatusUnauthorized)
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	duelID, err := uuid.FromString(mux.Vars(r)["duel_id"])
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	duel, err := u.uc.AcceptDuel(r.Context(), caller.ID, duelID)
	if err != nil {
		switch {
		case errors.Is(err, users.ErrorBadRequest), errors.Is(err, users.ErrorConflict):
			helpers.WriteError(w, http.StatusBadRequest)
		case errors.Is(err, users.ErrorForbidden):
			helpers.WriteError(w, http.StatusForbidden)
		case errors.Is(err, users.ErrorNotFound):
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, duel)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// DeclineDuel godoc
// @Summary      Отклонить дуэль
// @Description  Отклонить входящий вызов. Только opponent может отклонить.
// @Tags         duels
// @Param        duel_id  path  string  true  "UUID дуэли"
// @Success      204
// @Failure      400
// @Failure      401
// @Failure      403
// @Failure      404
// @Failure      500
// @Router       /duels/{duel_id}/decline [post]
func (u *UserHandler) DeclineDuel(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	caller, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		log.LogHandlerError(logger, errors.New("user unauthorized"), http.StatusUnauthorized)
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	duelID, err := uuid.FromString(mux.Vars(r)["duel_id"])
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := u.uc.DeclineDuel(r.Context(), caller.ID, duelID); err != nil {
		switch {
		case errors.Is(err, users.ErrorBadRequest), errors.Is(err, users.ErrorConflict):
			helpers.WriteError(w, http.StatusBadRequest)
		case errors.Is(err, users.ErrorForbidden):
			helpers.WriteError(w, http.StatusForbidden)
		case errors.Is(err, users.ErrorNotFound):
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

// GetDuelHistory godoc
// @Summary      История дуэлей текущего пользователя
// @Description  Возвращает список дуэлей (challenger или opponent), с пагинацией.
// @Tags         duels
// @Produce      json
// @Param        limit   query  int  false  "Лимит (по умолчанию 20)"
// @Param        offset  query  int  false  "Смещение (по умолчанию 0)"
// @Success      200  {object}  models.DuelHistoryResponse
// @Failure      401
// @Failure      500
// @Router       /duels [get]
func (u *UserHandler) GetDuelHistory(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	caller, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		log.LogHandlerError(logger, errors.New("user unauthorized"), http.StatusUnauthorized)
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	history, err := u.uc.GetDuelHistory(r.Context(), caller.ID, limit, offset)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, history)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetPublicDuels godoc
// @Summary      Публичные завершённые дуэли
// @Description  Возвращает список публичных завершённых дуэлей (is_public=true). Без авторизации.
// @Tags         duels
// @Produce      json
// @Param        limit   query  int  false  "Лимит (по умолчанию 20)"
// @Param        offset  query  int  false  "Смещение (по умолчанию 0)"
// @Success      200  {object}  models.DuelHistoryResponse
// @Failure      500
// @Router       /duels/public [get]
func (u *UserHandler) GetPublicDuels(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	limit := 20
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	offset := 0
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	result, err := u.uc.GetPublicDuels(r.Context(), limit, offset)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetDuelStats godoc
// @Summary      Статистика дуэлей пользователя
// @Description  Возвращает общую, победную, avg_score статистику по завершённым дуэлям.
// @Tags         duels
// @Produce      json
// @Success      200  {object}  models.DuelStats
// @Failure      401
// @Failure      500
// @Router       /duels/stats [get]
func (u *UserHandler) GetDuelStats(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	caller, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		log.LogHandlerError(logger, errors.New("user unauthorized"), http.StatusUnauthorized)
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	stats, err := u.uc.GetDuelStats(r.Context(), caller.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, stats)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetDuelByID godoc
// @Summary      Детали дуэли
// @Description  Возвращает информацию о дуэли. Scores скрыты до статуса completed.
// @Tags         duels
// @Produce      json
// @Param        duel_id  path  string  true  "UUID дуэли"
// @Success      200  {object}  models.DuelWithUsers
// @Failure      400
// @Failure      401
// @Failure      403
// @Failure      404
// @Failure      500
// @Router       /duels/{duel_id} [get]
func (u *UserHandler) GetDuelByID(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	caller, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		log.LogHandlerError(logger, errors.New("user unauthorized"), http.StatusUnauthorized)
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	duelID, err := uuid.FromString(mux.Vars(r)["duel_id"])
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	duel, err := u.uc.GetDuelByID(r.Context(), caller.ID, duelID)
	if err != nil {
		switch {
		case errors.Is(err, users.ErrorForbidden):
			helpers.WriteError(w, http.StatusForbidden)
		case errors.Is(err, users.ErrorNotFound):
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, duel)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}
