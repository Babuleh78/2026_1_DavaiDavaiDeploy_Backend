package models

type CompareTip struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type CompareDanceStats struct {
	AttemptCount int64   `json:"attempt_count"`
	BestScore    float64 `json:"best_score"`
}

type FrameScore struct {
	Frame       int       `json:"frame"`
	TimeSec     float64   `json:"time_sec"`
	Error       float64   `json:"error"`
	JointErrors []float64 `json:"joint_errors,omitempty"`
}

type FrameLabel struct {
	FrameIdx       int       `json:"frame_idx"`
	TimestampMs    float64   `json:"timestamp_ms"`
	Hit            bool      `json:"hit"`
	Reason         string    `json:"reason"`
	TimingScore    float64   `json:"timing_score"`
	AmplitudeScore float64   `json:"amplitude_score"`
	PoseScore      float64   `json:"pose_score"`
	JointErrors    []float64 `json:"joint_errors,omitempty"`
}

type CompareAttemptOwner struct {
	UserID string `json:"user_id"`
	Login  string `json:"login"`
}

type CompareResult struct {
	TaskID               string               `json:"task_id,omitempty"`
	UserGlbKey           string               `json:"user_glb_key"`
	ReferenceGlbKey      string               `json:"reference_glb_key"`
	Score                float64              `json:"score"`
	DtwDistance          float64              `json:"dtw_distance"`
	DanceID              string               `json:"dance_id"`
	UserDanceID          string               `json:"user_dance_id"`
	TimelineS3           string               `json:"timeline_s3"`
	Segments             []SegmentDiagnostic  `json:"segments"`
	Tips                 []CompareTip         `json:"tips,omitempty"`
	DanceStats           *CompareDanceStats   `json:"dance_stats,omitempty"`
	FrameScores          []FrameScore         `json:"frame_scores,omitempty"`
	FrameLabels          []FrameLabel         `json:"frame_labels,omitempty"`
	UserVideoKey         string               `json:"user_video_key,omitempty"`
	UserSkeletonKey      string               `json:"user_skeleton_key,omitempty"`
	ReferenceSkeletonKey string               `json:"reference_skeleton_key,omitempty"`
	Owner                *CompareAttemptOwner `json:"owner,omitempty"`
}
