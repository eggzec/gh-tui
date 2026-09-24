package issues

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestSetState(t *testing.T) {
	tests := []struct {
		name string
		// setup brings the section to the issue to act on.
		setup   func(t *testing.T, s *host)
		key     string
		sendErr error
		// wantChange is the change asked of the service, or "" for none.
		wantChange string
		// wantBefore and wantAfter are the states shown before and after
		// GitHub answers, or "" if the issue isn't shown.
		wantBefore, wantAfter core.State
	}{
		{
			name:       "close in the open list drops the issue once confirmed",
			setup:      func(t *testing.T, s *host) { t.Helper(); press(t, s, "down") },
			key:        "x",
			wantChange: "close 999",
			wantBefore: core.StateClosed,
			wantAfter:  "",
		},
		{
			// All issues, so the closed one stays in the list.
			name:       "close in the list",
			setup:      func(t *testing.T, s *host) { t.Helper(); press(t, s, "f", "f", "down") },
			key:        "x",
			wantChange: "close 999",
			wantBefore: core.StateClosed,
			wantAfter:  core.StateClosed,
		},
		{
			name: "reopen in the list",
			setup: func(t *testing.T, s *host) {
				t.Helper()
				press(t, s, "f", "f", "down", "down", "down", "down")
			},
			key:        "X",
			wantChange: "reopen 996",
			wantBefore: core.StateOpen,
			wantAfter:  core.StateOpen,
		},
		{
			name:       "close in the modal",
			setup:      func(t *testing.T, s *host) { t.Helper(); press(t, s, "down", "enter") },
			key:        "x",
			wantChange: "close 999",
			wantBefore: core.StateClosed,
			wantAfter:  core.StateClosed,
		},
		{
			name:       "a refused close rolls back",
			setup:      func(t *testing.T, s *host) { t.Helper(); press(t, s, "down", "enter") },
			key:        "x",
			sendErr:    errors.New("403 Forbidden"),
			wantChange: "close 999",
			wantBefore: core.StateClosed,
			wantAfter:  core.StateOpen,
		},
		{
			name:  "reopen an open issue does nothing",
			setup: func(t *testing.T, s *host) { t.Helper(); press(t, s, "down") },
			key:   "X",
		},
		{
			name: "close a closed issue does nothing",
			setup: func(t *testing.T, s *host) {
				t.Helper()
				press(t, s, "f", "enter")
			},
			key: "x",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			svc.sendErr = tt.sendErr
			s := started(t, svc, 80, 20)
			tt.setup(t, s)
			target, _ := s.target()
			if m := s.modal(); m != nil {
				target = m.issue
			}

			cmd := s.Update(keyMsg(tt.key))
			if tt.wantChange == "" {
				if cmd != nil || len(svc.changeCalls()) != 0 {
					t.Fatalf("%s asked for %v", tt.key, svc.changeCalls())
				}
				return
			}
			if got := svc.changeCalls(); !slices.Equal(got, []string{tt.wantChange}) {
				t.Fatalf("changes = %v, want %s", got, tt.wantChange)
			}
			done := runHolding(t, s, cmd)
			if got := shownState(s, target.Number); got != tt.wantBefore {
				t.Errorf("before the answer #%d shows %q, want %q", target.Number, got, tt.wantBefore)
			}
			verb, _, _ := strings.Cut(tt.wantChange, " ")
			want := ui.DoneMsg{From: ui.IssuesTitle, What: verb + " #" + strings.TrimPrefix(tt.wantChange, verb+" "), Err: tt.sendErr}
			if len(done) != 1 || done[0].What != want.What || !errors.Is(done[0].Err, tt.sendErr) {
				t.Fatalf("done = %v, want %v", done, want)
			}

			before := len(svc.listCalls())
			run(t, s, s.Update(done[0]))
			if len(svc.listCalls()) == before {
				t.Error("DoneMsg didn't reload the list")
			}
			if got := shownState(s, target.Number); got != tt.wantAfter {
				t.Errorf("after the answer #%d shows %q, want %q", target.Number, got, tt.wantAfter)
			}
		})
	}
}

// shownState returns the state shown for issue number: the header's in the
// modal, or the selected row's, which follows the issue.
func shownState(s *host, number int) core.State {
	if m := s.modal(); m != nil {
		v := ansi.Strip(m.header(m.issue))
		if strings.Contains(v, "● Open") {
			return core.StateOpen
		}
		return core.StateClosed
	}
	it, ok := s.list.Selected()
	if !ok || it.Number != number {
		return ""
	}
	return it.State
}

// A change in the modal shows in the modal and in the list behind it at
// once, and both show GitHub's answer.
func TestChangeInModalShowsInBoth(t *testing.T) {
	for _, sendErr := range []error{nil, errors.New("403 Forbidden")} {
		name := "confirmed"
		if sendErr != nil {
			name = "refused"
		}
		t.Run(name, func(t *testing.T) {
			svc := newFakeService(sampleIssues(12))
			svc.sendErr = sendErr
			// All issues, so the closed one stays in the list.
			h := started(t, svc, 80, 20)
			press(t, h, "f", "f", "down", "enter")
			m := h.modal()
			done := runHolding(t, h, h.Update(keyMsg("x")))
			row := func() core.State {
				it, _ := h.list.Selected()
				return it.State
			}
			if got := shownState(h, 999); got != core.StateClosed {
				t.Fatalf("before the answer the modal shows %q, want closed", got)
			}
			// A refused change is rolled back as soon as it is sent, which
			// the test runs before the list reads the cache again.
			if sendErr == nil && row() != core.StateClosed {
				t.Fatalf("before the answer the list shows %q, want closed", row())
			}
			if len(done) != 1 {
				t.Fatalf("done = %v, want one", done)
			}
			run(t, h, h.Update(done[0]))
			want := core.StateClosed
			if sendErr != nil {
				want = core.StateOpen
			}
			if got := shownState(h, 999); got != want || row() != want || m.issue.State != want {
				t.Errorf("after the answer the modal shows %q and the list %q, want both %q", got, row(), want)
			}
		})
	}
}

func TestDoneOfOthersIsIgnored(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	s := started(t, svc, 80, 20)
	before := len(svc.listCalls())
	run(t, s, s.Update(ui.DoneMsg{From: ui.PullsTitle, What: "merge #3"}))
	if len(svc.listCalls()) != before {
		t.Error("another section's DoneMsg reloaded the list")
	}
}

func TestHelpOffersTheApplicableChange(t *testing.T) {
	s := started(t, newFakeService(sampleIssues(12)), 80, 20)
	offered := func() (closeOn, reopenOn bool) {
		for _, b := range s.Help().ShortHelp() {
			switch {
			case b.Help().Desc == "close":
				closeOn = b.Enabled()
			case b.Help().Desc == "reopen":
				reopenOn = b.Enabled()
			}
		}
		return closeOn, reopenOn
	}
	if c, r := offered(); !c || r {
		t.Errorf("open issue offers close %v, reopen %v; want only close", c, r)
	}
	press(t, s, "f")
	if c, r := offered(); c || !r {
		t.Errorf("closed issue offers close %v, reopen %v; want only reopen", c, r)
	}
	if !s.keys.Close.Enabled() {
		t.Error("Help disabled the section's own close binding")
	}
}
