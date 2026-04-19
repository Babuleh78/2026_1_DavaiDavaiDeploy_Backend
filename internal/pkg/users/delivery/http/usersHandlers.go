package http

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/auth/delivery/grpc/gen"
	"DDDance/internal/pkg/helpers"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	CookieName     = "DDFilmsJWT"
	CSRFCookieName = "DDFilmsCSRF"
)

type UserHandler struct {
	client         gen.AuthClient
	uc             users.UsersUsecase
	cookieSecure   bool
	cookieSamesite http.SameSite
}

func NewUserHandler(client gen.AuthClient, uc users.UsersUsecase) *UserHandler {
	secure := false
	cookieValue := os.Getenv("COOKIE_SECURE")
	if cookieValue == "true" {
		secure = true
	}

	samesite := http.SameSiteLaxMode
	samesiteValue := os.Getenv("COOKIE_SAMESITE")
	if samesiteValue == "Strict" {
		samesite = http.SameSiteStrictMode
	}
	return &UserHandler{
		client:         client,
		uc:             uc,
		cookieSecure:   secure,
		cookieSamesite: samesite,
	}
}

func (u *UserHandler) JWTMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))
		var token string
		cookie, err := r.Cookie(CookieName)
		if err == nil {
			token = cookie.Value
		}

		user, err := u.client.ValidateAndGetUser(r.Context(), &gen.ValidateAndGetUserRequest{Token: token})
		if err != nil {
			st, _ := status.FromError(err)
			switch st.Code() {
			case codes.Unauthenticated:
				helpers.WriteError(w, http.StatusUnauthorized)
			default:
				helpers.WriteError(w, http.StatusInternalServerError)
			}
			return
		}
		neededUser := models.User{
			ID: uuid.FromStringOrNil(user.ID),
		}
		ctx := context.WithValue(r.Context(), users.UserKey, neededUser)

		log.LogHandlerInfo(logger, "success", http.StatusOK)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (u *UserHandler) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))
		csrfCookie, err := r.Cookie(CSRFCookieName)
		if err != nil {
			log.LogHandlerError(logger, errors.New("invalid csrf token"), http.StatusUnauthorized)
			helpers.WriteError(w, http.StatusUnauthorized)
			return
		}
		var csrfToken string

		tokenFromHeader := r.Header.Get("X-CSRF-Token")
		if tokenFromHeader != "" {
			csrfToken = tokenFromHeader
		} else {
			tokenFromForm := r.FormValue("csrftoken")
			if tokenFromForm != "" {
				csrfToken = tokenFromForm
			} else {
				log.LogHandlerError(logger, errors.New("csrf-token is empty"), http.StatusUnauthorized)
				helpers.WriteError(w, http.StatusUnauthorized)
				return
			}
		}

		if csrfCookie.Value != csrfToken {
			log.LogHandlerError(logger, errors.New("invalid csrf-token"), http.StatusUnauthorized)
			helpers.WriteError(w, http.StatusUnauthorized)
			return
		}
		var token string
		cookie, err := r.Cookie(CookieName)
		if err == nil {
			token = cookie.Value
		}

		user, err := u.client.ValidateAndGetUser(r.Context(), &gen.ValidateAndGetUserRequest{Token: token})
		if err != nil {
			st, _ := status.FromError(err)
			switch st.Code() {
			case codes.Unauthenticated:
				helpers.WriteError(w, http.StatusUnauthorized)
			default:
				helpers.WriteError(w, http.StatusInternalServerError)
			}
		}
		neededUser := models.User{
			ID: uuid.FromStringOrNil(user.ID),
		}
		ctx := context.WithValue(r.Context(), users.UserKey, neededUser)

		log.LogHandlerInfo(logger, "success", http.StatusOK)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUser godoc
// @Summary      Получить информацию о пользователе по ID
// @Description  Возвращает публичные данные пользователя (ID, версию, логин, аватар)
// @Tags         users
// @Security     ApiKeyAuth
// @Param        id   path      string  true  "UUID пользователя"
// @Success      200  {object}  models.User
// @Failure      400  {string}  string  "Неверный формат ID"
// @Failure      401  {string}  string  "Пользователь не авторизован"
// @Failure      500  {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/{id} [get]
func (u *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))
	vars := mux.Vars(r)
	id, err := uuid.FromString(vars["id"])
	if err != nil {
		log.LogHandlerError(logger, errors.New("invalid id of user"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	neededUser, err := u.client.GetUser(r.Context(), &gen.GetUserRequest{ID: id.String()})
	if err != nil {
		st, _ := status.FromError(err)
		switch st.Code() {
		case codes.Unauthenticated:
			helpers.WriteError(w, http.StatusUnauthorized)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	response := models.User{
		ID:      uuid.FromStringOrNil(neededUser.ID),
		Version: int(neededUser.Version),
		Login:   neededUser.Login,
		Avatar:  neededUser.Avatar,
	}

	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// ChangePassword godoc
// @Summary      Изменить пароль текущего пользователя
// @Description  Изменяет пароль, возвращает обновлённый JWT и CSRF-токен в куках и заголовке X-CSRF-Token
// @Tags         users
// @Security     ApiKeyAuth
// @Accept       json
// @Produce      json
// @Param        request  body      models.ChangePasswordInput  true  "Старый и новый пароль"
// @Success      200      {object}  models.User
// @Failure      400      {string}  string  "Неверный запрос"
// @Failure      401      {string}  string  "Пользователь не авторизован"
// @Failure      404      {string}  string  "Пользователь не найден"
// @Failure      500      {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/change/password [put]
func (u *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))
	userID, ok := r.Context().Value(users.UserKey).(uuid.UUID)
	if !ok {
		log.LogHandlerError(logger, errors.New("user unauthorized"), http.StatusUnauthorized)
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	var req models.ChangePasswordInput
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.LogHandlerError(logger, errors.New("invalid request"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	req.Sanitize()

	user, err := u.client.ChangePassword(r.Context(), &gen.ChangePasswordRequest{
		OldPassword: req.OldPassword,
		NewPassword: req.NewPassword,
		UserID:      userID.String()})

	if err != nil {
		st, _ := status.FromError(err)
		switch st.Code() {
		case codes.InvalidArgument:
			helpers.WriteError(w, http.StatusBadRequest)
		case codes.NotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    user.CSRFToken,
		HttpOnly: false,
		Secure:   u.cookieSecure,
		SameSite: u.cookieSamesite,
		Expires:  time.Now().Add(12 * time.Hour),
		Path:     "/",
	})

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    user.JWTToken,
		HttpOnly: true,
		Secure:   u.cookieSecure,
		SameSite: u.cookieSamesite,
		Expires:  time.Now().Add(12 * time.Hour),
		Path:     "/",
	})

	response := models.User{
		ID:      uuid.FromStringOrNil(user.User.ID),
		Version: int(user.User.Version),
		Login:   user.User.Login,
		Avatar:  user.User.Avatar,
	}

	w.Header().Set("X-CSRF-Token", user.CSRFToken)
	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// LoadDance godoc
// @Summary      Загрузить видео танца и обработать его
// @Description  Принимает видеофайл через multipart/form-data, конвертирует в H.264 и отправляет на анализ
// @Tags         dances
// @Security     OptionalAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        dance  formData  file  true  "Видеофайл (до 60 МБ)"
// @Success      200    {object}  models.LoadDanceResponse
// @Failure      400    {string}  string  "Ошибка чтения файла или неверный запрос"
// @Failure      413    {string}  string  "Файл слишком большой"
// @Failure      500    {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/load [post]
func (u *UserHandler) LoadDance(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	const maxRequestBodySize = 60 * 1024 * 1024
	limitedReader := http.MaxBytesReader(w, r.Body, maxRequestBodySize)
	defer func() {
		_ = limitedReader.Close()
	}()
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
	defer func() {
		_ = file.Close()
	}()

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

	danceResult, err := u.uc.UploadDance(r.Context(), buffer, "video/mp4")
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

	if user := getUserFromContext(r.Context()); user != nil {
		_ = u.uc.AddToHistory(r.Context(), user.ID, danceResult.DanceID, "")
	}

	response := models.LoadDanceResponse{
		DanceID:             danceResult.DanceID,
		FullGlbKey:          danceResult.FullGlbKey,
		GlbKeys:             danceResult.GlbKeys,
		SegmentsKey:         danceResult.SegmentsKey,
		NumFrames:           int(danceResult.NumFrames),
		NumSegments:         int(danceResult.NumSegments),
		DurationSec:         danceResult.DurationSec,
		NumSegmentsRendered: int(danceResult.NumSegmentsRendered),
		VideoPath:           danceResult.VideoPath,
	}

	response.Sanitize()

	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// LoadDanceByURL godoc
// @Summary      Загрузить танец по URL видео
// @Description  Принимает JSON с URL, скачивает видео и обрабатывает его
// @Tags         dances
// @Security     OptionalAuth
// @Accept       json
// @Produce      json
// @Param        request  body      models.LoadDanceByURLInput  true  "URL видео"
// @Success      200      {object}  models.LoadDanceResponse
// @Failure      400      {string}  string  "Неверный запрос или отсутствует URL"
// @Failure      404      {string}  string  "Видео не найдено"
// @Failure      500      {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/loadByURL [post]
func (u *UserHandler) LoadDanceByURL(w http.ResponseWriter, r *http.Request) {
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

	danceResult, err := u.uc.UploadDanceByURL(r.Context(), req.URL)
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

	if user := getUserFromContext(r.Context()); user != nil {
		_ = u.uc.AddToHistory(r.Context(), user.ID, danceResult.DanceID, req.URL)
	}

	response := models.LoadDanceResponse{
		DanceID:             danceResult.DanceID,
		FullGlbKey:          danceResult.FullGlbKey,
		GlbKeys:             danceResult.GlbKeys,
		SegmentsKey:         danceResult.SegmentsKey,
		NumFrames:           int(danceResult.NumFrames),
		NumSegments:         int(danceResult.NumSegments),
		DurationSec:         danceResult.DurationSec,
		NumSegmentsRendered: int(danceResult.NumSegmentsRendered),
		VideoPath:           danceResult.VideoPath,
	}
	response.Sanitize()

	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetDanceByID godoc
// @Summary      Получить информацию о танце по ID
// @Description  Возвращает метаданные обработанного танца
// @Tags         dances
// @Security     OptionalAuth
// @Param        id   path      string  true  "ID танца"
// @Success      200  {object}  models.LoadDanceResponse
// @Failure      400  {string}  string  "Не указан ID танца"
// @Failure      404  {string}  string  "Танец не найден"
// @Failure      500  {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/dance/{id} [get]
func (u *UserHandler) GetDanceByID(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	vars := mux.Vars(r)
	danceID := vars["id"]
	if danceID == "" {
		log.LogHandlerError(logger, errors.New("dance id is required"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	danceResult, err := u.uc.GetDanceByID(r.Context(), danceID)
	if err != nil {
		switch err {
		case users.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	response := models.LoadDanceResponse{
		DanceID:             danceResult.DanceID,
		FullGlbKey:          danceResult.FullGlbKey,
		GlbKeys:             danceResult.GlbKeys,
		SegmentsKey:         danceResult.SegmentsKey,
		NumFrames:           int(danceResult.NumFrames),
		NumSegments:         int(danceResult.NumSegments),
		DurationSec:         danceResult.DurationSec,
		NumSegmentsRendered: int(danceResult.NumSegmentsRendered),
		VideoPath:           danceResult.VideoPath,
	}
	response.Sanitize()

	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetMainPage godoc
// @Summary      Получить список танцев для главной страницы
// @Description  Возвращает массив танцев (возможно, популярных или недавних)
// @Tags         dances
// @Security     OptionalAuth
// @Produce      json
// @Success      200  {object}  models.MainPageResponse
// @Failure      500  {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/main_page [get]
func (u *UserHandler) GetMainPage(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	videos, err := u.uc.GetMainPage(r.Context())
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

// GetSegmentDescription godoc
// @Summary      Получить текстовое описание сегмента танца
// @Description  Возвращает описание для указанного сегмента танца
// @Tags         dances
// @Security     OptionalAuth
// @Param        dance_id     path      string  true  "ID танца"
// @Param        segment_idx  path      int     true  "Индекс сегмента (начиная с 0)"
// @Success      200          {object}  models.SegmentDescriptionResponse
// @Failure      400          {string}  string  "Неверные параметры запроса"
// @Failure      404          {string}  string  "Танец или сегмент не найден"
// @Failure      500          {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/dance/{dance_id}/segment/{segment_idx} [get]
func (u *UserHandler) GetSegmentDescription(w http.ResponseWriter, r *http.Request) {
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

	result, err := u.uc.GetSegmentDescription(r.Context(), danceID, segmentIdx)
	if err != nil {
		switch err {
		case users.ErrorNotFound:
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

// OptionalAuthMiddleware godoc
// @Summary      Middleware опциональной аутентификации
// @Description  Пытается извлечь пользователя из JWT-куки, при успехе добавляет в контекст.
//               Если токена нет или он невалиден — запрос продолжается без пользователя.
// @Tags         internal
// @Security     OptionalAuth
func (h *UserHandler) OptionalAuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        cookie, err := r.Cookie(CookieName)
        if err == nil && cookie.Value != "" {
            user, err := h.client.ValidateAndGetUser(r.Context(), &gen.ValidateAndGetUserRequest{Token: cookie.Value})
            if err == nil {
                neededUser := models.User{
                    ID: uuid.FromStringOrNil(user.ID),
                }
                ctx := context.WithValue(r.Context(), users.UserKey, neededUser)
                next.ServeHTTP(w, r.WithContext(ctx))
                return
            }
        }
        next.ServeHTTP(w, r)
    })
}

// GetSearchHistory godoc
// @Summary      История поиска пользователя
// @Description  Возвращает список ранее загруженных или просмотренных танцев текущего пользователя
// @Tags         users
// @Security     ApiKeyAuth
// @Produce      json
// @Success      200  {array}   models.SearchHistoryItem
// @Failure      401  {string}  string  "Пользователь не авторизован"
// @Failure      500  {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/history [get]
func (h *UserHandler) GetSearchHistory(w http.ResponseWriter, r *http.Request) {
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

func getUserFromContext(ctx context.Context) *models.User {
	user, ok := ctx.Value(users.UserKey).(models.User)
	if !ok {
		return nil
	}
	return &user
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

// DeleteFromHistory godoc
// @Summary      Удалить запись из истории поиска
// @Description  Удаляет запись истории по её ID, принадлежащую текущему пользователю
// @Tags         users
// @Security     ApiKeyAuth
// @Param        history_id  path      string  true  "UUID записи истории"
// @Success      204         "Запись успешно удалена"
// @Failure      400         {string}  string  "Неверный ID записи"
// @Failure      401         {string}  string  "Пользователь не авторизован"
// @Failure      404         {string}  string  "Запись не найдена"
// @Failure      500         {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/history/{history_id} [delete]
func (h *UserHandler) DeleteFromHistory(w http.ResponseWriter, r *http.Request) {
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

// UpdateHistoryName godoc
// @Summary      Обновить название записи в истории
// @Description  Изменяет пользовательское название для указанной записи истории
// @Tags         users
// @Security     ApiKeyAuth
// @Accept       json
// @Param        history_id  path      string                         true  "UUID записи истории"
// @Param        request     body      models.UpdateHistoryNameInput true  "Новое название"
// @Success      204         "Название успешно обновлено"
// @Failure      400         {string}  string  "Неверный ID записи или тело запроса"
// @Failure      401         {string}  string  "Пользователь не авторизован"
// @Failure      404         {string}  string  "Запись не найдена"
// @Failure      500         {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/history/{history_id} [put]
func (h *UserHandler) UpdateHistoryName(w http.ResponseWriter, r *http.Request) {
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