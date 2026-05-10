package models

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type DanceCompareRequest struct {
    VideoKey   string `json:"video_key"`
    DanceID    string `json:"dance_id"`
    SegmentIdx int    `json:"segment_idx"`
}

// UnmarshalJSON custom JSON unmarshaler to handle dance_id as either string or number
func (d *DanceCompareRequest) UnmarshalJSON(data []byte) error {
	// Use interface{} to accept any type
	aux := &struct {
		VideoKey   interface{} `json:"video_key"`
		DanceID    interface{} `json:"dance_id"`
		SegmentIdx interface{} `json:"segment_idx"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Convert VideoKey
	if aux.VideoKey != nil {
		d.VideoKey = fmt.Sprintf("%v", aux.VideoKey)
	}

	// Convert DanceID - handle both string and number
	if aux.DanceID != nil {
		switch v := aux.DanceID.(type) {
		case string:
			d.DanceID = v
		case float64:
			d.DanceID = strconv.FormatFloat(v, 'f', -1, 64)
		}
	}

	// Convert SegmentIdx
	if aux.SegmentIdx != nil {
		switch v := aux.SegmentIdx.(type) {
		case float64:
			d.SegmentIdx = int(v)
		case string:
			if intVal, err := strconv.Atoi(v); err == nil {
				d.SegmentIdx = intVal
			}
		}
	}

	return nil
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