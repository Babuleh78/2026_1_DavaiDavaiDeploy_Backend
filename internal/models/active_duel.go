package models

import uuid "github.com/satori/go.uuid"

type ActiveDuelForDance struct {
	DuelID        uuid.UUID `json:"duel_id"`
	OpponentLogin string    `json:"opponent_login"`
}
