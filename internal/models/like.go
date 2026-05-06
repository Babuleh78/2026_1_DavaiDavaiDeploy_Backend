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
    DanceID string       `json:"dance_id"`
    CreatedAt time.Time  `json:"created_at"`
}

type UserLikeDancesResponse struct {
    Likes []DanceLike `json:"likes"`
    Count int         `json:"count"`
}