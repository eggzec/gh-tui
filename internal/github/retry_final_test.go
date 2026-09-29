package github

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"testing/synctest"
)

// TestRetryNotUnsendable checks that a read that can't be sent as it is,
// whatever the attempt, is sent once, while a connection reset still is
// sent again.
func TestRetryNotUnsendable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"bad header", fmt.Errorf("net/http: invalid header field value for %q", "X"), 1},
		{"proxy wants credentials", errors.New(http.StatusText(http.StatusProxyAuthRequired)), 1},
		{"no scheme", errors.New(`unsupported protocol scheme ""`), 1},
		{"no host", errors.New("http: no Host in request URL"), 1},
		{"connection reset", errReset, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := &script{steps: []step{{err: tt.err}}}
				if err := get(t.Context(), scriptedClient(t, s)); err == nil {
					t.Error("the read succeeded")
				}
				if n, _ := s.attempts(); n != tt.want {
					t.Errorf("%d attempts, want %d", n, tt.want)
				}
			})
		})
	}
}
