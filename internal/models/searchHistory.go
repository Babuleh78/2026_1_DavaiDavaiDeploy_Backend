package models

import (
	"time"
	uuid "github.com/satori/go.uuid"
)

type SearchHistoryItem struct {
    ID        uuid.UUID `json:"id"`
    UserID    uuid.UUID `json:"user_id"`
    DanceID   string    `json:"dance_id"`
    Name      string    `json:"name"`
    SourceURL string    `json:"source_url"`
    CreatedAt time.Time `json:"created_at"`
}