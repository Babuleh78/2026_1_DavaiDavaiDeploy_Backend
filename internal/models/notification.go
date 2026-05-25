package models

import "time"

type Notification struct {
	ID          int64     `json:"id"`
	Type        string    `json:"type"`
	DanceID     string    `json:"dance_id,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	FromUserID  string    `json:"from_user_id,omitempty"`
	FromLogin   string    `json:"from_login,omitempty"`
	RefID       int64     `json:"ref_id,omitempty"`
	IsRead      bool      `json:"is_read"`
	CreatedAt   time.Time `json:"created_at"`
}

type NotificationsResponse struct {
	Notifications []Notification `json:"notifications"`
	UnreadCount   int            `json:"unread_count"`
}

type ClaimUploadsInput struct {
	DanceIDs []string `json:"dance_ids"`
}
