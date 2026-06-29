package http

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/auth/delivery/grpc/gen"
	"DDDance/internal/pkg/comparison"
	"DDDance/internal/pkg/dance"
	"DDDance/internal/pkg/helpers"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	CookieName     = "DDDanceJWT"
	CSRFCookieName = "DDDanceCSRF"
	DeviceIDCookie = "DDDanceDeviceID"
	deviceIDMaxAge = 60 * 60 * 24 * 365 // 1 год
)

type UserHandler struct {
	client         gen.AuthClient
	uc             users.UsersUsecase
	compUC         comparison.ComparisonUsecase
	danceUC        dance.DanceUsecase
	cookieSecure   bool
	cookieSamesite http.SameSite
}

func NewUserHandler(client gen.AuthClient, uc users.UsersUsecase, compUC comparison.ComparisonUsecase, danceUC dance.DanceUsecase, cookieSecure bool, cookieSameSite string) *UserHandler {
	samesite := http.SameSiteLaxMode
	if cookieSameSite == "Strict" {
		samesite = http.SameSiteStrictMode
	}
	return &UserHandler{
		client:         client,
		uc:             uc,
		compUC:         compUC,
		danceUC:        danceUC,
		cookieSecure:   cookieSecure,
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

		if subtle.ConstantTimeCompare([]byte(csrfCookie.Value), []byte(csrfToken)) != 1 {
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

func (u *UserHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))
	neededUser, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		log.LogHandlerError(logger, errors.New("user unauthorized"), http.StatusUnauthorized)
		helpers.WriteError(w, http.StatusUnauthorized)
		return
	}

	var req models.ChangePasswordInput
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.LogHandlerError(logger, errors.New("invalid request"), http.StatusBadRequest)
		helpers.WriteError(w, http.StatusBadRequest)
		return
	}
	req.Sanitize()

	user, err := u.client.ChangePassword(r.Context(), &gen.ChangePasswordRequest{
		OldPassword: req.OldPassword,
		NewPassword: req.NewPassword,
		UserID:      neededUser.ID.String(),
	})
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

func AdminTokenMiddleware(adminToken string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if adminToken == "" {
				helpers.WriteError(w, http.StatusServiceUnavailable)
				return
			}
			authHeader := r.Header.Get("Authorization")
			const prefix = "Bearer "
			if !strings.HasPrefix(authHeader, prefix) {
				helpers.WriteError(w, http.StatusUnauthorized)
				return
			}
			provided := strings.TrimPrefix(authHeader, prefix)
			if subtle.ConstantTimeCompare([]byte(provided), []byte(adminToken)) != 1 {
				helpers.WriteError(w, http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
