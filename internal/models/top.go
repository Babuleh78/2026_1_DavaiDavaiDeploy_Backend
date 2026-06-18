package models

import "time"

type TopDancerEntry struct {
	UserID       string  `json:"user_id"`
	Username     string  `json:"username"`
	Avatar       string  `json:"avatar"`
	AvgScore     float64 `json:"avg_score"`
	AttemptCount int     `json:"attempt_count"`
	BestScore    float64 `json:"best_score"`
}

type RecommenderDanceItem struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	AvgScore    float64   `json:"avg_score"`
	ViewCount   int64     `json:"view_count"`
	UploaderID  string    `json:"uploader_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type RecommendDance struct {
	ID        string  `json:"id"`
	Title     string  `json:"title"`
	URL       string  `json:"url"`
	AvgScore  float64 `json:"avg_score"`
	ViewCount int64   `json:"view_count"`
}

type RecommendResponse struct {
	Reasoning string           `json:"reasoning"`
	Dances    []RecommendDance `json:"dances"`
}
