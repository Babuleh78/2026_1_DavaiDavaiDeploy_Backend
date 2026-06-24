package repo

import (
	"github.com/jackc/pgtype/pgxtype"
)

type UserRepository struct {
	db pgxtype.Querier
}

func NewUserRepository(db pgxtype.Querier) *UserRepository {
	return &UserRepository{db: db}
}
