package models

import (
	"time"

	uuid "github.com/satori/go.uuid"
)

type Friend struct {
	UserID       uuid.UUID `json:"user_id"`
	Login        string    `json:"login"`
	Avatar       string    `json:"avatar"`
	FriendedAt   time.Time `json:"friended_at"`
	ActiveDuelID *string   `json:"active_duel_id,omitempty"`
}

type FriendScore struct {
	Login     string  `json:"friend_login"`
	AvatarURL string  `json:"avatar_url"`
	BestScore float64 `json:"best_score"`
}

type FriendshipStatus struct {
	Status       string `json:"status"`
	IsSender     bool   `json:"is_sender"`
	FriendshipID int64  `json:"friendship_id,omitempty"`
}
