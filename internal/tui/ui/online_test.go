package ui

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestUnreached(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{fmt.Errorf("list pulls: %w", core.ErrOffline), true},
		{fmt.Errorf("list pulls: %w", core.ErrUnavailable), true},
		{fmt.Errorf("list pulls: %w", &core.RateLimitError{}), true},
		{fmt.Errorf("list pulls: %w", core.ErrNotFound), false},
		{fmt.Errorf("list pulls: %w", core.ErrForbidden), false},
	}
	for _, tt := range tests {
		if got := Unreached(tt.err); got != tt.want {
			t.Errorf("Unreached(%v) = %t, want %t", tt.err, got, tt.want)
		}
	}
}

// retrier is a Retrier that counts its retries, and, inside a keptList,
// its reads of what it was served kept, which kept says it has.
type retrier struct {
	err            error
	kept           bool
	retries, reads int
}

func (r *retrier) Err() error { return r.err }

func (r *retrier) Retry() tea.Cmd {
	r.retries++
	return func() tea.Msg { return nil }
}

// keptList is a retrier that also reads again what it was served kept.
type keptList struct{ retrier }

func (r *keptList) RetryKept() tea.Cmd {
	if !r.kept {
		return nil
	}
	r.reads++
	return func() tea.Msg { return nil }
}

func TestRetryUnreached(t *testing.T) {
	offline := fmt.Errorf("list: %w", core.ErrOffline)
	tests := []struct {
		name           string
		r              Retrier
		retries, reads int
	}{
		{"failed", &retrier{err: offline}, 1, 0},
		{"refused", &retrier{err: core.ErrNotFound}, 0, 0},
		{"loaded", &retrier{}, 0, 0},
		{"kept", &keptList{retrier{kept: true}}, 0, 1},
		{"failed and kept", &keptList{retrier{err: offline, kept: true}}, 1, 1},
		{"fresh", &keptList{}, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := RetryUnreached(tt.r)
			var r *retrier
			switch v := tt.r.(type) {
			case *retrier:
				r = v
			case *keptList:
				r = &v.retrier
			}
			if r.retries != tt.retries || r.reads != tt.reads {
				t.Errorf("retries %d, kept reads %d; want %d, %d", r.retries, r.reads, tt.retries, tt.reads)
			}
			if (cmd != nil) != (tt.retries+tt.reads > 0) {
				t.Errorf("cmd = %v, want one only if something is read again", cmd)
			}
		})
	}
}
