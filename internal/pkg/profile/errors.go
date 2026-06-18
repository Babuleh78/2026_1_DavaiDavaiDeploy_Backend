package profile

import "DDDance/internal/pkg/users"

var (
	ErrorBadRequest          = users.ErrorBadRequest
	ErrorNotFound            = users.ErrorNotFound
	ErrorForbidden           = users.ErrorForbidden
	ErrorInternalServerError = users.ErrorInternalServerError
	ErrorAlreadyExists       = users.ErrorAlreadyExists
)
