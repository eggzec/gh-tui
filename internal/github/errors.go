package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Error is a failed API response. It unwraps to the core error that matches
// its status, so callers can test for it with errors.Is: a 401 matches
// core.ErrUnauthorized, a 403 core.ErrForbidden, a 404 or 410
// core.ErrNotFound, a 405, 409 or 422 core.ErrConflict, and a 5xx
// core.ErrUnavailable. A rate limit unwraps to a *core.RateLimitError that
// says when to retry, a 403 for a scope the token lacks, or a refusal to
// change a workflow for want of the workflow scope, to a *core.ScopeError,
// which matches core.ErrUnauthorized, and a 403 of an organization's SSO
// to a *core.SSOError.
type Error struct {
	StatusCode int
	// Message is GitHub's explanation, including field errors.
	Message string
	// err refines what the status alone says, such as a 403 that is a
	// rate limit. Without it, Unwrap goes by the status.
	err error
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("github: %d %s", e.StatusCode, msg)
}

// Reason returns GitHub's explanation on one line, safe to show in a
// terminal, or "" if it gave none. It isn't cut to any length; whatever
// shows it truncates it to fit.
func (e *Error) Reason() string {
	return oneLine(e.Message)
}

// Unwrap returns the matching core error, or nil.
func (e *Error) Unwrap() error {
	if e.err != nil {
		return e.err
	}
	switch {
	case e.StatusCode == http.StatusUnauthorized:
		return core.ErrUnauthorized
	case e.StatusCode == http.StatusForbidden:
		return core.ErrForbidden
	case e.StatusCode == http.StatusNotFound, e.StatusCode == http.StatusGone:
		return core.ErrNotFound
	// 405 is how REST refuses to merge a pull request that isn't mergeable.
	case e.StatusCode == http.StatusMethodNotAllowed, e.StatusCode == http.StatusConflict,
		e.StatusCode == http.StatusUnprocessableEntity:
		return core.ErrConflict
	case e.StatusCode >= http.StatusInternalServerError:
		return core.ErrUnavailable
	}
	return nil
}

// Unreachable reports whether err means that GitHub couldn't be reached
// (core.ErrOffline) or failed on its side, with a 5xx
// (core.ErrUnavailable), rather than refused the request. Only then may a
// caller fall back to what it read earlier: an account that lost access
// gets a 401, 403 or 404 and must not see what it cached before. An error
// after ctx is done is a cancellation, not an outage.
func Unreachable(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	return errors.Is(err, core.ErrOffline) || errors.Is(err, core.ErrUnavailable)
}

// Refused reports whether GitHub refused access to what was asked
// (core.ErrUnauthorized or core.ErrForbidden, as for a 401 or a 403 that
// isn't a rate limit), or said it isn't there (core.ErrNotFound, as for a
// 404, or a 410 for a deleted issue or the issues of a repository that
// turned them off). What was cached of it should then go, since the
// account may have lost access, or there is nothing left to show.
func Refused(err error) bool {
	return errors.Is(err, core.ErrUnauthorized) || errors.Is(err, core.ErrForbidden) ||
		errors.Is(err, core.ErrNotFound)
}

// offline tags err, the error of a request that got no response, with
// core.ErrOffline, unless it is because ctx is done, which is a
// cancellation, or a rate limit that stopped the request before it was
// sent: neither is an outage.
func offline(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, core.ErrRateLimited) {
		return err
	}
	return fmt.Errorf("%w: %w", core.ErrOffline, err)
}

// httpError builds an *Error from a response with an error status.
func (c *Client) httpError(resp *http.Response) error {
	var body apiError
	// The body is only for the message, so a malformed one is not an error.
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body)
	e := &Error{StatusCode: resp.StatusCode, Message: body.String()}
	switch resp.StatusCode {
	case http.StatusForbidden, http.StatusTooManyRequests:
		if reset, ok := c.rateLimitReset(resp); ok {
			e.err = &core.RateLimitError{Reset: reset}
			break
		}
		if resp.StatusCode != http.StatusForbidden {
			break
		}
		if u, ok := ssoRequired(resp.Header.Values("X-GitHub-SSO")); ok {
			e.err = &core.SSOError{URL: c.ssoLink(u)}
			break
		}
		switch missing := missingScopes(resp.Header); {
		case len(missing) > 0:
			e.err = &core.ScopeError{Scopes: missing}
		case secondaryLimit(body.Message):
			// GitHub's docs say to wait a minute when a secondary limit
			// doesn't say how long, or longer when it comes again.
			e.err = &core.RateLimitError{Reset: c.budget.liftsAt(c.budget.now().Add(secondaryBackoff))}
		}
	}
	if e.err == nil && resp.StatusCode < http.StatusInternalServerError {
		e.err = workflowRefusal(body.Message)
	}
	// A fine-grained or App token is told the permissions it lacks, which
	// no scope grants, so they only explain.
	if needs := permissionsNeeded(resp.Header.Get("X-Accepted-GitHub-Permissions")); needs != "" {
		e.Message = strings.TrimSpace(e.Message + " (needs " + needs + ")")
	}
	return e
}

// secondaryLimit reports whether the message of a 403 is GitHub's for a
// secondary rate limit, which may come without Retry-After or the
// rate-limit headers, such as "You have exceeded a secondary rate limit".
func secondaryLimit(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "rate limit")
}

// missingScopes returns the scopes of which a 403 needs one that the
// token lacks, the one to grant first, or nil if it isn't for a scope.
// X-OAuth-Scopes lists the scopes of an OAuth or classic token, and
// X-Accepted-OAuth-Scopes those of which the endpoint needs one. Other
// tokens get neither, and many endpoints accept any token, so then it
// can't tell and says nil.
func missingScopes(h http.Header) []string {
	granted, ok := h[http.CanonicalHeaderKey("X-OAuth-Scopes")]
	accepted := core.ParseScopes(h.Get("X-Accepted-OAuth-Scopes"))
	if !ok || len(accepted) == 0 {
		return nil
	}
	have := core.ParseScopes(strings.Join(granted, ","))
	if slices.ContainsFunc(accepted, func(want string) bool {
		return slices.ContainsFunc(have, func(g string) bool { return core.Covers(g, want) })
	}) {
		return nil
	}
	return core.SortScopes(accepted)
}

// insufficientScopes are the scopes that GraphQL's INSUFFICIENT_SCOPES
// says a field requires one of, as in "The 'teams' field requires one of
// the following scopes: ['read:org'], but your token has only been
// granted the: ['repo'] scopes."
var insufficientScopes = regexp.MustCompile(`following scopes: \[([^\]]*)\]`)

// scopesRequired returns the scopes, the one to grant first, that msg, an
// INSUFFICIENT_SCOPES message, says are required, or nil if it names none.
func scopesRequired(msg string) []string {
	m := insufficientScopes.FindStringSubmatch(msg)
	if m == nil {
		return nil
	}
	return core.SortScopes(core.ParseScopes(strings.NewReplacer("'", "", `"`, "").Replace(m[1])))
}

// workflowRefusal returns what GitHub's msg means when it refuses to
// change a workflow file, as merging a pull request that changes one
// does: a scope an OAuth or classic token lacks, as in "refusing to allow
// an OAuth App to create or update workflow `.github/workflows/ci.yml`
// without `workflow` scope", or a permission a GitHub App lacks ("…
// without `workflows` permission"). It returns nil for any other msg.
func workflowRefusal(msg string) error {
	m := strings.NewReplacer("`", "", "'", "", `"`, "").Replace(strings.ToLower(msg))
	if !strings.Contains(m, "refusing to allow") {
		return nil
	}
	switch {
	case strings.Contains(m, "without workflow scope"):
		return &core.ScopeError{Scopes: []string{"workflow"}}
	case strings.Contains(m, "without workflows permission"):
		return core.ErrForbidden
	}
	return nil
}

// permissionsNeeded says the permissions of X-Accepted-GitHub-Permissions,
// such as "pull_requests=write,contents=read", which GitHub sends when it
// refuses a fine-grained or App token, as "Pull requests: write, Contents:
// read". Sets that would each do, separated by semicolons, are joined by
// "or". It returns "" for a header it can't read, or one longer than any
// GitHub sends.
func permissionsNeeded(header string) string {
	if len(header) > maxPermissions {
		return ""
	}
	var sets []string
	for set := range strings.SplitSeq(header, ";") {
		var perms []string
		for p := range strings.SplitSeq(set, ",") {
			name, level, ok := strings.Cut(strings.TrimSpace(p), "=")
			if !ok || !lowerWord(name) || !lowerWord(level) {
				return ""
			}
			name = strings.ReplaceAll(name, "_", " ")
			perms = append(perms, strings.ToUpper(name[:1])+name[1:]+": "+level)
		}
		sets = append(sets, strings.Join(perms, ", "))
	}
	return strings.Join(sets, " or ")
}

// maxPermissions is the most of X-Accepted-GitHub-Permissions read, far
// more than GitHub's, which name a few permissions.
const maxPermissions = 1 << 10

// lowerWord reports whether s is a word of lower-case letters and
// underscores, as the names and levels of permissions are.
func lowerWord(s string) bool {
	return s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyz_") == ""
}

// ssoLink returns u, the URL where GitHub says to authorize the token for
// an organization's SSO, if it is a page of the GitHub the client talks
// to, since the user may be sent to open it, or "". It never names a user.
func (c *Client) ssoLink(u string) string {
	link, err := url.Parse(u)
	if err != nil || link.Scheme != "https" && link.Scheme != c.restURL.Scheme {
		return ""
	}
	web := &url.URL{Scheme: link.Scheme, Host: c.WebHost()}
	if !strings.EqualFold(link.Hostname(), web.Hostname()) || port(link) != port(web) {
		return ""
	}
	link.User = nil
	return link.String()
}

// port returns the port of u, or its scheme's when it names none.
func port(u *url.URL) string {
	if p := u.Port(); p != "" {
		return p
	}
	switch u.Scheme {
	case "https":
		return "443"
	case "http":
		return "80"
	}
	return ""
}

// oneLine puts s on one line without escape sequences, control characters
// or the kitty image placeholder, and with single spaces, so that
// GitHub's text can neither break the layout nor reach the terminal as a
// command.
func oneLine(s string) string {
	return strings.Join(strings.Fields(termtext.OneLine(s)), " ")
}

// apiError is the body GitHub sends with an error status.
type apiError struct {
	Message string `json:"message"`
	// Errors holds field errors. GitHub sends them as objects or as plain
	// strings, so they are decoded one by one.
	Errors []json.RawMessage `json:"errors"`
}

func (a apiError) String() string {
	details := make([]string, 0, len(a.Errors))
	for _, raw := range a.Errors {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			details = append(details, s)
			continue
		}
		var d struct {
			Field   string `json:"field"`
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &d) != nil {
			continue
		}
		switch {
		case d.Message != "":
			details = append(details, d.Message)
		case d.Code != "":
			details = append(details, strings.TrimSpace(d.Field+" "+d.Code))
		}
	}
	if len(details) == 0 {
		return a.Message
	}
	return fmt.Sprintf("%s (%s)", a.Message, strings.Join(details, "; "))
}
