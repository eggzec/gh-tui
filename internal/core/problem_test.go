package core

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"
)

// apiError stands in for an error of the github package: it gives GitHub's
// reason and unwraps to the sentinel that tags it.
type apiError struct {
	msg string
	tag error
}

func (e *apiError) Error() string  { return "github: " + e.msg }
func (e *apiError) Reason() string { return e.msg }
func (e *apiError) Unwrap() error  { return e.tag }

func TestExplain(t *testing.T) {
	reset := time.Date(2026, 9, 27, 14, 5, 0, 0, time.UTC)
	dial := &url.Error{Op: "Get", URL: "https://api.github.com/user", Err: errors.New("dial tcp: connection refused")}
	sso := &apiError{"Resource protected by organization SAML enforcement.", ErrForbidden}
	tests := []struct {
		name    string
		err     error
		kind    ProblemKind
		reason  string
		reset   time.Time
		subject string
		grant   string
		sso     bool
		ssoURL  string
	}{
		{name: "plain", err: errors.New("decode response: unexpected EOF"), kind: Internal},
		{name: "canceled", err: fmt.Errorf("list pulls: %w", &url.Error{Op: "Get", Err: context.Canceled}), kind: Canceled},
		{name: "deadline", err: fmt.Errorf("list pulls: %w", context.DeadlineExceeded), kind: Canceled},
		{name: "client timeout", err: fmt.Errorf("%w: %w", ErrOffline, &url.Error{Op: "Get", Err: fmt.Errorf("timeout: %w", context.DeadlineExceeded)}), kind: Offline},
		{name: "offline", err: fmt.Errorf("viewer header: %w", fmt.Errorf("%w: %w", ErrOffline, dial)), kind: Offline},
		{name: "unavailable", err: fmt.Errorf("a: %w", fmt.Errorf("b: %w", &apiError{"Bad Gateway", ErrUnavailable})), kind: Unavailable, reason: "Bad Gateway"},
		{name: "unauthorized", err: fmt.Errorf("a: %w", &apiError{"Bad credentials", ErrUnauthorized}), kind: Auth, reason: "Bad credentials"},
		{name: "rate limit", err: fmt.Errorf("a: %w", fmt.Errorf("b: %w", &apiError{"API rate limit exceeded", &RateLimitError{Reset: reset}})), kind: RateLimited, reason: "API rate limit exceeded", reset: reset},
		{name: "bare rate limit", err: &RateLimitError{Reset: reset}, kind: RateLimited, reset: reset},
		{name: "forbidden", err: fmt.Errorf("list issues: %w", fmt.Errorf("get: %w", sso)), kind: Forbidden, reason: sso.msg},
		{name: "not found", err: fmt.Errorf("get repo: %w", ErrNotFound), kind: NotFound},
		{
			name:    "no number",
			err:     fmt.Errorf("kind of eggzec/gh-tui#5: %w", &NoNumberError{Repo: RepoRef{Owner: "eggzec", Name: "gh-tui"}, Number: 5, Err: &apiError{"This issue was deleted", ErrNotFound}}),
			kind:    NotFound,
			reason:  "This issue was deleted",
			subject: "eggzec/gh-tui#5",
		},
		{name: "conflict", err: fmt.Errorf("merge #5: %w", &apiError{"Pull Request is not mergeable", ErrConflict}), kind: Rejected, reason: "Pull Request is not mergeable"},
		{
			name:   "refused as forbidden",
			err:    &RefusedError{Action: "run can't be re-run", Reason: "raw", Err: &apiError{"Unable to re-run this workflow run because it was created over a month ago", ErrForbidden}},
			kind:   Rejected,
			reason: "Unable to re-run this workflow run because it was created over a month ago",
		},
		{name: "refused without a reasoner", err: &RefusedError{Action: "run can't be cancelled", Reason: "Cannot cancel a workflow run that is completed.", Err: ErrConflict}, kind: Rejected, reason: "Cannot cancel a workflow run that is completed."},
		{name: "refused for the token", err: &RefusedError{Action: "run can't be re-run", Err: &apiError{"Requires workflow scope", ErrUnauthorized}}, kind: Auth, reason: "Requires workflow scope"},
		{name: "invalid query", err: fmt.Errorf("search code: %w", &InvalidQueryError{Reason: "unknown qualifier lang"}), kind: Rejected, reason: "unknown qualifier lang"},
		{name: "rate limit over forbidden", err: errors.Join(ErrForbidden, &RateLimitError{Reset: reset}), kind: RateLimited, reset: reset},
		{name: "auth over not found", err: errors.Join(ErrNotFound, ErrUnauthorized), kind: Auth},
		{
			name:   "scope from GitHub",
			err:    fmt.Errorf("merge #5: %w", &apiError{"Resource not accessible by personal access token", &ScopeError{Scopes: []string{"workflow", "repo"}}}),
			kind:   Auth,
			reason: "Resource not accessible by personal access token",
			grant:  "workflow",
		},
		{name: "scope known before asking", err: fmt.Errorf("re-run: %w", &ScopeError{Scopes: []string{"repo"}}), kind: Auth, reason: "needs the repo scope", grant: "repo"},
		{name: "scope unnamed", err: &ScopeError{}, kind: Auth},
		{
			name:   "sso",
			err:    fmt.Errorf("list issues: %w", &apiError{sso.msg, &SSOError{URL: "https://github.com/orgs/eggzec/sso?authorization_request=x"}}),
			kind:   Forbidden,
			reason: sso.msg,
			sso:    true,
			ssoURL: "https://github.com/orgs/eggzec/sso?authorization_request=x",
		},
		{name: "sso without a URL", err: &SSOError{Err: sso}, kind: Forbidden, reason: sso.msg, sso: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Explain("load it", tt.err)
			if p.Kind != tt.kind || p.Reason != tt.reason || !p.Reset.Equal(tt.reset) || p.Subject != tt.subject ||
				p.Grant != tt.grant || p.SSO != tt.sso || p.SSOURL != tt.ssoURL {
				t.Errorf("Explain(%v) = {%v %q %v %q %q %v %q}, want {%v %q %v %q %q %v %q}", tt.err,
					p.Kind, p.Reason, p.Reset, p.Subject, p.Grant, p.SSO, p.SSOURL,
					tt.kind, tt.reason, tt.reset, tt.subject, tt.grant, tt.sso, tt.ssoURL)
			}
			if k := KindOf(tt.err); k != tt.kind {
				t.Errorf("KindOf(%v) = %v, want %v", tt.err, k, tt.kind)
			}
			if p.Action != "load it" || p.Err != tt.err { //nolint:errorlint // Explain keeps the very error it was given.
				t.Errorf("Explain kept action %q and err %v, want the ones given", p.Action, p.Err)
			}
		})
	}
}

func TestExplainNil(t *testing.T) {
	if p := Explain("load it", nil); p != nil {
		t.Errorf("Explain(nil) = %v, want nil", p)
	}
}

func TestExplainIdempotent(t *testing.T) {
	p := Explain("load your profile", fmt.Errorf("viewer: %w", ErrUnauthorized))
	if again := Explain("load your profile", p); again != p {
		t.Errorf("Explain(*Problem) = %p, want the same %p", again, p)
	}
	if wrapped := Explain("show the dashboard", fmt.Errorf("dashboard: %w", p)); wrapped != p {
		t.Errorf("Explain(wrapped *Problem) = %+v, want the one it wraps", wrapped)
	}
	// A problem keeps its kind, whatever else the chain matches.
	p = &Problem{Kind: Forbidden, Err: ErrOffline}
	if k := KindOf(fmt.Errorf("dashboard: %w", p)); k != Forbidden {
		t.Errorf("KindOf(wrapped *Problem) = %v, want %v", k, Forbidden)
	}
}

func TestProblemChain(t *testing.T) {
	cause := &RateLimitError{Reset: time.Unix(0, 0)}
	err := fmt.Errorf("list pulls: %w", fmt.Errorf("graphql: %w", cause))
	p := Explain("load pull requests", err)
	if !errors.Is(p, ErrRateLimited) {
		t.Error("errors.Is(problem, ErrRateLimited) = false, want true")
	}
	if got, ok := errors.AsType[*RateLimitError](p); !ok || got != cause {
		t.Errorf("errors.As(problem) = %v, want the rate limit it wraps", got)
	}
	if got, want := p.Error(), "load pull requests: "+err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if got := (&Problem{Err: err}).Error(); got != err.Error() {
		t.Errorf("Error() without an action = %q, want %q", got, err.Error())
	}
}

func TestScopeError(t *testing.T) {
	cause := &apiError{"Resource not accessible", ErrForbidden}
	err := &ScopeError{Scopes: []string{"workflow"}, Err: cause}
	if !errors.Is(err, ErrUnauthorized) || !errors.Is(err, ErrForbidden) {
		t.Errorf("%v matches neither ErrUnauthorized nor what it wraps", err)
	}
	if got, want := err.Error(), "the token lacks the workflow scope: github: Resource not accessible"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if !errors.Is(&SSOError{}, ErrForbidden) {
		t.Error("SSOError doesn't match ErrForbidden")
	}
}

func TestProblemKindString(t *testing.T) {
	for k := Internal; k <= Rejected; k++ {
		if k != Internal && k.String() == Internal.String() {
			t.Errorf("%d has no name of its own", k)
		}
	}
}
