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

// TestModalOnlineRetriesOnce checks that the open detail reads again, once
// GitHub answers again, the detail and the comments that failed for want
// of an answer, and that what GitHub refused, or what loaded, costs
// nothing.
func TestModalOnlineRetriesOnce(t *testing.T) {
	offline := fmt.Errorf("github: POST /graphql: %w", core.ErrOffline)
	refused := fmt.Errorf("github: 403 Forbidden: %w", core.ErrForbidden)
	tests := []struct {
		name                   string
		getErr, commentsErr    error
		wantGets, wantComments int
	}{
		{"both offline", offline, offline, 1, 1},
		{"detail offline", offline, nil, 1, 0},
		{"comments offline", nil, offline, 0, 1},
		{"refused", refused, refused, 0, 0},
		{"loaded", nil, nil, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService()
			svc.getErr, svc.commentsErr = tt.getErr, tt.commentsErr
			h := started(t, svc, 100, 30)
			press(t, h, "enter")
			if h.modal() == nil {
				t.Fatal("no modal open")
			}
			svc.mu.Lock()
			svc.getErr, svc.commentsErr = nil, nil
			gets, comments := len(svc.gets), len(svc.comments)
			// The page read last, which failed if any did.
			last := svc.comments[comments-1]
			svc.mu.Unlock()

			drain(t, h, h.Update(ui.OnlineMsg{}))
			drain(t, h, h.Update(ui.OnlineMsg{}))
			svc.mu.Lock()
			// Once the page loads, the thread reads on as it always does;
			// only the page that failed counts.
			gotGets, gotComments := len(svc.gets)-gets, 0
			for _, q := range svc.comments[comments:] {
				if q == last {
					gotComments++
				}
			}
			svc.mu.Unlock()
			if gotGets != tt.wantGets || gotComments != tt.wantComments {
				t.Errorf("after two OnlineMsg: %d reads of the detail and %d of the comments, want %d and %d",
					gotGets, gotComments, tt.wantGets, tt.wantComments)
			}
			if errors.Is(tt.commentsErr, core.ErrOffline) && h.modal().thread.Err() != nil {
				t.Errorf("comments still failed once online: %v", h.modal().thread.Err())
			}
		})
	}
}

// TestModalOnlineDuringRefresh checks that GitHub answering again while a
// refresh of a detail that failed is under way reads the detail once, by
// the refresh, rather than again.
func TestModalOnlineDuringRefresh(t *testing.T) {
	svc := newFakeService()
	svc.getErr = fmt.Errorf("github: POST /graphql: %w", core.ErrOffline)
	h := started(t, svc, 100, 30)
	press(t, h, "enter")
	if h.modal() == nil || h.modal().failed == nil {
		t.Fatal("want an open modal whose detail failed")
	}
	svc.mu.Lock()
	svc.getErr = nil
	gets := len(svc.gets)
	svc.mu.Unlock()

	// The refresh is under way, its command not run yet, as GitHub
	// answers again.
	refresh := h.Update(keyMsg("r"))
	drain(t, h, h.Update(ui.OnlineMsg{}))
	drain(t, h, refresh)
	svc.mu.Lock()
	got := len(svc.gets) - gets
	svc.mu.Unlock()
	if got != 1 {
		t.Errorf("reads of the detail = %d, want 1, by the refresh", got)
	}
	if m := h.modal(); m.failed != nil || !m.loaded {
		t.Errorf("failed %v, loaded %v; want the detail", m.failed, m.loaded)
	}
}
