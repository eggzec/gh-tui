package uitest

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// GitHubError stands for an error of the github package, whose text names
// the request and its status, with GitHub's own reason.
type GitHubError struct {
	Is  error
	Why string
}

func (e *GitHubError) Error() string {
	return "github: PUT /repos/eggzec/gh-tui/pulls/42/merge: 405 " + e.Why
}

// Unwrap returns the sentinel the error matches.
func (e *GitHubError) Unwrap() error { return e.Is }

// Reason returns GitHub's reason, as the github package's errors do.
func (e *GitHubError) Reason() string { return e.Why }

// Failure is an error of a kind, and the cause a toast gives for it.
type Failure struct {
	Name string
	Err  error
	// Cause is what a toast says of Err after "Couldn't <action>: ", for a
	// failure about subject, such as "eggzec/gh-tui#42"; "" means no toast.
	Cause func(subject string) string
}

// ToastLog is the log file that ToastVoice points to.
const ToastLog = "~/.local/state/gh-tui/gh-tui.log"

// toastNow is the clock of ToastVoice.
var toastNow = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

// ToastVoice returns the voice of the default keys, telling times in UTC
// at noon, pointing to ToastLog, with a classic token that has the repo
// scope and whose hints name :auth.
func ToastVoice() ui.Voice {
	v := ui.NewVoice(config.Default().Keys, "")
	v.Log, v.Loc, v.Now = ToastLog, time.UTC, func() time.Time { return toastNow }
	v.Token = Token(&Checker{A: Classic("repo")})
	return v
}

// Failures are errors of every kind that GitHub or the app may fail with,
// each with a chain the user must never see, and what a toast in the words
// of ToastVoice says of them.
func Failures() []Failure {
	offline := &url.Error{Op: "Put", URL: "https://api.github.com/repos/eggzec/gh-tui", Err: errors.New("dial tcp: no route to host")}
	fixed := func(cause string) func(string) string { return func(string) string { return cause } }
	return []Failure{
		{Name: "offline", Err: fmt.Errorf("github: %w: %w", core.ErrOffline, offline), Cause: fixed("can't reach GitHub.")},
		{Name: "unavailable", Err: &GitHubError{Is: core.ErrUnavailable}, Cause: fixed("GitHub isn't responding.")},
		{
			Name: "rate limited", Err: fmt.Errorf("github: 403: %w", &core.RateLimitError{Reset: toastNow.Add(time.Hour)}),
			Cause: fixed("rate limited until 13:00."),
		},
		{
			Name: "auth", Err: &GitHubError{Is: core.ErrUnauthorized, Why: "Bad credentials"},
			Cause: fixed("GitHub rejected the token. Run gh auth login, then :auth."),
		},
		{
			Name: "scope", Err: &core.ScopeError{Scopes: []string{"workflow"}, Err: &GitHubError{Is: core.ErrUnauthorized, Why: "Requires workflow scope"}},
			Cause: fixed("the token lacks the workflow scope · :auth to grant it."),
		},
		{
			Name: "forbidden", Err: &GitHubError{Is: core.ErrForbidden, Why: "Resource not accessible"},
			Cause: func(s string) string {
				repo, _, _ := strings.Cut(s, "#")
				return "you don't have access to " + repo + "."
			},
		},
		{
			Name: "sso", Err: &GitHubError{Is: core.ErrForbidden, Why: "Resource protected by organization SAML enforcement"},
			Cause: func(s string) string {
				org, _, _ := strings.Cut(s, "/")
				return org + " requires SSO · :auth to authorize the token."
			},
		},
		{
			Name: "not found", Err: &GitHubError{Is: core.ErrNotFound, Why: "Not Found"},
			Cause: func(s string) string { return s + " doesn't exist or is private." },
		},
		{Name: "rejected", Err: &GitHubError{Is: core.ErrConflict, Why: "Pull Request is not mergeable"}, Cause: fixed("Pull Request is not mergeable.")},
		{Name: "internal", Err: errors.New("github: decode 200: unexpected EOF"), Cause: fixed("something went wrong, see " + ToastLog + ".")},
		{Name: "canceled", Err: fmt.Errorf("github: PUT /repos: %w", context.Canceled), Cause: fixed("")},
	}
}

// Toast returns the toast the app shows in the words of ToastVoice when
// err stopped what, in a toast wide enough for any of them.
func Toast(what string, err error) string {
	return ui.SayToast(core.Explain(what, err), ToastVoice(), func(s string) bool { return len(s) <= ui.SayWidth })
}

// leaks are what a toast must never show of an error: the package,
// requests, paths, status codes and what the transport said.
var leaks = []string{"github:", "PUT", "GET", "/repos", "405", "403", "200", "api.github.com", "dial tcp", "EOF", "canceled"}

// CheckToast fails t unless text is the toast that f gives when it
// stopped action on subject, and shows nothing of the error's chain.
func CheckToast(t *testing.T, f Failure, action, subject, text string) {
	t.Helper()
	want := ""
	if cause := f.Cause(subject); cause != "" {
		want = "Couldn't " + action + ": " + cause
	}
	if text != want {
		t.Errorf("toast %q, want %q", text, want)
	}
	for _, l := range leaks {
		if strings.Contains(text, l) {
			t.Errorf("toast shows %q: %s", l, text)
		}
	}
}
