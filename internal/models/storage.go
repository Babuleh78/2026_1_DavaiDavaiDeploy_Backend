package models

import "time"

type StoredUserVideo struct {
	Key          string
	UserID       string
	DanceID      string
	LastModified time.Time
}
