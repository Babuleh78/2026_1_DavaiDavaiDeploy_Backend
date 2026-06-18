package models

type BotAuthResponse struct {
	OK       bool   `json:"ok"`
	UserID   string `json:"user_id,omitempty"`
	Username string `json:"username,omitempty"`
	Error    string `json:"error,omitempty"`
}

type BotUserStats struct {
	AttemptCount         int64   `json:"attempt_count"`
	MaxScore             float64 `json:"max_score"`
	DuelCount            int64   `json:"duel_count"`
	DuelWinCount         int64   `json:"duel_win_count"`
	AchievementsUnlocked int     `json:"achievements_unlocked"`
	AchievementsTotal    int     `json:"achievements_total"`
}
