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
	// ErrForbidden is GitHub refusing access for a reason other than the
	// token or a rate limit, such as SSO or a missing permission.
	ErrForbidden = errors.New("forbidden")
	// ErrOffline is a request that never got an answer from GitHub.
	ErrOffline = errors.New("offline")
	// ErrUnavailable is GitHub answering with a server error.
	ErrUnavailable = errors.New("unavailable")
	// ErrTooLarge is matched by a *TooLargeError.
	ErrTooLarge = errors.New("too large")
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
