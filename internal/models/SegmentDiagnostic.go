package models

type SegmentDiagnostic struct {
	SegmentID         int     `json:"segment_id"`
	Label             string  `json:"label"`
	TimingScore       float64 `json:"timing"`
	AmplitudeScore    float64 `json:"amplitude"`
	PoseAccuracyScore float64 `json:"pose_accuracy"`
	SegmentScore      float64 `json:"score"`
	Feedback          string  `json:"feedback"`
	OrigStartFrame    int     `json:"orig_start_frame"`
	OrigEndFrame      int     `json:"orig_end_frame"`
	UserStartFrame    int     `json:"user_start_frame"`
	UserEndFrame      int     `json:"user_end_frame"`
	OrigStartMs       float64 `json:"orig_start_ms"`
	OrigEndMs         float64 `json:"orig_end_ms"`
	UserStartMs       float64 `json:"user_start_ms"`
	UserEndMs         float64 `json:"user_end_ms"`
}
