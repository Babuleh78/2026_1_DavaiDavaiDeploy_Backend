package models

import "time"

type DanceCatalogItem struct {
	ID                string    `json:"id"`
	URL               string    `json:"url"`
	Title             string    `json:"title,omitempty"`
	Difficulty        string    `json:"difficulty"`
	DifficultyScore   int       `json:"difficulty_score"`
	DifficultyByUsers bool      `json:"difficulty_by_users"`
	AttemptCount      int64     `json:"attempt_count"`
	AvgScore          float64   `json:"avg_score"`
	ViewCount         int64     `json:"view_count"`
	LikeCount         int64     `json:"like_count"`
	DurationSec       int       `json:"duration_sec"`
	CreatedAt         time.Time `json:"created_at"`
}

type PaginationMeta struct {
	Page    int  `json:"page"`
	Limit   int  `json:"limit"`
	Total   int  `json:"total"`
	HasMore bool `json:"has_more"`
}

type DanceCatalogResponse struct {
	Count      int                `json:"count"`
	Dances     []DanceCatalogItem `json:"dances"`
	Pagination PaginationMeta     `json:"pagination"`
}

type DanceStats struct {
	AttemptCount int64   `json:"attempt_count"`
	AvgScore     float64 `json:"avg_score"`
	TopScore     float64 `json:"top_score"`
	TopUser      string  `json:"top_user"`
	ViewCount    int64   `json:"view_count"`
}

type DanceStatusInput struct {
	Status string `json:"status"`
}

type DanceAuthor struct {
	ID     string `json:"id"`
	Login  string `json:"login"`
	Avatar string `json:"avatar"`
}

type SegmentInfo struct {
	Index       int     `json:"index"`
	StartTime   float64 `json:"start_time"`
	EndTime     float64 `json:"end_time"`
	Description string  `json:"description,omitempty"`
}
