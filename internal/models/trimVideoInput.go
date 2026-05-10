package models

type TrimVideoInput struct {
    StartSec float64 `json:"start_sec"`
    EndSec   float64 `json:"end_sec"`
}