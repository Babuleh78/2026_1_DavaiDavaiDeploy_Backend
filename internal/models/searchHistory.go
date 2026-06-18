package models

import (
	uuid "github.com/satori/go.uuid"
	"time"
)

type SearchHistoryItem struct {
	ID         uuid.UUID `json:"id"`
	UserID     uuid.UUID `json:"user_id"`
	DanceID    string    `json:"dance_id"`
	Name       string    `json:"name"`
	DanceTitle string    `json:"dance_title"`
	SourceURL  string    `json:"source_url"`
	CreatedAt  time.Time `json:"created_at"`
	Score      *float64  `json:"score"`
}
