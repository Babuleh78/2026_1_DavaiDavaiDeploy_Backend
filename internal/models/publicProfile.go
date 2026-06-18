package models

import (
	"time"

	uuid "github.com/satori/go.uuid"
)

type SavedAttemptItem struct {
	UserDanceID       string    `json:"user_dance_id"`
	DanceID           string    `json:"dance_id"`
	DanceTitle        string    `json:"dance_title"`
	UserName          string    `json:"user_name"`
	IsPrivate         bool      `json:"is_private"`
	Score             float64   `json:"score"`
	SavedAt           time.Time `json:"saved_at"`
	ReferenceVideoKey string    `json:"reference_video_key"`
	UserAnimationKey  string    `json:"user_animation_key"`
	UserSkeletonKey   string    `json:"user_skeleton_key"`
	UserVideoKey      string    `json:"user_video_key,omitempty"`
	HasVideo          bool      `json:"has_video"`
}

type PersonalTopItem struct {
	DanceID     string    `json:"dance_id"`
	UserDanceID string    `json:"user_dance_id"`
	DanceTitle  string    `json:"dance_title"`
	UserName    string    `json:"user_name"`
	BestScore   float64   `json:"best_score"`
	AchievedAt  time.Time `json:"achieved_at"`
}

type PublicProfileUser struct {
	ID        uuid.UUID `json:"id"`
	Login     string    `json:"login"`
	Avatar    string    `json:"avatar"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProfileStats struct {
	AttemptCount     int64   `json:"attempt_count"`
	MaxScore         float64 `json:"max_score"`
	UniqueDanceCount int64   `json:"unique_dance_count"`
	DuelWinCount     int64   `json:"duel_win_count"`
	UploadCount      int64   `json:"upload_count"`
}

type PublicProfileResponse struct {
	User             PublicProfileUser  `json:"user"`
	SavedAttempts    []SavedAttemptItem `json:"saved_attempts"`
	UploadedDances   []UploadedDance    `json:"uploaded_dances"`
	PersonalTop      []PersonalTopItem  `json:"personal_top"`
	IsOwnProfile     bool               `json:"is_own_profile"`
	FriendsCount     int                `json:"friends_count"`
	FriendshipStatus *FriendshipStatus  `json:"friendship_status,omitempty"`
	Stats            *ProfileStats      `json:"stats,omitempty"`
}
