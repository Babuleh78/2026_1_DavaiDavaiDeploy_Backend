package events

import (
	"DDDance/internal/models"
	"DDDance/internal/pkg/users"
	"DDDance/internal/pkg/utils/log"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"
)

const maxConnsPerUser = 5

type Handler struct {
	redisAddr string

	mu    sync.Mutex
	conns map[string]int
}

func NewHandler(redisAddr string) *Handler {
	return &Handler{redisAddr: redisAddr, conns: make(map[string]int)}
}

func (h *Handler) acquire(userID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[userID] >= maxConnsPerUser {
		return false
	}
	h.conns[userID]++
	return true
}

func (h *Handler) release(userID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.conns[userID] <= 1 {
		delete(h.conns, userID)
		return
	}
	h.conns[userID]--
}

func (h *Handler) ServeSSE(w http.ResponseWriter, r *http.Request) {
	logger := log.GetLoggerFromContext(r.Context()).With(slog.String("handler", "ServeSSE"))

	u, ok := r.Context().Value(users.UserKey).(models.User)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userID := u.ID.String()

	if !h.acquire(userID) {
		http.Error(w, "too many connections", http.StatusTooManyRequests)
		return
	}
	defer h.release(userID)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	client := redis.NewClient(&redis.Options{Addr: h.redisAddr})
	defer client.Close()

	channel := fmt.Sprintf("channel:user:%s", userID)
	sub := client.Subscribe(r.Context(), channel)
	defer sub.Close()

	ch := sub.Channel()

	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	logger.Info("SSE client connected", "user_id", userID)

	// TODO: close the SSE connection when the JWT expires. ValidateAndGetUser

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-r.Context().Done()
		cancel()
	}()

	for {
		select {
		case <-ctx.Done():
			logger.Info("SSE client disconnected", "user_id", userID)
			return
		case msg, open := <-ch:
			if !open {
				return
			}
			payload := strings.NewReplacer("\r", "", "\n", " ").Replace(msg.Payload)
			fmt.Fprintf(w, "data: %s\n\n", payload)
			flusher.Flush()
		}
	}
}
