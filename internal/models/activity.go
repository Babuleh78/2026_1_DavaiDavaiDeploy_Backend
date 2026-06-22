package models

import (
	"encoding/json"
	"time"
)

type FeedItem struct {
	ID          string          `json:"id"`
	ActionType  string          `json:"action_type"`
	Metadata    json.RawMessage `json:"metadata"`
	CreatedAt   time.Time       `json:"created_at"`
	ActorLogin  string          `json:"actor_login"`
	ActorAvatar string          `json:"actor_avatar"`
	ActorID     string          `json:"actor_id"`
	DanceTitle  string          `json:"dance_title,omitempty"`
}

type FeedResponse struct {
	Items      []FeedItem `json:"items"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

type ActivityEntry struct {
	Day   string `json:"day"` // ISO date "2006-01-02"
	Count int    `json:"count"`
}

type MostImprovedDance struct {
	DanceID    string  `json:"dance_id"`
	Title      string  `json:"title"`
	FirstScore float64 `json:"first_score"`
	LastScore  float64 `json:"last_score"`
	Delta      float64 `json:"delta"`
}

type CreatorDailyStats struct {
	Day      string `json:"day"`
	Views    int    `json:"views"`
	Likes    int    `json:"likes"`
	Attempts int    `json:"attempts"`
}

type CreatorTopDance struct {
	DanceID  string `json:"dance_id"`
	Title    string `json:"title"`
	Attempts int    `json:"attempts"`
	Likes    int    `json:"likes"`
}

type CreatorAnalytics struct {
	Daily     []CreatorDailyStats `json:"daily"`
	TopDances []CreatorTopDance   `json:"top_dances"`
}
