package models

type WeakSpots struct {
	WorstMetric string  `json:"worst_metric"` // "timing" | "amplitude" | "pose"
	Avg         float64 `json:"avg"`
	Suggestion  string  `json:"suggestion"`
}
