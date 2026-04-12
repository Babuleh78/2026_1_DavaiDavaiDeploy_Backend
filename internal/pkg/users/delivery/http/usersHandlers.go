package http

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/auth/delivery/grpc/gen"
	"DDDance/internal/pkg/helpers"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"
	"os/exec"
	"fmt"
	"bytes"
	"strconv"
	
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
	cookieSecure   bool
	cookieSamesite http.SameSite
}

func NewUserHandler(client gen.AuthClient) *UserHandler {
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
		ctx := context.WithValue(r.Context(), users.UserKey, neededUser.ID)

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
		ctx := context.WithValue(r.Context(), users.UserKey, neededUser.ID)

		log.LogHandlerInfo(logger, "success", http.StatusOK)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetUser godoc
// @Summary Get user by ID
// @Tags users
// @Produce json
// @Param        id   path      string  true  "Genre ID"
// @Success 200 {object} models.User
// @Failure 400
// @Failure 404
// @Failure 500
// @Router /users/{id} [get]
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
// @Summary Change user password
// @Tags users
// @Accept json
// @Produce json
// @Param input body models.ChangePasswordInput true "Password data (old_password and new_password are required)"
// @Success 200 {object} models.User
// @Failure 400
// @Failure 401
// @Failure 500
// @Router /users/password [put]
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
// @Summary Load Dance
// @Tags users
// @Accept multipart/form-data
// @Produce json
// @Param dance formData file true "Dance video file (required, max 50MB, formats: mp4, mov)"
// @Success 200 {object} models.LoadDanceResponse
// @Failure 400
// @Failure 413
// @Failure 500
// @Router /users/load [post]
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

	danceResult, err := u.client.LoadDance(r.Context(), &gen.LoadDanceRequest{
		Dance:      buffer,
		FileFormat: "video/mp4",
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

	response := models.LoadDanceResponse{
		DanceID:             danceResult.DanceID,
		FullGlbKey:          danceResult.FullGlbKey,
		GlbKeys:             danceResult.GlbKeys,
		SegmentsKey:         danceResult.SegmentsKey,
		NumFrames:           int(danceResult.NumFrames),
		NumSegments:         int(danceResult.NumSegments),
		DurationSec:         danceResult.DurationSec,
		NumSegmentsRendered: int(danceResult.NumSegmentsRendered),
		VideoPath:			 danceResult.VideoPath,
	}

	response.Sanitize()

	helpers.WriteJSON(w, response)
	log.LogHandlerInfo(logger, "success", http.StatusOK)
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


// LoadDanceByURL godoc
// @Summary Load Dance by URL
// @Tags users
// @Accept json
// @Produce json
// @Param input body models.LoadDanceByURLInput true "Dance URL and dance_id"
// @Success 200 {object} models.LoadDanceResponse
// @Failure 400
// @Failure 500
// @Router /users/loadByURL [post]
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

    danceResult, err := u.client.LoadDanceByURL(r.Context(), &gen.LoadDanceByURLRequest{
        Url:     req.URL,
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

    response := models.LoadDanceResponse{
        DanceID:             danceResult.DanceID,
        FullGlbKey:          danceResult.FullGlbKey,
        GlbKeys:             danceResult.GlbKeys,
        SegmentsKey:         danceResult.SegmentsKey,
        NumFrames:           int(danceResult.NumFrames),
        NumSegments:         int(danceResult.NumSegments),
        DurationSec:         danceResult.DurationSec,
        NumSegmentsRendered: int(danceResult.NumSegmentsRendered),
    }
    response.Sanitize()

    helpers.WriteJSON(w, response)
    log.LogHandlerInfo(logger, "success", http.StatusOK)
}

// GetDanceByID godoc
// @Summary Get dance result by ID
// @Tags users
// @Produce json
// @Param id path string true "Dance ID"
// @Success 200 {object} models.LoadDanceResponse
// @Failure 400
// @Failure 404
// @Failure 500
// @Router /users/dance/{id} [get]
func (u *UserHandler) GetDanceByID(w http.ResponseWriter, r *http.Request) {
    logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

    vars := mux.Vars(r)
    danceID := vars["id"]
    if danceID == "" {
        log.LogHandlerError(logger, errors.New("dance id is required"), http.StatusBadRequest)
        helpers.WriteError(w, http.StatusBadRequest)
        return
    }

    danceResult, err := u.client.GetDanceByID(r.Context(), &gen.GetDanceByIDRequest{
        DanceId: danceID,
    })
    if err != nil {
        st, _ := status.FromError(err)
        switch st.Code() {
        case codes.NotFound:
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
// @Summary Get main page videos
// @Tags users
// @Produce json
// @Success 200 {object} models.MainPageResponse
// @Failure 500
// @Router /users/main_page [get]
func (u *UserHandler) GetMainPage(w http.ResponseWriter, r *http.Request) {
    logger := log.GetLoggerFromContext(r.Context()).With(slog.String("func", log.GetFuncName()))

    result, err := u.client.GetMainPage(r.Context(), &gen.GetMainPageRequest{})
    if err != nil {
        st, _ := status.FromError(err)
        switch st.Code() {
        default:
            helpers.WriteError(w, http.StatusInternalServerError)
        }
        return
    }

    var videos []models.VideoItem
    for _, v := range result.Videos {
        videos = append(videos, models.VideoItem{
            ID:  v.ID,
            URL: v.URL,
        })
    }

    response := models.MainPageResponse{
        Count:  int(result.Count),
        Videos: videos,
    }

    helpers.WriteJSON(w, response)
    log.LogHandlerInfo(logger, "success", http.StatusOK)
}


// GetSegmentDescription godoc
// @Summary Get segment description by dance ID and segment index
// @Tags users
// @Produce json
// @Param dance_id path string true "Dance ID"
// @Param segment_idx path int true "Segment index"
// @Success 200 {object} models.SegmentDescriptionResponse
// @Failure 400
// @Failure 404
// @Failure 500
// @Router /users/dance/{dance_id}/segment/{segment_idx} [get]
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

    result, err := u.client.GetSegmentDescription(r.Context(), &gen.GetSegmentDescriptionRequest{
        DanceId:    danceID,
        SegmentIdx: int32(segmentIdx),
    })
    if err != nil {
        st, _ := status.FromError(err)
        switch st.Code() {
        case codes.NotFound:
            helpers.WriteError(w, http.StatusNotFound)
        default:
            helpers.WriteError(w, http.StatusInternalServerError)
        }
        return
    }

    response := models.SegmentDescriptionResponse{
        DanceID:     result.DanceId,
        SegmentIdx:  int(result.SegmentIdx),
        Description: result.Description,
    }

    helpers.WriteJSON(w, response)
    log.LogHandlerInfo(logger, "success", http.StatusOK)
}