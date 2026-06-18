package models

type ReelsAttemptItem struct {
	AttemptID  string  `json:"attempt_id"`
	DanceID    string  `json:"dance_id"`
	DanceTitle string  `json:"dance_title"`
	UserID     string  `json:"user_id"`
	UserLogin  string  `json:"user_login"`
	UserAvatar string  `json:"user_avatar"`
	Score      float64 `json:"score"`
	VideoKey   string  `json:"video_key"`
}
