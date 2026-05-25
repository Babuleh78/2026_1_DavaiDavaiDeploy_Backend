package models

import "encoding/json"

type AsyncEnqueueResult struct {
	TaskID          string `json:"task_id"`
	DanceID         string `json:"dance_id,omitempty"`
	UserDanceID     string `json:"user_dance_id,omitempty"`
	ReferenceDanceID string `json:"reference_dance_id,omitempty"`
	Status          string `json:"status"`
}

type TaskStatusResponse struct {
	Status     string          `json:"status"` 
	Stage      string          `json:"stage"`
	StageLabel string          `json:"stage_label"`
	Progress   int             `json:"progress"` 
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
}

type MlStatusResp struct {
	Status     string          `json:"status"`
	Stage      string          `json:"stage"`
	StageLabel string          `json:"stage_label"`
	Progress   int             `json:"progress"`
	Result     json.RawMessage `json:"result,omitempty"`
	Error      string          `json:"error,omitempty"`
}
