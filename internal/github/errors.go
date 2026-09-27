package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
)

// Error is a failed API response. It unwraps to the core error that matches
// its status, so callers can test for it with errors.Is: a 401, or a 403
// for a scope the token lacks, matches core.ErrUnauthorized, another 403
// core.ErrForbidden, a 404 or 410 core.ErrNotFound, a 405, 409 or 422
// core.ErrConflict, and a 5xx core.ErrUnavailable. A rate limit unwraps to
// a *core.RateLimitError that says when to retry.
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
		switch {
		case missingScope(resp.Header):
			e.err = core.ErrUnauthorized
		case secondaryLimit(body.Message):
			// GitHub's docs say to wait a minute when a secondary limit
			// doesn't say how long.
			e.err = &core.RateLimitError{Reset: c.budget.now().Add(secondaryBackoff)}
		}
	}
	return e
}

// secondaryLimit reports whether the message of a 403 is GitHub's for a
// secondary rate limit, which may come without Retry-After or the
// rate-limit headers, such as "You have exceeded a secondary rate limit".
func secondaryLimit(msg string) bool {
	return strings.Contains(strings.ToLower(msg), "rate limit")
}

// missingScope reports whether a 403 is for a scope the token lacks.
// X-OAuth-Scopes lists the scopes of an OAuth or classic token, and
// X-Accepted-OAuth-Scopes those of which the endpoint needs one. Other
// tokens get neither, and many endpoints accept any token, so then it
// can't tell and says no.
func missingScope(h http.Header) bool {
	granted, ok := h[http.CanonicalHeaderKey("X-OAuth-Scopes")]
	accepted := scopes(h.Get("X-Accepted-OAuth-Scopes"))
	if !ok || len(accepted) == 0 {
		return false
	}
	have := scopes(strings.Join(granted, ","))
	return !slices.ContainsFunc(accepted, func(want string) bool {
		return slices.ContainsFunc(have, func(g string) bool { return coversScope(g, want) })
	})
}

// scopes splits a list of scopes such as "repo, read:org".
func scopes(list string) []string {
	var out []string
	for s := range strings.SplitSeq(list, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// coversScope reports whether a token with scope have may do what scope
// want allows. Scopes form a hierarchy: repo includes repo:status,
// public_repo, repo_deployment and admin:repo_hook, user includes
// read:user, and admin:org includes write:org, which includes read:org.
func coversScope(have, want string) bool {
	if have == want {
		return true
	}
	switch have {
	case "repo":
		switch want {
		case "public_repo", "security_events", "repo_deployment":
			return true
		}
		return strings.HasPrefix(want, "repo:") || coversScope("admin:repo_hook", want)
	case "user":
		return strings.HasPrefix(want, "user:") || want == "read:user"
	case "project":
		return want == "read:project"
	}
	if name, ok := strings.CutPrefix(have, "admin:"); ok {
		return want == "write:"+name || want == "read:"+name
	}
	if name, ok := strings.CutPrefix(have, "write:"); ok {
		return want == "read:"+name
	}
	return false
}

// oneLine puts s on one line without escape sequences or other control
// characters, so that GitHub's text can neither break the layout nor
// reach the terminal as a command.
func oneLine(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == utf8.RuneError || unicode.IsControl(r) {
			return ' '
		}
		return r
	}, ansi.Strip(s))
	return strings.Join(strings.Fields(s), " ")
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
