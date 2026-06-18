package models

import uuid "github.com/satori/go.uuid"

type UserSearchItem struct {
	ID     uuid.UUID `json:"id"`
	Login  string    `json:"login"`
	Avatar string    `json:"avatar"`
}
