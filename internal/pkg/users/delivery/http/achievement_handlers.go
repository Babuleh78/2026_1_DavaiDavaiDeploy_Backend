package http

import (
	"DDDance/internal/pkg/helpers"
	"DDDance/internal/pkg/utils/log"
	"log/slog"
	"net/http"

	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
)

// GetAllAchievements godoc
// @Summary      Список всех достижений
// @Description  Возвращает полный каталог достижений без привязки к конкретному пользователю.
// @Tags         achievements
// @Produce      json
// @Success      200  {array}   models.Achievement
// @Failure      500
// @Router       /achievements [get]
func (u *UserHandler) GetAllAchievements(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	achievements, err := u.uc.GetAllAchievements(r.Context())
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, achievements)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetUserAchievements godoc
// @Summary      Достижения пользователя
// @Description  Возвращает все достижения с флагом разблокировки для указанного пользователя.
// @Tags         achievements
// @Produce      json
// @Param        user_id  path  string  true  "UUID пользователя"
// @Success      200  {array}   models.AchievementWithStatus
// @Failure      400
// @Failure      500
// @Router       /users/{user_id}/achievements [get]
func (u *UserHandler) GetUserAchievements(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	userID, err := uuid.FromString(mux.Vars(r)["user_id"])
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	achievements, err := u.uc.GetUserAchievements(r.Context(), userID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, achievements)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}
