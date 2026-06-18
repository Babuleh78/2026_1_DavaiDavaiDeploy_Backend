package models

type ReelItem struct {
	DanceID      string  `json:"dance_id"`
	Title        string  `json:"title"`
	UploaderID   string  `json:"uploader_id"`
	Username     string  `json:"username"`
	AvatarURL    string  `json:"avatar_url"`
	VideoURL     string  `json:"video_url"`
	PreviewURL   string  `json:"preview_url"`
	ViewCount    int64   `json:"view_count"`
	LikeCount    int64   `json:"like_count"`
	AvgScore     float64 `json:"avg_score"`
	AttemptCount int64   `json:"attempt_count"`
	UserLiked    bool    `json:"user_liked"`
}

type ReelsFeedResponse struct {
	Items []ReelItem `json:"items"`
	Total int        `json:"total"`
}

type UserReelsHistoryItem struct {
	DanceID  string  `json:"dance_id"`
	Score    float64 `json:"score"`
	ViewedAt string  `json:"viewed_at"`
	Liked    bool    `json:"liked"`
}

type BehaviorLogEntry struct {
	DanceID   string `json:"dance_id"`
	Action    string `json:"action"`
	Timestamp int64  `json:"timestamp"`
}
