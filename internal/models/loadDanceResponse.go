package models

import (
	"html"
)

type LoadDanceResponse struct {
	DanceID             string        `json:"dance_id"`
	FullGlbKey          string        `json:"full_glb_key"`
	GlbKeys             []string      `json:"glb_keys"`
	SegmentsKey         string        `json:"segments_key"`
	NumFrames           int           `json:"num_frames"`
	NumSegments         int           `json:"num_segments"`
	DurationSec         float64       `json:"duration_sec"`
	NumSegmentsRendered int           `json:"num_segments_rendered"`
	VideoPath           string        `json:"video_path"`
	KeyframesURL        string        `json:"keyframes_url,omitempty"`
	Segments            []SegmentInfo `json:"segments,omitempty"`
	LikesCount          int64         `json:"likes_count"`
	IsLiked             bool          `json:"is_liked"`
	Author              *DanceAuthor  `json:"author,omitempty"`
	Difficulty          string        `json:"difficulty,omitempty"`
	DifficultyByUsers   bool          `json:"difficulty_by_users"`
	LastAttemptID       string        `json:"last_attempt_id,omitempty"`
	LastAttemptScore    *float64      `json:"last_attempt_score,omitempty"`
	UniqueViewersApprox int64         `json:"unique_viewers_approx,omitempty"`
}

func (l *LoadDanceResponse) Sanitize() {
	for i, v := range l.GlbKeys {
		l.GlbKeys[i] = html.EscapeString(v)
	}
	l.SegmentsKey = html.EscapeString(l.SegmentsKey)
}
