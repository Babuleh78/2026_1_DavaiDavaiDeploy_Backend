package models

import "time"

type Achievement struct {
	ID          int    `json:"id"`
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	IconKey     string `json:"icon_key"`
	Category    string `json:"category"`
	Threshold   int    `json:"threshold"`
}

type UserAchievement struct {
	Achievement
	UnlockedAt time.Time `json:"unlocked_at"`
}

type AchievementWithStatus struct {
	Achievement
	Unlocked   bool       `json:"unlocked"`
	UnlockedAt *time.Time `json:"unlocked_at,omitempty"`
}

type AchievementsWithMeta struct {
	Achievements  []AchievementWithStatus `json:"achievements"`
	UnlockedCount int                     `json:"unlocked_count"`
	TotalCount    int                     `json:"total_count"`
	Percentile    float64                 `json:"percentile"`
}

type UserAchievementStats struct {
	TotalLikes       int64   `json:"total_likes"`
	MaxScore         float64 `json:"max_score"`
	UploadCount      int64   `json:"upload_count"`
	AttemptCount     int64   `json:"attempt_count"`
	DuelCount        int64   `json:"duel_count"`
	DuelWinCount     int64   `json:"duel_win_count"`
	UniqueDanceCount int64   `json:"unique_dance_count"`
	DuelWinStreak    int64   `json:"duel_win_streak"`
}
