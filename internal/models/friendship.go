package models

import (
	"time"

	uuid "github.com/satori/go.uuid"
)

type Friend struct {
	UserID     uuid.UUID `json:"user_id"`
	Login      string    `json:"login"`
	Avatar     string    `json:"avatar"`
	FriendedAt time.Time `json:"friended_at"`
}

type FriendshipStatus struct {
	Status       string `json:"status"`
	IsSender     bool   `json:"is_sender"`
	FriendshipID int64  `json:"friendship_id,omitempty"`
}
