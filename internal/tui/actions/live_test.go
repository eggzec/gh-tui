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
		h.keys("j") // the running run is the second, and starts folded
		// The host ran the tick, a second of the fake clock, and holds it.
		if !m.ticking || len(h.held) != 1 {
			t.Fatalf("ticking %v with %d ticks, want the timers running", m.ticking, len(h.held))
		}
		h.release()
		if s := screen(m); !strings.Contains(s, "Duration 1m 22s") {
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
