package models

type SegmentDescriptionResult struct {
    DanceID     string `json:"dance_id"`
    SegmentIdx  int    `json:"segment_idx"`
    Description string `json:"description"`
}
