package errors

import "net/http"

// Error code constants provide machine-readable identifiers for API responses
// and structured logging.
const (
	CodeSessionNotFound  = "session_not_found"
	CodeTokenExpired     = "token_expired"
	CodeInvalidCallback  = "invalid_callback"
	CodeDuplicateState   = "duplicate_state"
	CodeKeyNotFound      = "key_not_found"
	CodeClientNotFound   = "client_not_found"
	CodeInvalidConfig    = "invalid_config"
	CodeStoreUnavailable = "store_unavailable"
	CodeInternalError    = "internal_error"
	CodeUnauthorized     = "unauthorized"
	CodeBadRequest       = "bad_request"
)

// Pre-built AppErrors for common failure cases.
// These can be returned directly from handlers or used as templates.
var (
	ErrAppSessionNotFound = &AppError{
		Code:       CodeSessionNotFound,
		Message:    "The requested session was not found",
		HTTPStatus: http.StatusNotFound,
	}

	ErrAppTokenExpired = &AppError{
		Code:       CodeTokenExpired,
		Message:    "The authentication token has expired",
		HTTPStatus: http.StatusUnauthorized,
	}

	ErrAppInvalidCallback = &AppError{
		Code:       CodeInvalidCallback,
		Message:    "Invalid OAuth callback parameters",
		HTTPStatus: http.StatusBadRequest,
	}

	ErrAppInternalError = &AppError{
		Code:       CodeInternalError,
		Message:    "An internal error occurred",
		HTTPStatus: http.StatusInternalServerError,
	}

	ErrAppStoreUnavailable = &AppError{
		Code:       CodeStoreUnavailable,
		Message:    "The data store is unavailable",
		HTTPStatus: http.StatusServiceUnavailable,
	}
)
