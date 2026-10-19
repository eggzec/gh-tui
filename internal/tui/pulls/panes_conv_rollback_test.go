package pulls

import (
	"errors"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// async runs commands as the runtime does, each in its own goroutine, and
// gives their messages to the host one at a time, so that a command that waits
// leaves the others to go on.
type async struct {
	h    *host
	out  chan tea.Msg
	msgs []tea.Msg
}

func runAsync(h *host, cmd tea.Cmd) *async {
	a := &async{h: h, out: make(chan tea.Msg, 256)}
	a.run(cmd)
	return a
}

func (a *async) run(c tea.Cmd) {
	if c != nil {
		go a.exec(c)
	}
}

func (a *async) exec(c tea.Cmd) {
	msg := c()
	if seq, ok := sequence(msg); ok {
		for _, sc := range seq {
			if sc != nil {
				a.exec(sc)
			}
		}
		return
	}
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			a.run(c)
		}
	default:
		a.out <- msg
	}
}

// settle gives the host the messages that come until none comes for quiet,
// and returns every message so far.
func (a *async) settle(quiet time.Duration) []tea.Msg {
	for {
		select {
		case msg := <-a.out:
			a.msgs = append(a.msgs, msg)
			switch msg.(type) {
			case spinner.TickMsg, ui.NotifyMsg, ui.OpenMsg:
			default:
				a.run(a.h.Update(msg))
			}
		case <-time.After(quiet):
			return a.msgs
		}
	}
}

// confirmHeld answers y to the question and holds the send of the change until
// the returned release is called, after the pane has taken in what the
// confirming started.
func confirmHeld(t *testing.T, svc *fakeService, h *host) func() []tea.Msg {
	t.Helper()
	hold := &convHold{started: make(chan struct{}), release: make(chan struct{})}
	svc.mu.Lock()
	svc.convHold = hold
	svc.mu.Unlock()
	a := runAsync(h, h.Update(h.key("y")))
	select {
	case <-hold.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the change was never sent")
	}
	a.settle(150 * time.Millisecond)
	return func() []tea.Msg {
		close(hold.release)
		return a.settle(150 * time.Millisecond)
	}
}

func TestConvChangeShowsAtOnceAndFailureRollsBack(t *testing.T) {
	t.Parallel()
	// A check reports that the pane shows the change.
	deleted := func(id string) func(*panesModal) bool {
		return func(m *panesModal) bool { return !m.conv.model.Reveal(id) }
	}
	resolved := func(want bool) func(*panesModal) bool {
		return func(m *panesModal) bool {
			s, ok := m.conv.model.Current()
			return ok && s.Thread != nil && s.Thread.Resolved == want
		}
	}
	for _, tc := range []struct {
		name, key, login, at string
		edit                 func(*core.Thread)
		shows                func(*panesModal) bool
	}{
		{"delete comment", "D", "ann", "C1", nil, deleted("C1")},
		{"delete reply", "D", "bob", "T1-b", nil, deleted("T1-b")},
		{"delete thread comment", "D", "dee", "T1-a", nil, deleted("T1-a")},
		{"resolve", "T", "dee", "T1-a", canResolve, resolved(true)},
		{"reopen", "T", "dee", "T1-a", canUnresolve, resolved(false)},
	} {
		for _, fail := range []bool{false, true} {
			name := tc.name
			if fail {
				name += " fails"
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				svc := convService()
				if fail {
					svc.sendErr = errors.New("boom")
				}
				h, m := convActions(t, svc, tc.login, tc.at, tc.edit)
				if tc.shows(m) {
					t.Fatal("the pane shows the change before it is asked for")
				}
				if !m.conv.model.Reveal(tc.at) {
					t.Fatalf("the conversation has no row for %q", tc.at)
				}
				press(t, h, tc.key)
				if asked(m) == "" {
					t.Fatalf("%s asked nothing", tc.key)
				}
				release := confirmHeld(t, svc, h)
				m.View()
				if !tc.shows(m) {
					t.Fatal("the pane doesn't show the change while the send is held")
				}
				msgs := release()
				m.View()
				if !fail {
					if !tc.shows(m) {
						t.Error("the pane lost the change when the send went through")
					}
					return
				}
				if tc.shows(m) {
					t.Error("the pane still shows the change after the send failed")
				}
				var failed bool
				for _, msg := range msgs {
					if d, ok := msg.(ui.DoneMsg); ok && d.Err != nil {
						failed = true
					}
				}
				if !failed {
					t.Errorf("no DoneMsg carried the error: %v", msgs)
				}
				if got := toasts(msgs); len(got) != 0 {
					t.Errorf("a failed change toasted %v as done", got)
				}
			})
		}
	}
}
