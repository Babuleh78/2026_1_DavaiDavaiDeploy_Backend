package models

import "time"

type UploadedDance struct {
	DanceID           string    `json:"dance_id"`
	Title             string    `json:"title"`
	Status            string    `json:"status"`
	VideoPath         string    `json:"video_path"`
	UploadedAt        time.Time `json:"uploaded_at"`
	Difficulty        string    `json:"difficulty"`
	DifficultyByUsers bool      `json:"difficulty_by_users"`
	AttemptCount      int64     `json:"attempt_count"`
	AvgScore          float64   `json:"avg_score"`
	LikeCount         int64     `json:"like_count"`
	ViewCount         int64     `json:"view_count"`
}
