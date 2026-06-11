package domain

import (
	"errors"
	"fmt"
)

var (
	// ErrNotFound indicates the requested entity does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict indicates a uniqueness or state conflict (e.g. duplicate email).
	ErrConflict = errors.New("conflict")
	// ErrValidation indicates invalid input from the caller.
	ErrValidation = errors.New("validation error")
)

// ValidationError wraps ErrValidation with a human-readable message.
type ValidationError struct {
	Msg string
}

func (e *ValidationError) Error() string { return e.Msg }
func (e *ValidationError) Unwrap() error { return ErrValidation }

// Validationf builds a *ValidationError, formatting like fmt.Sprintf.
func Validationf(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}
