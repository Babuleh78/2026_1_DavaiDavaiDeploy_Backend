package models

import (
	"html"
)

type LoadDanceResponse struct {
	DanceID             string   `json:"dance_id" binding:"required"`
	FullGlbKey          string   `json:"full_glb_key" binding:"required"`
	GlbKeys             []string `json:"glb_keys" binding:"required"`
	SegmentsKey         string   `json:"segments_key" binding:"required"`
	NumFrames           int      `json:"num_frames" binding:"required"`
	NumSegments         int      `json:"num_segments" binding:"required"`
	DurationSec         float64  `json:"duration_sec" binding:"required"`
	NumSegmentsRendered int      `json:"num_segments_rendered" binding:"required"`
}

func (l *LoadDanceResponse) Sanitize() {
	for i, v := range l.GlbKeys {
		l.GlbKeys[i] = html.EscapeString(v)
	}
	l.SegmentsKey = html.EscapeString(l.SegmentsKey)
}
