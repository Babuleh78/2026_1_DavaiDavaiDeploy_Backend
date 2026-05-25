package models

type SaveAttemptInput struct {
	DanceID      string   `json:"dance_id"`
	IncludeVideo bool     `json:"include_video"`
	Score        *float64 `json:"score,omitempty"`
	UserName     string   `json:"user_name"`
	IsPrivate    bool     `json:"is_private"`
}
