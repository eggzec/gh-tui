package dashboard

import (
	"fmt"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// callsSince returns the reads the fake was asked for after the first n.
func (f *fakeService) callsSince(n int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls[n:])
}

func (f *fakeService) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// TestOnlineRetriesOnce checks that GitHub answering again reads once
// what failed for want of it, and nothing that GitHub refused or that
// loaded.
func TestOnlineRetriesOnce(t *testing.T) {
	svc := newFake()
	offline := fmt.Errorf("github: GET /user: %w", core.ErrOffline)
	svc.fail["header"] = offline
	svc.fail["repos @me@"] = offline
	svc.fail["work"] = fmt.Errorf("github: 404: %w", core.ErrNotFound)
	s := newSection(t, svc, nil, 140, 38)
	clear(svc.fail)

	n := svc.callCount()
	run(t, s, s.Update(ui.OnlineMsg{}))
	got := svc.callsSince(n)
	slices.Sort(got)
	// The header lists the organizations, whose tabs read nothing until
	// shown.
	if want := []string{"header", "repos @me@"}; !slices.Equal(got, want) {
		t.Errorf("reads once online = %q, want %q", got, want)
	}

	n = svc.callCount()
	run(t, s, s.Update(ui.OnlineMsg{}))
	if got := svc.callsSince(n); len(got) != 0 {
		t.Errorf("reads when nothing failed = %q, want none", got)
	}
}

// TestOnlineReadsKeptValues checks that what was served from what an
// earlier read kept, offline or rate limited, is read again once GitHub
// answers or the limit lifts, once. The fake serves the calendar as read.
func TestOnlineReadsKeptValues(t *testing.T) {
	for _, limited := range []bool{false, true} {
		t.Run(map[bool]string{false: "offline", true: "limited"}[limited], func(t *testing.T) {
			svc := newFake()
			svc.offline, svc.limited = !limited, limited
			s := newSection(t, svc, nil, 140, 38)
			svc.mu.Lock()
			svc.offline, svc.limited = false, false
			svc.mu.Unlock()

			n := svc.callCount()
			run(t, s, s.Update(ui.OnlineMsg{}))
			got := svc.callsSince(n)
			slices.Sort(got)
			if want := []string{"header", "work"}; !slices.Equal(got, want) {
				t.Errorf("reads once online = %q, want %q", got, want)
			}
			if s.offlineNow() || s.limitedNow() {
				t.Error("still kept after reading again")
			}
			n = svc.callCount()
			run(t, s, s.Update(ui.OnlineMsg{}))
			if got := svc.callsSince(n); len(got) != 0 {
				t.Errorf("reads on the next OnlineMsg = %q, want none", got)
			}
		})
	}
}
