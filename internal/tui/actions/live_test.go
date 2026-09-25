package actions

import (
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
)

// withTimers moves the timers every second of the fake clock of a
// synctest bubble, which starts at testNow.
func withTimers() Option {
	start := time.Now()
	return func(o *options) {
		o.tick = time.Second
		o.now = func() time.Time { return testNow.Add(time.Since(start)) }
	}
}

func TestTimersMoveOnWhileTheRunRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, h := newModal(t, newFake(), wideW, wideH, withTimers())
		h.hold = func(msg tea.Msg) bool { _, ok := msg.(tickMsg); return ok }
		if m.ticking {
			t.Fatal("the timers run for a done run")
		}
		h.keys("j")
		// The host ran the tick, a second of the fake clock, and holds it.
		if !m.ticking || len(h.held) != 1 {
			t.Fatalf("ticking %v with %d ticks, want the timers running", m.ticking, len(h.held))
		}
		if s := paneText(m, logPane); !strings.Contains(s, "Run golangci-lint 1m 19s") {
			t.Errorf("after a second the step shows:\n%s", s)
		}
		h.release()
		if s := paneText(m, logPane); !strings.Contains(s, "Run golangci-lint 1m 20s") {
			t.Errorf("after two seconds the step shows:\n%s", s)
		}
		if s := paneText(m, runsPane); !strings.Contains(s, "1m 32s") {
			t.Errorf("the running run's timer didn't move on:\n%s", s)
		}
		// Once no run shown runs, the next tick stops them.
		h.keys("k")
		h.release()
		if m.ticking {
			t.Error("the timers run on for a done run")
		}
	})
}
