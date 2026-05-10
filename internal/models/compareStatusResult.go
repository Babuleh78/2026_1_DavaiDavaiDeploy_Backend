package models

type CompareStatusResult struct {
    ComparisonScore float64 `json:"comparison_score"`
    DtwDistance     float64 `json:"dtw_distance"`
    OriginalVideoS3 string  `json:"original_video_s3"`
    UserVideoS3     string  `json:"user_video_s3"`
    UserGlbS3       string  `json:"user_glb_s3"`
    ProcessedAt     string  `json:"processed_at"`
}
