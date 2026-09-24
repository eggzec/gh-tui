package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestUnreachable(t *testing.T) {
	dial := &url.Error{Op: "Get", URL: "https://api.github.com/", Err: errors.New("connection refused")}
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"network", fmt.Errorf("list: %w", dial), true},
		{"server error", &Error{StatusCode: 502}, true},
		{"unavailable", &Error{StatusCode: 503}, true},
		{"not found", &Error{StatusCode: 404, err: core.ErrNotFound}, false},
		{"unauthorized", &Error{StatusCode: 401, err: core.ErrUnauthorized}, false},
		{"forbidden", &Error{StatusCode: 403}, false},
		{"rate limited", &Error{StatusCode: 429, err: &core.RateLimitError{}}, false},
		{"other", errors.New("decode response: unexpected EOF"), false},
	}
	for _, tt := range tests {
		if got := Unreachable(t.Context(), tt.err); got != tt.want {
			t.Errorf("Unreachable(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestUnreachableCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := &url.Error{Op: "Get", URL: "https://api.github.com/", Err: context.Canceled}
	if Unreachable(ctx, err) {
		t.Error("Unreachable after cancel = true, want false")
	}
}

func TestRefused(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"not found", fmt.Errorf("get: %w", &Error{StatusCode: 404, err: core.ErrNotFound}), true},
		{"graphql not found", &GraphQLError{causes: []error{core.ErrNotFound}}, true},
		{"unauthorized", &Error{StatusCode: 401, err: core.ErrUnauthorized}, true},
		{"forbidden", &Error{StatusCode: 403}, true},
		{"bare not found", &Error{StatusCode: 404}, true},
		{"rate limited", &Error{StatusCode: 403, err: &core.RateLimitError{}}, false},
		{"too many requests", &Error{StatusCode: 429, err: &core.RateLimitError{}}, false},
		{"server error", &Error{StatusCode: 502}, false},
		{"network", &url.Error{Op: "Get", URL: "https://api.github.com/", Err: errors.New("refused")}, false},
	}
	for _, tt := range tests {
		if got := Refused(tt.err); got != tt.want {
			t.Errorf("Refused(%s) = %v, want %v", tt.name, got, tt.want)
		}
	}
}
