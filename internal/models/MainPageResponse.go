package models

type VideoItem struct {
	ID           string  `json:"id"`
	URL          string  `json:"url"`
	Title        string  `json:"title,omitempty"`
	AttemptCount int64   `json:"attempt_count"`
	AvgScore     float64 `json:"avg_score"`
	ViewCount    int64   `json:"view_count"`
	LikeCount    int64   `json:"like_count"`
}

type MainPageResponse struct {
	Count  int         `json:"count"`
	Videos []VideoItem `json:"videos"`
}

type TrendingResponse struct {
	Count  int         `json:"count"`
	Videos []VideoItem `json:"videos"`
}

type DanceEnrichedInfo struct {
	Title        string
	AttemptCount int64
	AvgScore     float64
	ViewCount    int64
	LikeCount    int64
}
