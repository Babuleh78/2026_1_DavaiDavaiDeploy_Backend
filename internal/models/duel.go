package models

import (
	"time"

	uuid "github.com/satori/go.uuid"
)

const (
	DuelModeSingleDance = "single_dance"
	DuelModeRandomDance = "random_dance"

	DuelStatusPending        = "pending"
	DuelStatusActive         = "active"
	DuelStatusChallengerDone = "challenger_done"
	DuelStatusOpponentDone   = "opponent_done"
	DuelStatusCompleted      = "completed"
	DuelStatusExpired        = "expired"
	DuelStatusDeclined       = "declined"
)

type Duel struct {
	ID                  uuid.UUID  `json:"id"`
	Mode                string     `json:"mode"`
	ChallengerID        uuid.UUID  `json:"challenger_id"`
	OpponentID          uuid.UUID  `json:"opponent_id"`
	DanceID             string     `json:"dance_id,omitempty"`
	Status              string     `json:"status"`
	ChallengerAttemptID *uuid.UUID `json:"challenger_attempt_id,omitempty"`
	OpponentAttemptID   *uuid.UUID `json:"opponent_attempt_id,omitempty"`
	ChallengerScore     *float64   `json:"challenger_score,omitempty"`
	OpponentScore       *float64   `json:"opponent_score,omitempty"`
	WinnerID            *uuid.UUID `json:"winner_id,omitempty"`
	ExpiresAt           time.Time  `json:"expires_at"`
	CreatedAt           time.Time  `json:"created_at"`
	CompletedAt         *time.Time `json:"completed_at,omitempty"`
	IsPublic            bool       `json:"is_public"`
}

type DuelInvite struct {
	ID        uuid.UUID `json:"id"`
	DuelID    uuid.UUID `json:"duel_id"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
}

type DuelWithUsers struct {
	Duel
	ChallengerLogin  string `json:"challenger_login"`
	ChallengerAvatar string `json:"challenger_avatar"`
	OpponentLogin    string `json:"opponent_login"`
	OpponentAvatar   string `json:"opponent_avatar"`
	DanceTitle       string `json:"dance_title,omitempty"`
	InviteToken      string `json:"invite_token,omitempty"`
}

type CreateDuelInput struct {
	OpponentID string `json:"opponent_id"`
	Mode       string `json:"mode"`
	DanceID    string `json:"dance_id,omitempty"`
}

type DuelHistoryResponse struct {
	Duels      []DuelWithUsers `json:"duels"`
	Pagination PaginationMeta  `json:"pagination"`
}

type DuelParticipants struct {
	ID           uuid.UUID
	ChallengerID uuid.UUID
	OpponentID   uuid.UUID
}

type DuelStats struct {
	Total    int64   `json:"total"`
	Wins     int64   `json:"wins"`
	AvgScore float64 `json:"avg_score"`
	WinRate  float64 `json:"win_rate"`
}
