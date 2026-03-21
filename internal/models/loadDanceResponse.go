package models

import (
	"html"
)

type LoadDanceResponse struct {
	ResultKey   string  `json:"result_key" binding:"required"`
	NumFrames   int     `json:"num_frames" binding:"required"`
	NumSegments int     `json:"num_segments" binding:"required"`
	DurationSec float64 `json:"duration_sec" binding:"required"`
}

func (l *LoadDanceResponse) Sanitize() {
	l.ResultKey = html.EscapeString(l.ResultKey)
}
