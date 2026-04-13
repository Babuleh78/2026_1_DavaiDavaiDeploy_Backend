package models
type ProcessingResult struct {
    DanceID             string   `json:"dance_id"`
	VideoPath			string   `json:"video_path"`
	FullGlbKey          string   `json:"full_glb_key"`
    SegmentsKey         string   `json:"segments_key"`
    GlbKeys             []string `json:"glb_keys"`
    NumFrames           int      `json:"num_frames"`
    NumSegments         int      `json:"num_segments"`
    NumSegmentsRendered int      `json:"num_segments_rendered"`
    DurationSec         float64  `json:"duration_sec"`
}