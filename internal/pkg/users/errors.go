package users

import "errors"

var (
	ErrorBadRequest          = errors.New("bad request")
	ErrorUnauthorized        = errors.New("user is unauthorized")
	ErrorInternalServerError = errors.New("internal server error")
	ErrorNotFound            = errors.New("not found")
	ErrorModerationPending   = errors.New("moderation pending")
	ErrorAlreadyExists       = errors.New("already exists")
	ErrorForbidden           = errors.New("forbidden")
)
