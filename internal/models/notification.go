package models

import "time"

const (
	NotifDuelChallengeReceived = "duel_challenge_received"
	NotifDuelAccepted          = "duel_accepted"
	NotifDuelDeclined          = "duel_declined"
	NotifDuelChallengerDone    = "duel_challenger_done"
	NotifDuelOpponentDone      = "duel_opponent_done"
	NotifDuelCompleted         = "duel_completed"
	NotifDuelExpired           = "duel_expired"
)

type Notification struct {
	ID         int64     `json:"id"`
	Type       string    `json:"type"`
	DanceID    string    `json:"dance_id,omitempty"`
	DuelID     string    `json:"duel_id,omitempty"`
	Reason     string    `json:"reason,omitempty"`
	FromUserID string    `json:"from_user_id,omitempty"`
	FromLogin  string    `json:"from_login,omitempty"`
	RefID      int64     `json:"ref_id,omitempty"`
	IsRead     bool      `json:"is_read"`
	CreatedAt  time.Time `json:"created_at"`
}

type NotificationsResponse struct {
	Notifications []Notification `json:"notifications"`
	UnreadCount   int            `json:"unread_count"`
}

type ClaimUploadsInput struct {
	DanceIDs []string `json:"dance_ids"`
}
