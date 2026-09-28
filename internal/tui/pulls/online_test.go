package pulls

import (
	"errors"
	"fmt"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// TestOnlineRetriesOnce checks that GitHub answering again reads once the
// list that failed for want of it, and that a list that loaded, or that
// GitHub refused, costs nothing.
func TestOnlineRetriesOnce(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"offline", fmt.Errorf("list pulls: %w", core.ErrOffline), 1},
		{"server error", fmt.Errorf("list pulls: %w", core.ErrUnavailable), 1},
		{"refused", fmt.Errorf("list pulls: %w", core.ErrNotFound), 0},
		{"loaded", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			svc.listErr = tt.err
			h := started(t, svc, 80, 20)
			svc.mu.Lock()
			svc.listErr = nil
			svc.mu.Unlock()

			n := len(svc.requested())
			drain(t, h, h.Update(ui.OnlineMsg{}))
			drain(t, h, h.Update(ui.OnlineMsg{}))
			if got := len(svc.requested()) - n; got != tt.want {
				t.Errorf("lists read after two OnlineMsg = %d, want %d", got, tt.want)
			}
			if tt.err != nil && !errors.Is(tt.err, core.ErrNotFound) && h.feed.Err() != nil {
				t.Errorf("Err() = %v once online, want the list", h.feed.Err())
			}
		})
	}
}
