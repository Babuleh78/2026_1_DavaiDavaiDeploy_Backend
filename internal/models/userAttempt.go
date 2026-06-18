package models

import "time"

type UserAttemptItem struct {
	AttemptID    string    `json:"attempt_id"`
	DanceID      string    `json:"dance_id"`
	DanceTitle   string    `json:"dance_title"`
	Score        float64   `json:"score"`
	CreatedAt    time.Time `json:"created_at"`
	IsSaved      bool      `json:"is_saved"`
	IsOpen       bool      `json:"is_open"`
	UserName     string    `json:"user_name"`
	Rank         *int      `json:"rank,omitempty"`
	TotalDancers *int      `json:"total_dancers,omitempty"`
}

type DanceProgressEntry struct {
	AttemptID string    `json:"attempt_id"`
	Score     float64   `json:"score"`
	CreatedAt time.Time `json:"created_at"`
}
