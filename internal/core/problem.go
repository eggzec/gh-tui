package core

import (
	"context"
	"errors"
	"strconv"
	"time"
)

// ProblemKind sorts a failure by what the user can do about it.
type ProblemKind uint8

// The kinds of problem, from Explain.
const (
	// Internal is anything unexpected: a bug, or a response the app
	// doesn't understand.
	Internal ProblemKind = iota
	// Canceled is work the app stopped itself, such as a read that the
	// user navigated away from.
	Canceled
	// Offline means GitHub couldn't be reached.
	Offline
	// Unavailable means GitHub answered with a server error.
	Unavailable
	// RateLimited means the account used up a rate limit, until Reset.
	RateLimited
	// Auth means GitHub rejected the token, or the token lacks a scope.
	Auth
	// Forbidden means the account may not see or do what was asked, as
	// when an organization requires SSO.
	Forbidden
	// NotFound means what was asked doesn't exist, or the account may not
	// know that it does.
	NotFound
	// Rejected means GitHub refused an action for a reason it gave, such
	// as a pull request that can't be merged.
	Rejected
)

func (k ProblemKind) String() string {
	switch k {
	case Canceled:
		return "canceled"
	case Offline:
		return "offline"
	case Unavailable:
		return "unavailable"
	case RateLimited:
		return "rate limited"
	case Auth:
		return "auth"
	case Forbidden:
		return "forbidden"
	case NotFound:
		return "not found"
	case Rejected:
		return "rejected"
	default:
		return "internal"
	}
}

// Problem is an error said the way the user should see it: what failed,
// of what kind, and GitHub's reason if it gave one. Error keeps the whole
// chain, for logs.
type Problem struct { //nolint:errname // It explains an error for the user rather than being another kind of one.
	Kind ProblemKind
	// Action is what failed, such as "load your profile" or "merge #5".
	Action string
	// Subject is what the action was on, such as "eggzec/gh-tui#5", when
	// the error names it.
	Subject string
	// Reason is GitHub's explanation on one clean line, or empty.
	Reason string
	// Reset is when a rate limit lifts, for RateLimited.
	Reset time.Time
	// Grant is the scope to grant the token, for Auth, or "" if the
	// problem isn't a missing scope, or GitHub didn't say which.
	Grant string
	// SSO is set for Forbidden when an organization refused the token
	// until it is authorized for the organization's single sign-on, and
	// SSOURL is where to authorize it, or "" if GitHub didn't say.
	SSO    bool
	SSOURL string
	Err    error
}

func (p *Problem) Error() string {
	if p.Action == "" {
		return p.Err.Error()
	}
	return p.Action + ": " + p.Err.Error()
}

// Unwrap returns the error the problem explains.
func (p *Problem) Unwrap() error {
	return p.Err
}

// SubjectError names what an error is about, such as the pull request
// "eggzec/gh-tui#5" a change failed on, for Explain to name when the error
// doesn't name it itself. It says nothing more than the error it wraps.
type SubjectError struct {
	Subject string
	Err     error
}

func (e *SubjectError) Error() string {
	return e.Err.Error()
}

// Unwrap returns the error that is about Subject.
func (e *SubjectError) Unwrap() error {
	return e.Err
}

// About returns err as an error about subject, or nil for a nil err.
func About(subject string, err error) error {
	if err == nil {
		return nil
	}
	return &SubjectError{Subject: subject, Err: err}
}

// reasoner is an error that carries the explanation GitHub gave, already
// on one clean line, such as the errors of the github package.
type reasoner interface {
	error
	Reason() string
}

// Explain says what kind of problem err is, for action, such as "load
// your profile". It returns nil for a nil err. When err is or wraps a
// *Problem, that one is returned as it is, so explaining twice changes
// nothing.
func Explain(action string, err error) *Problem {
	if err == nil {
		return nil
	}
	if p, ok := errors.AsType[*Problem](err); ok {
		return p
	}
	p := &Problem{Kind: kind(err), Action: action, Err: err}
	if r, ok := errors.AsType[reasoner](err); ok {
		p.Reason = r.Reason()
	}
	switch p.Kind {
	case Auth:
		if e, ok := errors.AsType[*ScopeError](err); ok {
			p.Grant = e.Grant()
			if p.Reason == "" && p.Grant != "" {
				p.Reason = "needs the " + p.Grant + " scope"
			}
		}
	case Forbidden:
		if e, ok := errors.AsType[*SSOError](err); ok {
			p.SSO, p.SSOURL = true, e.URL
		}
	case RateLimited:
		if rl, ok := errors.AsType[*RateLimitError](err); ok {
			p.Reset = rl.Reset
		}
	case NotFound:
		if e, ok := errors.AsType[*NoNumberError](err); ok {
			p.Subject = e.Repo.String() + "#" + strconv.Itoa(e.Number)
		}
	case Rejected:
		if p.Reason != "" {
			break
		}
		if e, ok := errors.AsType[*RefusedError](err); ok {
			p.Reason = e.Reason
		} else if e, ok := errors.AsType[*InvalidQueryError](err); ok {
			p.Reason = e.Reason
		}
	default:
	}
	if e, ok := errors.AsType[*SubjectError](err); ok && p.Subject == "" {
		p.Subject = e.Subject
	}
	return p
}

// KindOf returns the kind of problem that Explain says err is.
func KindOf(err error) ProblemKind {
	if p, ok := errors.AsType[*Problem](err); ok {
		return p.Kind
	}
	return kind(err)
}

// kind sorts err. The order matters where an error matches several: an
// outage comes first, a cancellation hides whatever it cut short, a token
// problem hides what the token was refused, and an action GitHub refused
// with a reason is rejected even when the refusal was a 403.
func kind(err error) ProblemKind {
	_, refused := errors.AsType[*RefusedError](err)
	_, invalid := errors.AsType[*InvalidQueryError](err)
	switch {
	// The client tags an error offline only while its context is live,
	// so what is tagged is an outage even if it also matches a deadline,
	// as the http.Client's own timeout does.
	case errors.Is(err, ErrOffline):
		return Offline
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return Canceled
	case errors.Is(err, ErrUnauthorized):
		return Auth
	case errors.Is(err, ErrRateLimited):
		return RateLimited
	case refused:
		return Rejected
	case errors.Is(err, ErrForbidden):
		return Forbidden
	case errors.Is(err, ErrNotFound):
		return NotFound
	case errors.Is(err, ErrConflict), invalid:
		return Rejected
	case errors.Is(err, ErrUnavailable):
		return Unavailable
	default:
		return Internal
	}
}
