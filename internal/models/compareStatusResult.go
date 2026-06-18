package models

type CompareStatusResult struct {
	Success *bool  `json:"success,omitempty"`
	Error   string `json:"error,omitempty"`

	DanceID         string              `json:"dance_id,omitempty"`
	UserID          string              `json:"user_id,omitempty"`
	ComparisonScore float64             `json:"comparison_score"`
	DtwDistance     float64             `json:"dtw_distance"`
	OriginalVideoS3 string              `json:"original_video_s3"`
	UserVideoS3     string              `json:"user_video_s3"`
	UserGlbS3       string              `json:"user_glb_s3"`
	ProcessedAt     string              `json:"processed_at"`
	Segments        []SegmentDiagnostic `json:"segments"`
	Tips            []CompareTip        `json:"tips"`
	FrameScores     []FrameScore        `json:"frame_scores"`
	FrameLabels     []FrameLabel        `json:"frame_labels,omitempty"`
}
