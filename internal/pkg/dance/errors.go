package dance

import "DDDance/internal/pkg/users"

var (
	ErrorBadRequest          = users.ErrorBadRequest
	ErrorNotFound            = users.ErrorNotFound
	ErrorForbidden           = users.ErrorForbidden
	ErrorModerationPending   = users.ErrorModerationPending
	ErrorInternalServerError = users.ErrorInternalServerError
)
