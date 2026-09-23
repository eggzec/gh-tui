package github

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// Error is a failed API response. It unwraps to the core error that matches
// its status, if any, so callers can test for it with errors.Is.
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
	case http.StatusConflict, http.StatusUnprocessableEntity:
		e.err = core.ErrConflict
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
