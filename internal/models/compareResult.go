package models

type CompareResult struct {
    UserGlbKey      string  `json:"user_glb_key"`
    ReferenceGlbKey string  `json:"reference_glb_key"`
    Score           float64 `json:"score"`
    DtwDistance     float64 `json:"dtw_distance"`
    DanceID         string  `json:"dance_id"`
    UserDanceID     string  `json:"user_dance_id"`
}