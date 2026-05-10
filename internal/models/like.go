package models

import "time"

type LikeResponse struct {
    DanceID   string `json:"dance_id"`
    Liked     bool   `json:"liked"`
    LikesCount int64 `json:"likes_count"`
}

type DanceLikeStat struct {
    DanceID    string `json:"dance_id"`
    LikesCount int64  `json:"likes_count"`
}

type DanceLike struct {
    HistoryID string    `json:"history_id"`
    DanceID   string    `json:"dance_id"`
    Name      string    `json:"name"`
    CreatedAt time.Time `json:"created_at"`
}

type UserLikedDancesResponse struct {
    Likes []DanceLike `json:"likes"`
}