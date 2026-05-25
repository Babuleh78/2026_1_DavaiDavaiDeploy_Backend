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
	"strings"
	"time"

	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	CookieName       = "DDFilmsJWT"
	CSRFCookieName   = "DDFilmsCSRF"
	DeviceIDCookie   = "DDDanceDeviceID"
	deviceIDMaxAge   = 60 * 60 * 24 * 365 // 1 год
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
	neededUser, ok := r.Context().Value(users.UserKey).(models.User)

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
		UserID:      neededUser.ID.String()})

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

// UpdateProfile godoc
// @Summary      Обновить профиль (логин и/или аватар)
// @Description  Принимает multipart/form-data. Поля login (string, опц.) и avatar (file, опц.). Если меняется login — выдаётся новый JWT.
// @Tags         users
// @Security     ApiKeyAuth
// @Accept       multipart/form-data
// @Produce      json
// @Param        login   formData  string  false  "Новый логин (6-15 символов)"
// @Param        avatar  formData  file    false  "Файл аватара (jpeg/png/webp, до 5МБ)"
// @Success      200     {object}  models.User
// @Failure      400     {string}  string  "Неверные данные или логин уже занят"
// @Failure      401     {string}  string  "Пользователь не авторизован"
// @Failure      413     {string}  string  "Файл слишком большой"
// @Failure      500     {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/profile [put]
func (u *UserHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
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

		avatarBuffer, err = io.ReadAll(f)
		if err != nil {
			log.LogHandlerError(logger, err, http.StatusBadRequest)
			helpers.WriteError(w, http.StatusBadRequest)
			return
		}
		avatarContentType = fileHeader.Header.Get("Content-Type")
	}

	if newLogin == nil && len(avatarBuffer) == 0 {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	updated, newToken, err := u.uc.UpdateProfile(r.Context(), neededUser.ID, newLogin, avatarBuffer, avatarContentType)
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
			Name:     CookieName,
			Value:    newToken,
			HttpOnly: true,
			Secure:   u.cookieSecure,
			SameSite: u.cookieSamesite,
			Expires:  time.Now().Add(12 * time.Hour),
			Path:     "/",
		})
	}

	updated.Sanitize()
	helpers.WriteJSON(w, updated)
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

	var uploaderUserID, uploaderLogin string
	if user := getUserFromContext(r.Context()); user != nil {
		uploaderUserID = user.ID.String()
		uploaderLogin = user.Login
	}

	danceResult, err := u.uc.UploadDance(r.Context(), buffer, "video/mp4", uploaderUserID, uploaderLogin)
	if err != nil {
		switch err {
		case users.ErrorModerationPending:
			writeModerationPending(w, danceResult)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(models.AsyncEnqueueResult{
		TaskID:  danceResult.TaskID,
		DanceID: danceResult.DanceID,
		Status:  "queued",
	})
	log.LogHandlerInfo(logger, "queued", http.StatusAccepted)
}

// writeModerationPending отвечает 202 с кодом MODERATION_PENDING и причиной —
// фронт показывает пользователю, что видео не прошло модерацию, и почему.
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

	var uploaderUserID, uploaderLogin string
	if user := getUserFromContext(r.Context()); user != nil {
		uploaderUserID = user.ID.String()
		uploaderLogin = user.Login
	}

	danceResult, err := u.uc.UploadDanceByURL(r.Context(), req.URL, uploaderUserID, uploaderLogin)
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(models.AsyncEnqueueResult{
		TaskID:  danceResult.TaskID,
		DanceID: danceResult.DanceID,
		Status:  "queued",
	})
	log.LogHandlerInfo(logger, "queued", http.StatusAccepted)
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

	danceID := mux.Vars(r)["id"]
	if danceID == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	// Получаем userID опционально
	var userID *uuid.UUID
	if user := getUserFromContext(r.Context()); user != nil {
		userID = &user.ID
	}

	result, err := u.uc.GetDanceByID(r.Context(), danceID, userID) // передаём userID
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
		DanceID:             result.DanceID,
		FullGlbKey:          result.FullGlbKey,
		GlbKeys:             result.GlbKeys,
		SegmentsKey:         result.SegmentsKey,
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
	}

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
//
//	Если токена нет или он невалиден — запрос продолжается без пользователя.
//
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

	// -r 30 -vsync cfr — принудительно делаем выход 30 fps CFR.
	// Это критично для синхрона скелета и видео: ML рассчитывает t = frame_idx/fps,
	// и если на входе VFR-запись (webm из MediaRecorder), timestamp каждого кадра
	// «плавает» и не совпадает с frame_idx/fps. Форсированный CFR убирает дрейф.
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

// ToggleLike godoc
// @Summary      Поставить или снять лайк с танца
// @Description  Если лайк уже стоит — снимает его, если нет — ставит. Один пользователь может поставить только один лайк.
// @Tags         dances
// @Security     ApiKeyAuth
// @Param        id   path      string  true  "ID танца"
// @Success      200  {object}  models.LikeResponse
// @Failure      400  {string}  string  "Не указан ID танца"
// @Failure      401  {string}  string  "Пользователь не авторизован"
// @Failure      500  {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/dance/{id}/like [post]
func (h *UserHandler) ToggleLike(w http.ResponseWriter, r *http.Request) {
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

// CompareDance godoc
// @Summary      Сравнить танец пользователя с оригиналом
// @Description  Загружает видео пользователя и сравнивает с указанным танцем по сегменту.
//
//	segment_idx = -1 означает сравнение всего видео целиком.
//
// @Tags         dances
// @Accept       json
// @Produce      json
// @Param        input  body      models.DanceCompareRequest   true  "Параметры сравнения"
// @Success      200    {object}  models.DanceCompareResponse
// @Failure      400    {string}  string  "Неверные параметры запроса"
// @Failure      500    {string}  string  "Внутренняя ошибка сервера"
// @Router       /users/dance/compare [post]
func (u *UserHandler) CompareDance(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	var input models.DanceCompareRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		log.LogHandlerError(logger, err, http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if input.VideoKey == "" || input.DanceID == "" {
		log.LogHandlerError(logger, errors.New("video_key and dance_id are required"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if input.SegmentIdx == 0 {
		input.SegmentIdx = -1
	}

	result, err := u.uc.CompareDance(r.Context(), input.VideoKey, input.DanceID, input.SegmentIdx)
	if err != nil {
		switch err {
		case users.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}


// GetUserLikedDances godoc
// @Summary Get all liked dances for current user
// @Tags users
// @Produce json
// @Success 200 {object} models.UserLikeDancesResponse
// @Failure 401
// @Failure 500
// @Router /users/likes [get]
func(u *UserHandler) GetUserLikedDances(w http.ResponseWriter, r *http.Request){
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	userID, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		log.LogHandlerError(logger, errors.New("user unauthorized"), http.StatusUnauthorized)
        helpers.WriteError(w, http.StatusUnauthorized)
        return
	}

	likes, err := u.uc.GetUserLikedDances(r.Context(), userID.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	response := models.UserLikedDancesResponse{
		Likes: likes,
	}
	if response.Likes == nil {
		response.Likes = []models.DanceLike{}
	}

	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}


// TrimAndLoadDance godoc
// @Summary Trim video and load dance
// @Tags users
// @Accept multipart/form-data
// @Produce json
// @Param dance formData file true "Dance video file"
// @Param start_sec formData number true "Start time in seconds"
// @Param end_sec formData number true "End time in seconds"
// @Success 200 {object} models.LoadDanceResponse
// @Failure 400
// @Failure 500
// @Router /users/load/trim [post]
func (u *UserHandler) TrimAndLoadDance(w http.ResponseWriter, r *http.Request) {
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

    danceResult, err := u.uc.UploadDance(r.Context(), buffer, "video/mp4", uploaderUserID, uploaderLogin)
    if err != nil {
        switch err {
        case users.ErrorModerationPending:
            writeModerationPending(w, danceResult)
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

    // -r 30 -vsync cfr форсирует выход 30 fps CFR — иначе для VFR-записей
    // (типичный webm из MediaRecorder) ML расходится с реальным временем кадров.
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



// SaveRating godoc
// @Summary Save dance rating
// @Tags users
// @Accept json
// @Produce json
// @Param input body models.SaveRatingInput true "Rating data"
// @Success 200 {object} models.RatingResponse
// @Failure 400
// @Failure 401
// @Failure 500
// @Router /users/dance/rate [post]
func (u *UserHandler) SaveRating(w http.ResponseWriter, r *http.Request) {
    logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

    userID, ok := r.Context().Value(users.UserKey).(models.User)
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

    result, err := u.uc.SaveRating(r.Context(), userID.ID, req)
    if err != nil {
        helpers.WriteError(w, http.StatusInternalServerError)
        return
    }

    helpers.WriteJSON(w, result)
    log.LogHandlerInfo(logger, "success", http.StatusOK)
}



// CompareDanceWithFile godoc
// @Summary Compare user dance with reference
// @Tags users
// @Accept multipart/form-data
// @Produce json
// @Param dance formData file true "User dance video"
// @Param reference_dance_id formData string true "Reference dance ID"
// @Success 200 {object} models.CompareResult
// @Failure 400
// @Failure 500
// @Router /users/dance/compare-upload [post]
func (u *UserHandler) CompareDanceWithFile(w http.ResponseWriter, r *http.Request) {
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
        log.LogHandlerError(logger, errors.New("reference_dance_id is required"), http.StatusBadRequest)
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

    // Опциональная обрезка: фронт-редактор передаёт start_sec/end_sec, чтобы
    // пользователь мог убрать «подход к камере» и «отход от неё» до отправки
    // на сравнение. Если параметры валидны (end > start) — используем
    // trimAndConvertVideo, который сам перекодирует в H.264. Иначе обычный
    // convertToH264 на полный буфер.
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
            buffer, err = trimAndConvertVideo(buffer, startSec, endSec)
            if err != nil {
                log.LogHandlerError(logger, fmt.Errorf("failed to trim: %w", err), http.StatusBadRequest)
                helpers.WriteError(w, http.StatusBadRequest)
                return
            }
        } else {
            buffer, err = convertToH264(buffer)
        }
    } else {
        buffer, err = convertToH264(buffer)
    }
    if err != nil {
        log.LogHandlerError(logger, fmt.Errorf("failed to convert: %w", err), http.StatusBadRequest)
        helpers.WriteError(w, http.StatusBadRequest)
        return
    }

    result, err := u.uc.CompareDanceFromBuffer(r.Context(), buffer, "video/mp4", referenceDanceID, userID)
    if err != nil {
        logger.Error("CompareDanceFromBuffer failed", "error", err)
        switch err {
        case users.ErrorNotFound:
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

// GetTaskStatus godoc
// @Summary      Статус асинхронной задачи обработки/сравнения
// @Tags         users
// @Produce      json
// @Param        task_id       path   string true  "ID задачи Celery"
// @Param        type          query  string true  "Тип задачи: upload или compare"
// @Param        dance_id      query  string false "ID танца"
// @Param        user_dance_id query  string false "ID попытки пользователя (для compare)"
// @Param        video_key     query  string false "S3-ключ видео пользователя (для compare)"
// @Success      200  {object}  models.TaskStatusResponse
// @Router       /users/task/{task_id}/status [get]
func (u *UserHandler) GetTaskStatus(w http.ResponseWriter, r *http.Request) {
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

    result, err := u.uc.GetTaskStatus(r.Context(), taskID, taskType, danceID, userDanceID, videoKey, uploaderUserID, userID)
    if err != nil {
        logger.Error("GetTaskStatus failed", "error", err)
        helpers.WriteError(w, http.StatusInternalServerError)
        return
    }

    helpers.WriteJSON(w, result)
    log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetDanceTimeline godoc
// @Summary      Получить покадровый таймлайн сравнения танца
// @Description  Возвращает тело timeline.json из S3 для указанного пользователя и танца
// @Tags         dances
// @Param        dance_id  path      string  true  "ID танца"
// @Param        user_id   path      string  true  "ID пользователя"
// @Produce      json
// @Success      200  {object}  object
// @Failure      400  {string}  string  "Неверные параметры"
// @Failure      404  {string}  string  "Файл не найден"
// @Failure      500  {string}  string  "Внутренняя ошибка"
// @Router       /dances/{dance_id}/users/{user_id}/timeline [get]
func (u *UserHandler) GetDanceTimeline(w http.ResponseWriter, r *http.Request) {
    logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

    vars := mux.Vars(r)
    danceID := vars["dance_id"]
    userID := vars["user_id"]

    if danceID == "" || userID == "" {
        helpers.WriteError(w, http.StatusBadRequest)
        return
    }

    data, err := u.uc.GetDanceTimeline(r.Context(), danceID, userID)
    if err != nil {
        switch err {
        case users.ErrorNotFound:
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

// GetDanceKeyframes godoc
// @Summary      Получить ключевые кадры эталонного танца
// @Description  Возвращает тело keyframes.json из S3 для указанного танца. 404 если файл ещё не создан.
// @Tags         dances
// @Param        dance_id  path      string  true  "ID танца"
// @Produce      json
// @Success      200  {object}  object
// @Failure      400  {string}  string  "Неверные параметры"
// @Failure      404  {string}  string  "Файл не найден"
// @Failure      500  {string}  string  "Внутренняя ошибка"
// @Router       /dances/{dance_id}/keyframes [get]
func (u *UserHandler) GetDanceKeyframes(w http.ResponseWriter, r *http.Request) {
    logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

    danceID := mux.Vars(r)["dance_id"]
    if danceID == "" {
        helpers.WriteError(w, http.StatusBadRequest)
        return
    }

    data, err := u.uc.GetDanceKeyframes(r.Context(), danceID)
    if err != nil {
        switch err {
        case users.ErrorNotFound:
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

func AdminTokenMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		adminToken := os.Getenv("ADMIN_TOKEN")
		if adminToken == "" {
			helpers.WriteError(w, http.StatusServiceUnavailable)
			return
		}
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			helpers.WriteError(w, http.StatusUnauthorized)
			return
		}
		if strings.TrimPrefix(authHeader, "Bearer ") != adminToken {
			helpers.WriteError(w, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// UpdateDanceStatus godoc
// @Summary Update dance moderation status (admin only)
// @Tags admin
// @Accept json
// @Param id path string true "Dance ID"
// @Param request body models.DanceStatusInput true "New status"
// @Success 204 "Status updated"
// @Failure 400
// @Failure 401
// @Failure 500
// @Router /admin/dances/{id}/status [patch]
func (u *UserHandler) UpdateDanceStatus(w http.ResponseWriter, r *http.Request) {
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

	if err := u.uc.UpdateDanceStatus(r.Context(), id, input.Status); err != nil {
		switch err {
		case users.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

// GetDanceCatalog godoc
// @Summary Get dance catalog
// @Tags dances
// @Produce json
// @Param sort   query string false "Sort: popular|newest|new|easiest|easy|hardest"
// @Param search query string false "Search by title"
// @Param page   query int    false "Page number (default 1)"
// @Param limit  query int    false "Items per page (max 48, default 12)"
// @Success 200 {object} models.DanceCatalogResponse
// @Failure 500
// @Router /dances [get]
func (u *UserHandler) GetDanceCatalog(w http.ResponseWriter, r *http.Request) {
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
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}

	result, err := u.uc.GetDanceCatalog(r.Context(), sort, search, page, limit)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetDanceTrending godoc
// @Summary Get trending dances (top 6 by attempts in last 7 days)
// @Tags dances
// @Produce json
// @Success 200 {object} models.TrendingResponse
// @Failure 500
// @Router /dances/trending [get]
func (u *UserHandler) GetDanceTrending(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	result, err := u.uc.GetDanceTrending(r.Context())
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetDanceStatsHandler godoc
// @Summary Get aggregated stats for a dance
// @Tags dances
// @Produce json
// @Param id path string true "Dance ID"
// @Success 200 {object} models.DanceStats
// @Failure 400
// @Failure 500
// @Router /dances/{id}/stats [get]
func (u *UserHandler) GetDanceStatsHandler(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	id := mux.Vars(r)["id"]
	if id == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	stats, err := u.uc.GetDanceStats(r.Context(), id)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, stats)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetDanceModerationStatus godoc
// @Summary      Статус танца и причина модерации (публичный)
// @Description  Лёгкий эндпоинт: позволяет анонимному пользователю отследить
//
//	судьбу своей загрузки (processing/pending/private/published/rejected).
//
// @Tags         dances
// @Produce      json
// @Param        id   path      string  true  "ID танца"
// @Success      200  {object}  map[string]string
// @Failure      404  {string}  string  "Танец не найден"
// @Router       /dances/{id}/status [get]
func (u *UserHandler) GetDanceModerationStatus(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	id := mux.Vars(r)["id"]
	if id == "" {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	status, reason, err := u.uc.GetDanceModerationStatus(r.Context(), id)
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

// GetLeaderboard godoc
// @Summary Get dance leaderboard (top 10 + current user rank)
// @Tags dances
// @Produce json
// @Param id path string true "Dance ID"
// @Success 200 {object} models.LeaderboardResponse
// @Failure 400
// @Failure 500
// @Router /dances/{id}/leaderboard [get]
func (u *UserHandler) GetLeaderboard(w http.ResponseWriter, r *http.Request) {
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

	result, err := u.uc.GetLeaderboard(r.Context(), danceID, userID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// SaveAttempt godoc
// @Summary      Сохранить текущую попытку в профиль («Мои танцы»)
// @Description  Берёт лучший результат пользователя по этому танцу и публикует его в профиле.
//
//	Тело {include_video: bool} (по умолчанию false) — оставить ли пользовательское видео
//	в S3. Если false — видео удаляется немедленно; если true — остаётся.
//
// @Tags         users
// @Security     ApiKeyAuth
// @Accept       json
// @Param        dance_id  path  string                  true   "ID эталонного танца"
// @Param        body      body  models.SaveAttemptInput false  "Параметры сохранения"
// @Success      204  "Сохранено"
// @Failure      400  {string}  string  "Не указан dance_id"
// @Failure      401  {string}  string  "Не авторизован"
// @Failure      404  {string}  string  "Нет попыток по этому танцу"
// @Failure      500  {string}  string  "Внутренняя ошибка"
// @Router       /users/dance/{user_dance_id}/save [post]
func (u *UserHandler) SaveAttempt(w http.ResponseWriter, r *http.Request) {
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

	// Тело обязательно (нужен dance_id): include_video по умолчанию false
	// (приватнее — видео удаляется, остаётся только GLB и скелет).
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

	if err := u.uc.SaveAttempt(r.Context(), user.ID, attemptID, input.DanceID, input.IncludeVideo, input.Score, input.UserName, input.IsPrivate); err != nil {
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

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

// UnsaveAttempt godoc
// @Summary      Убрать попытку из «Моих танцев»
// @Tags         users
// @Security     ApiKeyAuth
// @Param        dance_id  path  string  true  "ID эталонного танца"
// @Success      204
// @Failure      400
// @Failure      401
// @Failure      500
// @Router       /users/dance/{user_dance_id}/save [delete]
func (u *UserHandler) UnsaveAttempt(w http.ResponseWriter, r *http.Request) {
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

	if err := u.uc.UnsaveAttempt(r.Context(), user.ID, attemptID); err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

// GetSavedAttempts godoc
// @Summary      Получить «Мои танцы» текущего пользователя
// @Tags         users
// @Security     ApiKeyAuth
// @Produce      json
// @Success      200  {array}  models.SavedAttemptItem
// @Failure      401
// @Failure      500
// @Router       /users/saved-dances [get]
func (u *UserHandler) GetSavedAttempts(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	items, err := u.uc.GetSavedAttempts(r.Context(), user.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, items)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetUserAttempts godoc
// @Summary      Все попытки текущего пользователя («Мои попытки»)
// @Tags         users
// @Security     ApiKeyAuth
// @Produce      json
// @Success      200  {array}  models.UserAttemptItem
// @Failure      401
// @Failure      500
// @Router       /users/attempts [get]
func (u *UserHandler) GetUserAttempts(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	items, err := u.uc.GetUserAttempts(r.Context(), user.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, items)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetPublicProfile godoc
// @Summary      Публичный профиль пользователя
// @Description  Возвращает данные юзера, его опубликованные «Мои танцы» и личный топ-3.
// @Tags         users
// @Security     OptionalAuth
// @Param        id  path  string  true  "UUID пользователя"
// @Produce      json
// @Success      200  {object}  models.PublicProfileResponse
// @Failure      400
// @Failure      404
// @Failure      500
// @Router       /users/{id}/profile [get]
func (u *UserHandler) GetPublicProfile(w http.ResponseWriter, r *http.Request) {
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

	profile, err := u.uc.GetPublicProfile(r.Context(), profileID, viewerID)
	if err != nil {
		switch err {
		case users.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, profile)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// ensureDeviceID возвращает значение DeviceIDCookie. Если куки нет —
// генерирует новый UUID и ставит её на 1 год. Используется для дедупа
// просмотров уроков анонимными юзерами.
func (u *UserHandler) ensureDeviceID(w http.ResponseWriter, r *http.Request) string {
	if c, err := r.Cookie(DeviceIDCookie); err == nil && c.Value != "" {
		return c.Value
	}
	id := uuid.NewV4().String()
	http.SetCookie(w, &http.Cookie{
		Name:     DeviceIDCookie,
		Value:    id,
		HttpOnly: true,
		Secure:   u.cookieSecure,
		SameSite: u.cookieSamesite,
		MaxAge:   deviceIDMaxAge,
		Path:     "/",
	})
	return id
}

// RecordDanceView godoc
// @Summary      Засчитать просмотр урока
// @Description  Идемпотентно по (dance_id, viewer_id). Для авторизованных юзеров
//
//	viewer_id = user.ID; для анонимов — UUID из cookie DDDanceDeviceID
//	(ставится автоматически при первом запросе).
//
// @Tags         dances
// @Security     OptionalAuth
// @Param        id  path  string  true  "ID танца"
// @Success      204  "Просмотр учтён (или уже был засчитан раньше)"
// @Failure      400  {string}  string  "Не указан ID танца"
// @Failure      500  {string}  string  "Внутренняя ошибка"
// @Router       /dances/{id}/view [post]
func (u *UserHandler) RecordDanceView(w http.ResponseWriter, r *http.Request) {
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
		viewerID = u.ensureDeviceID(w, r)
	}

	if err := u.uc.RecordDanceView(r.Context(), danceID, viewerID); err != nil {
		switch err {
		case users.ErrorBadRequest:
			helpers.WriteError(w, http.StatusBadRequest)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
	log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

// GetRating godoc
// @Summary Get aggregated dance rating
// @Tags users
// @Produce json
// @Param dance_id query string true "Dance ID"
// @Success 200 {object} models.RatingResponse
// @Failure 400
// @Failure 500
// @Router /users/dance/rate [get]
func (u *UserHandler) GetRating(w http.ResponseWriter, r *http.Request) {
    logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

    danceID := r.URL.Query().Get("dance_id")
    if danceID == "" {
        log.LogHandlerError(logger, errors.New("dance_id is required"), http.StatusBadRequest)
        helpers.WriteError(w, http.StatusBadRequest)
        return
    }

    rating, err := u.uc.GetRating(r.Context(), danceID)
    if err != nil {
        switch err {
        case users.ErrorBadRequest:
            helpers.WriteError(w, http.StatusBadRequest)
        default:
            helpers.WriteError(w, http.StatusInternalServerError)
        }
        return
    }

    helpers.WriteJSON(w, rating)
    log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetNotifications godoc
// @Summary      Получить список уведомлений
// @Description  Возвращает до 50 последних уведомлений + счётчик непрочитанных.
// @Tags         notifications
// @Security     ApiKeyAuth
// @Produce      json
// @Success      200  {object}  models.NotificationsResponse
// @Failure      401
// @Failure      500
// @Router       /notifications [get]
func (u *UserHandler) GetNotifications(w http.ResponseWriter, r *http.Request) {
    logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

    user, ok := r.Context().Value(users.UserKey).(models.User)
    if !ok {
        helpers.WriteError(w, http.StatusUnauthorized)
        return
    }

    resp, err := u.uc.GetNotifications(r.Context(), user.ID)
    if err != nil {
        helpers.WriteError(w, http.StatusInternalServerError)
        return
    }

    helpers.WriteJSON(w, resp)
    log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// MarkNotificationRead godoc
// @Summary      Пометить уведомление прочитанным
// @Tags         notifications
// @Security     ApiKeyAuth
// @Param        id  path  int  true  "Notification ID"
// @Success      204
// @Failure      400
// @Failure      401
// @Failure      500
// @Router       /notifications/{id}/read [post]
func (u *UserHandler) MarkNotificationRead(w http.ResponseWriter, r *http.Request) {
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

    if err := u.uc.MarkNotificationRead(r.Context(), id, user.ID); err != nil {
        helpers.WriteError(w, http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusNoContent)
    log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

// MarkAllNotificationsRead godoc
// @Summary      Пометить все уведомления прочитанными
// @Tags         notifications
// @Security     ApiKeyAuth
// @Success      204
// @Failure      401
// @Failure      500
// @Router       /notifications/read-all [post]
func (u *UserHandler) MarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
    logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

    user, ok := r.Context().Value(users.UserKey).(models.User)
    if !ok {
        helpers.WriteError(w, http.StatusUnauthorized)
        return
    }

    if err := u.uc.MarkAllNotificationsRead(r.Context(), user.ID); err != nil {
        helpers.WriteError(w, http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusNoContent)
    log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

// ClaimUploads godoc
// @Summary      Закрепить за собой анонимные загрузки
// @Description  После регистрации фронт отправляет dance_id, накопленные в localStorage
// @Description  во время анонимных загрузок. Каждый dance_id привязывается к user.
// @Tags         notifications
// @Security     ApiKeyAuth
// @Accept       json
// @Param        request  body  models.ClaimUploadsInput  true  "Список dance_id"
// @Success      204
// @Failure      400
// @Failure      401
// @Failure      500
// @Router       /uploads/claim [post]
func (u *UserHandler) ClaimUploads(w http.ResponseWriter, r *http.Request) {
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

    if err := u.uc.ClaimDanceUploads(r.Context(), user.ID, input.DanceIDs); err != nil {
        helpers.WriteError(w, http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusNoContent)
    log.LogHandlerInfo(logger, "success", http.StatusNoContent)
}

// GetCompareResult godoc
// @Summary      Получить сохранённый результат сравнения
// @Description  Возвращает CompareResult для ранее выполненного сравнения. user_dance_id = dance_id из saved_attempts.
// @Tags         users
// @Security     ApiKeyAuth
// @Param        user_dance_id  path  string  true  "ID эталонного танца"
// @Produce      json
// @Success      200  {object}  models.CompareResult
// @Failure      401
// @Failure      404
// @Failure      500
// @Router       /users/dance/{user_dance_id}/result [get]
func (u *UserHandler) GetCompareResult(w http.ResponseWriter, r *http.Request) {
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

	result, err := u.uc.GetCompareResult(r.Context(), user.ID, userDanceID)
	if err != nil {
		switch err {
		case users.ErrorNotFound:
			helpers.WriteError(w, http.StatusNotFound)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, result)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (u *UserHandler) SendFriendRequest(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	sender, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	receiverIDStr := mux.Vars(r)["id"]
	receiverID, err := uuid.FromString(receiverIDStr)
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := u.uc.SendFriendRequest(r.Context(), sender.ID, receiverID); err != nil {
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

func (u *UserHandler) RespondFriendRequest(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	friendshipIDStr := mux.Vars(r)["id"]
	friendshipID, err := strconv.ParseInt(friendshipIDStr, 10, 64)
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

	if err := u.uc.RespondFriendRequest(r.Context(), user.ID, friendshipID, body.Accept); err != nil {
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

func (u *UserHandler) GetFriends(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	friends, err := u.uc.GetFriends(r.Context(), user.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, friends)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (u *UserHandler) GetPublicFriends(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	profileUserIDStr := mux.Vars(r)["id"]
	profileUserID, err := uuid.FromString(profileUserIDStr)
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	friends, err := u.uc.GetFriends(r.Context(), profileUserID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, friends)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (u *UserHandler) RemoveFriend(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	friendIDStr := mux.Vars(r)["friend_id"]
	friendID, err := uuid.FromString(friendIDStr)
	if err != nil {
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}

	if err := u.uc.RemoveFriend(r.Context(), user.ID, friendID); err != nil {
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

func (u *UserHandler) GetUploadedDances(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

	user, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	dances, err := u.uc.GetUploadedDances(r.Context(), user.ID)
	if err != nil {
		helpers.WriteError(w, http.StatusInternalServerError)
		return
	}

	helpers.WriteJSON(w, dances)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (u *UserHandler) SetDanceName(w http.ResponseWriter, r *http.Request) {
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

	if err := u.uc.SetDanceName(r.Context(), user.ID, danceID, title, body.Difficulty); err != nil {
		switch err {
		case users.ErrorForbidden:
			helpers.WriteError(w, http.StatusForbidden)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	if body.Publish {
		if err := u.uc.PublishDance(r.Context(), user.ID, danceID); err != nil {
			helpers.WriteError(w, http.StatusInternalServerError)
			return
		}
	}

	helpers.WriteJSON(w, map[string]string{"status": "ok"})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// DeleteDance godoc
// @Summary      Удалить свой танец навсегда
// @Description  Жёстко удаляет танец автора (БД + S3); 403 для не-автора.
// @Tags         dances
// @Security     ApiKeyAuth
// @Param        dance_id  path  string  true  "ID танца"
// @Success      200  {object}  map[string]string
// @Failure      400  {string}  string
// @Failure      403  {string}  string
// @Failure      500  {string}  string
// @Router       /users/dance/{dance_id} [delete]
func (u *UserHandler) DeleteDance(w http.ResponseWriter, r *http.Request) {
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

	if err := u.uc.DeleteDance(r.Context(), user.ID, danceID); err != nil {
		switch err {
		case users.ErrorForbidden:
			helpers.WriteError(w, http.StatusForbidden)
		default:
			helpers.WriteError(w, http.StatusInternalServerError)
		}
		return
	}

	helpers.WriteJSON(w, map[string]string{"status": "deleted"})
	log.LogHandlerInfo(logger, "success", http.StatusOK)
}

func (u *UserHandler) PublishDance(w http.ResponseWriter, r *http.Request) {
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

	if err := u.uc.PublishDance(r.Context(), user.ID, danceID); err != nil {
		switch err {
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

func (u *UserHandler) UnpublishDance(w http.ResponseWriter, r *http.Request) {
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

	if err := u.uc.UnpublishDance(r.Context(), user.ID, danceID); err != nil {
		switch err {
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
