package core

import (
	"errors"
	"time"
)

// Errors that callers may test for with errors.Is.
var (
	ErrNotFound     = errors.New("not found")
	ErrUnauthorized = errors.New("unauthorized")
	ErrRateLimited  = errors.New("rate limited")
	ErrConflict     = errors.New("conflict")
)

// RateLimitError reports when the rate limit resets. It matches
// ErrRateLimited.
type RateLimitError struct {
	Reset time.Time
}

func (e *RateLimitError) Error() string {
	return "rate limited until " + e.Reset.Format(time.Kitchen)
}

// Is reports whether target is ErrRateLimited.
func (e *RateLimitError) Is(target error) bool {
	return target == ErrRateLimited
}
