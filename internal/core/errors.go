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
	// ErrUnsupported is a GitHub Enterprise Server whose version lacks
	// what was asked, such as a field of a query.
	ErrUnsupported = errors.New("unsupported")
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

// ScopeError is an operation that the token lacks a scope for, as GitHub
// said or as Access knew before asking. It matches ErrUnauthorized, and
// Err, the error it came with, if any.
type ScopeError struct {
	// Scopes are those of which the operation needs any one, the one to
	// grant first.
	Scopes []string
	Err    error
}

func (e *ScopeError) Error() string {
	msg := "the token lacks the " + e.Grant() + " scope"
	if e.Err == nil {
		return msg
	}
	return msg + ": " + e.Err.Error()
}

// Grant returns the scope to grant the token, or "" if none is known.
func (e *ScopeError) Grant() string {
	if len(e.Scopes) == 0 {
		return ""
	}
	return e.Scopes[0]
}

// Unwrap returns ErrUnauthorized and Err.
func (e *ScopeError) Unwrap() []error {
	if e.Err == nil {
		return []error{ErrUnauthorized}
	}
	return []error{ErrUnauthorized, e.Err}
}

// KindError is an operation that no token of Kind may do, whatever it is
// allowed, as a fine-grained token may not read notifications. It
// matches ErrUnauthorized.
type KindError struct {
	Kind TokenKind
}

func (e *KindError) Error() string {
	return "a token of kind " + e.Kind.String() + " can't do this"
}

// Reason says what the operation needs, for Explain.
func (e *KindError) Reason() string {
	return "needs a classic token"
}

// Is reports whether target is ErrUnauthorized.
func (e *KindError) Is(target error) bool {
	return target == ErrUnauthorized
}

// SSOError is an organization refusing a token that isn't authorized for
// its single sign-on. It matches ErrForbidden, and Err, the error it came
// with, if any.
type SSOError struct {
	// URL is where the user authorizes the token, or "" if GitHub didn't
	// say.
	URL string
	Err error
}

func (e *SSOError) Error() string {
	if e.Err == nil {
		return "the token isn't authorized for the organization's SSO"
	}
	return "sso: " + e.Err.Error()
}

// Unwrap returns ErrForbidden and Err.
func (e *SSOError) Unwrap() []error {
	if e.Err == nil {
		return []error{ErrForbidden}
	}
	return []error{ErrForbidden, e.Err}
}
