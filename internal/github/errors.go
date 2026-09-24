package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// Error is a failed API response. It unwraps to the core error that matches
// its status, if any, so callers can test for it with errors.Is. A rate limit
// unwraps to a *core.RateLimitError that says when to retry.
type Error struct {
	StatusCode int
	// Message is GitHub's explanation, including field errors.
	Message string
	err     error
}

func (e *Error) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.StatusCode)
	}
	return fmt.Sprintf("github: %d %s", e.StatusCode, msg)
}

// Unwrap returns the matching core error, or nil.
func (e *Error) Unwrap() error {
	return e.err
}

// Unreachable reports whether err means that GitHub couldn't be reached or
// failed on its side, with a 5xx, rather than refused the request. Only then
// may a caller fall back to what it read earlier: an account that lost
// access gets a 401, 403 or 404 and must not see what it cached before. An
// error after ctx is done is a cancellation, not an outage.
func Unreachable(ctx context.Context, err error) bool {
	if err == nil || ctx.Err() != nil {
		return false
	}
	if e, ok := errors.AsType[*Error](err); ok {
		return e.StatusCode >= http.StatusInternalServerError
	}
	_, ok := errors.AsType[*url.Error](err)
	return ok
}

// Refused reports whether GitHub refused access to what was asked, with a
// 401, a 404, or a 403 that isn't a rate limit. What was cached of it
// should then go, since the account may have lost access.
func Refused(err error) bool {
	if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrUnauthorized) {
		return true
	}
	e, ok := errors.AsType[*Error](err)
	if !ok {
		return false
	}
	switch e.StatusCode {
	case http.StatusUnauthorized, http.StatusNotFound:
		return true
	case http.StatusForbidden:
		return !errors.Is(err, core.ErrRateLimited)
	}
	return false
}

// httpError builds an *Error from a response with an error status.
func (c *Client) httpError(resp *http.Response) error {
	var body apiError
	// The body is only for the message, so a malformed one is not an error.
	_ = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body)
	e := &Error{StatusCode: resp.StatusCode, Message: body.String()}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		e.err = core.ErrUnauthorized
	case http.StatusNotFound:
		e.err = core.ErrNotFound
	// 405 is how REST refuses to merge a pull request that isn't mergeable.
	case http.StatusMethodNotAllowed, http.StatusConflict, http.StatusUnprocessableEntity:
		e.err = core.ErrConflict
	case http.StatusForbidden, http.StatusTooManyRequests:
		if reset, ok := c.rateLimitReset(resp); ok {
			e.err = &core.RateLimitError{Reset: reset}
		}
	}
	return e
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
