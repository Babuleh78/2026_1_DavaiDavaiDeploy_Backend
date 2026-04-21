package models

type DanceCompareRequest struct {
    VideoKey   string `json:"video_key"`
    DanceID    string `json:"dance_id"`
    SegmentIdx int    `json:"segment_idx"`
}

type VelocityMetrics struct {
    Mean float64 `json:"mean"`
    Max  float64 `json:"max"`
    Std  float64 `json:"std"`
}

type ROMMetrics struct {
    MaxDistance  float64 `json:"max_distance"`
    MeanDistance float64 `json:"mean_distance"`
}

type JointAngleMetrics struct {
    MeanDeg  float64 `json:"mean_deg"`
    RangeDeg float64 `json:"range_deg"`
}

type SegmentNumericMetrics struct {
    Velocity     VelocityMetrics              `json:"velocity"`
    Smoothness   float64                      `json:"smoothness"`
    ROM          ROMMetrics                   `json:"rom"`
    TempoBPM     float64                      `json:"tempo_bpm"`
    SymmetryRatio float64                     `json:"symmetry_ratio"`
    JointAngles  map[string]JointAngleMetrics `json:"joint_angles"`
}

type SegmentCompareDetail struct {
    SegmentIdx      int                `json:"segment_idx"`
    DTWScores       map[string]float64 `json:"dtw_scores"`
    VelocityDiff    float64            `json:"velocity_diff"`
    SmoothnessDiff  float64            `json:"smoothness_diff"`
    ROMDiff         float64            `json:"rom_diff"`
    TempoDiff       float64            `json:"tempo_diff"`
    SymmetryDiff    float64            `json:"symmetry_diff"`
    JointAnglesDiff map[string]float64 `json:"joint_angles_diff"`
    SegmentScore    float64            `json:"segment_score"`
}

type DanceCompareResponse struct {
    DanceID        string                 `json:"dance_id"`
    SegmentIdx     int                    `json:"segment_idx"`
    OverallScore   float64                `json:"overall_score"`
    Segments       []SegmentCompareDetail `json:"segments"`
    WeakestMetrics []string               `json:"weakest_metrics"`
}