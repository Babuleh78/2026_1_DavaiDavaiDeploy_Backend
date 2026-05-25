package models

import uuid "github.com/satori/go.uuid"

type LeaderboardEntry struct {
	Rank   int       `json:"rank"`
	Login  string    `json:"login"`
	Score  float64   `json:"score"`
	IsMe   bool      `json:"is_me"`
	UserID uuid.UUID `json:"user_id"`
	Avatar string    `json:"avatar"`
}

type LeaderboardResponse struct {
	Top       []LeaderboardEntry  `json:"top"`
	UserEntry *LeaderboardEntry   `json:"user_entry,omitempty"`
}
