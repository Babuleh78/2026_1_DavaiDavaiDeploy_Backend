package models

type LikeResponse struct {
    DanceID   string `json:"dance_id"`
    Liked     bool   `json:"liked"`
    LikesCount int64 `json:"likes_count"`
}

type DanceLikeStat struct {
    DanceID    string `json:"dance_id"`
    LikesCount int64  `json:"likes_count"`
}