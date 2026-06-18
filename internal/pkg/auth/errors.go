package auth

import "errors"

var (
	ErrorBadRequest          = errors.New("wrong login or password")
	ErrorConflict            = errors.New("user already exists")
	ErrorLoginAlreadyExists  = errors.New("login already taken")
	ErrorUnauthorized        = errors.New("user is unauthorized")
	ErrorInternalServerError = errors.New("internal server error")
	ErrorPreconditionFailed  = errors.New("precondition failed")
)
