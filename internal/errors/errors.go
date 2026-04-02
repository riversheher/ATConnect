package errors

import (
	stderrors "errors"
	"fmt"
)

// Sentinel errors for common failure cases.
// Use errors.Is() to check against these.
var (
	ErrSessionNotFound  = stderrors.New("session not found")
	ErrTokenExpired     = stderrors.New("token expired")
	ErrInvalidCallback  = stderrors.New("invalid OAuth callback")
	ErrDuplicateState   = stderrors.New("duplicate auth state")
	ErrKeyNotFound      = stderrors.New("signing key not found")
	ErrClientNotFound   = stderrors.New("OIDC client not found")
	ErrInvalidConfig    = stderrors.New("invalid configuration")
	ErrStoreUnavailable = stderrors.New("store unavailable")
)

// AppError is a structured application error that carries a machine-readable
// code, human-readable message, HTTP status code, and an optional wrapped cause.
//
// It implements the error interface and supports Go's error wrapping via Unwrap().
type AppError struct {
	Code       string // Machine-readable code, e.g. "session_not_found"
	Message    string // Human-readable message
	HTTPStatus int    // Corresponding HTTP status code
	Err        error  // Wrapped underlying error (optional)
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the wrapped error for use with errors.Is/errors.As.
func (e *AppError) Unwrap() error {
	return e.Err
}

// New creates a new AppError with the given code, message, and HTTP status.
func New(code string, message string, httpStatus int) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
	}
}

// Wrap creates a new AppError wrapping an existing error.
func Wrap(code string, message string, httpStatus int, err error) *AppError {
	return &AppError{
		Code:       code,
		Message:    message,
		HTTPStatus: httpStatus,
		Err:        err,
	}
}
