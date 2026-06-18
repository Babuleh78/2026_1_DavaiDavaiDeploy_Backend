package models

type SaveAttemptInput struct {
	DanceID        string   `json:"dance_id"`
	IncludeVideo   bool     `json:"include_video"`
	Score          *float64 `json:"score,omitempty"`
	TimingScore    *float64 `json:"timing_score,omitempty"`
	AmplitudeScore *float64 `json:"amplitude_score,omitempty"`
	PoseScore      *float64 `json:"pose_score,omitempty"`
	UserName       string   `json:"user_name"`
	IsPrivate      bool     `json:"is_private"`
	DuelID         *string  `json:"duel_id,omitempty"`
	PublicConsent  bool     `json:"public_consent"`
	SubmitToDuels  bool     `json:"submit_to_duels"`
}
