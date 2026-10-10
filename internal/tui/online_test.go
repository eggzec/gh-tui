package tui

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// linkApp is the app, with what tells it the connection, and counts of what
// woke once GitHub answered again.
type linkApp struct {
	m     *Model
	fakes []*fakeSection
	rates *fixedRates
	// wakes are when the polls were woken.
	wakes []time.Time
}

func newLinkApp(t *testing.T) *linkApp {
	t.Helper()
	a := &linkApp{rates: &fixedRates{s: core.RateStatus{Answered: time.Now(), At: time.Now()}}}
	a.m, a.fakes = newTestApp(t, WithRateStatus(a.rates), WithOnline(func() { a.wakes = append(a.wakes, time.Now()) }))
	return a
}

// offline has a request fail a moment later, and tells the app. online
// has GitHub answer a moment later, and returns the command the app
// answers with.
func (a *linkApp) offline() {
	time.Sleep(time.Millisecond)
	s := a.rates.RateStatus()
	s.Failed, s.At = time.Now(), time.Now()
	a.rates.set(s)
	a.sync()
}

func (a *linkApp) online() tea.Cmd {
	time.Sleep(time.Millisecond)
	s := a.rates.RateStatus()
	s.Answered, s.At = time.Now(), time.Now()
	a.rates.set(s)
	return a.sync()
}

func (a *linkApp) sync() tea.Cmd {
	_, cmd := a.m.Update(ui.SyncMsg{Key: core.SyncRateLimit})
	return cmd
}

// onlines counts the ui.OnlineMsg the first section was sent.
func (a *linkApp) onlines() int {
	n := 0
	for _, msg := range a.fakes[0].msgs {
		if _, ok := msg.(ui.OnlineMsg); ok {
			n++
		}
	}
	return n
}

func (a *linkApp) check(t *testing.T, when string, want int) {
	t.Helper()
	if got := a.onlines(); got != want {
		t.Errorf("%s: sections were sent %d OnlineMsg, want %d", when, got, want)
	}
	if got := len(a.wakes); got != want {
		t.Errorf("%s: the polls were woken %d times, want %d", when, got, want)
	}
}

// TestOnlineWakesOnce checks that GitHub answering after the app couldn't
// reach it wakes the polls and the sections once, and that nothing else
// does: not an answer while online, nor a change of another key.
func TestOnlineWakesOnce(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		a := newLinkApp(t)
		run(a.m, a.online())
		a.check(t, "answered while online", 0)

		a.offline()
		// A read that the caches served tells the rate limits nothing,
		// and other keys aren't the connection.
		run(a.m, a.sync())
		run(a.m, func() tea.Msg { return ui.SyncMsg{Key: "pulls"} })
		a.check(t, "while offline", 0)

		run(a.m, a.online())
		a.check(t, "answered again", 1)
		run(a.m, a.online())
		a.check(t, "answered once more", 1)
	})
}

// TestFailingWakesOnRecovery checks that GitHub failing with server
// errors, while reads were served what earlier ones kept, shows in the
// status bar, and that the failing resource answering well again wakes
// the polls and the sections once, as coming online does.
func TestFailingWakesOnRecovery(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		a := newLinkApp(t)
		a.m.Update(tea.WindowSizeMsg{Width: 200, Height: 12})
		time.Sleep(time.Minute)
		s := a.rates.RateStatus()
		s.Answered, s.Failing, s.At = time.Now(), time.Now(), time.Now()
		a.rates.set(s)
		run(a.m, a.sync())
		if line := ansi.Strip(lastLine(a.m)); !strings.Contains(line, "GitHub failing since") {
			t.Errorf("while GitHub fails the bar says %q, want it failing", line)
		}
		a.check(t, "while failing", 0)

		time.Sleep(time.Minute)
		s = a.rates.RateStatus()
		s.Answered, s.Failing, s.Mended, s.At = time.Now(), time.Time{}, time.Now(), time.Now()
		a.rates.set(s)
		run(a.m, a.sync())
		if line := ansi.Strip(lastLine(a.m)); !strings.Contains(line, "● online") {
			t.Errorf("once GitHub answers well the bar says %q, want it online", line)
		}
		a.check(t, "answered well", 1)
		run(a.m, a.online())
		a.check(t, "answered once more", 1)
	})
}

// TestLimitLiftWakes checks that a rate limit lifting wakes the polls
// and the sections once, a spent quota's as well as a secondary limit's,
// since what was kept while it held was served and read only on a wake,
// and that a limit holding wakes nothing.
func TestLimitLiftWakes(t *testing.T) {
	t.Parallel()
	limits := map[string]func(s *core.RateStatus, until time.Time){
		"quota": func(s *core.RateStatus, until time.Time) {
			s.Quotas = []core.Quota{{Resource: "core", Limit: 5000, Reset: until, LimitedUntil: until}}
		},
		"secondary": func(s *core.RateStatus, until time.Time) { s.SecondaryUntil = until },
	}
	for name, limit := range limits {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a := newLinkApp(t)
				s := a.rates.RateStatus()
				limit(&s, time.Now().Add(time.Minute))
				s.At = time.Now()
				a.rates.set(s)
				run(a.m, a.sync())
				run(a.m, a.online())
				a.check(t, "while limited", 0)

				time.Sleep(time.Minute)
				s = a.rates.RateStatus()
				limit(&s, time.Time{})
				s.At = time.Now()
				a.rates.set(s)
				run(a.m, a.sync())
				a.check(t, "lifted", 1)
				run(a.m, a.online())
				a.check(t, "answered once more", 1)
			})
		})
	}
}

// TestOnlineAfterOutageTellsTheLimit checks that GitHub answering again
// after an outage, while a rate limit still holds, wakes the sections with
// an OnlineMsg that says so, and that the limit lifting later wakes them
// with one that doesn't, so that reads ahead resume only then.
func TestOnlineAfterOutageTellsTheLimit(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		a := newLinkApp(t)
		run(a.m, a.limit(func(s *core.RateStatus) { s.Quotas = []core.Quota{spent("core", time.Now().Add(time.Minute))} }))
		a.offline()
		run(a.m, a.online())
		time.Sleep(time.Minute)
		run(a.m, a.limit(func(s *core.RateStatus) { s.Quotas = []core.Quota{spent("core", time.Time{})} }))

		var got []bool
		for _, msg := range a.fakes[0].msgs {
			if o, ok := msg.(ui.OnlineMsg); ok {
				got = append(got, o.Limited)
			}
		}
		if want := []bool{true, false}; !slices.Equal(got, want) {
			t.Errorf("OnlineMsg Limited = %v, want %v", got, want)
		}
	})
}

// limit sets the rate status to change, taken now, and tells the app,
// returning the command it answers with.
func (a *linkApp) limit(change func(s *core.RateStatus)) tea.Cmd {
	s := a.rates.RateStatus()
	// The app keeps the status it read, quotas and all.
	s.Quotas = slices.Clone(s.Quotas)
	change(&s)
	s.At = time.Now()
	a.rates.set(s)
	return a.sync()
}

// spent returns the quota of resource, spent until until, or not spent if
// until is zero.
func spent(resource string, until time.Time) core.Quota {
	return core.Quota{Resource: resource, Limit: 5000, Reset: until, LimitedUntil: until}
}

// TestLimitLiftWaitsForTheLast checks that a quota lifting while another
// still holds wakes nothing, and that the last one lifting wakes the polls
// and the sections.
func TestLimitLiftWaitsForTheLast(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		a := newLinkApp(t)
		now := time.Now()
		run(a.m, a.limit(func(s *core.RateStatus) {
			s.Quotas = []core.Quota{spent("core", now.Add(time.Minute)), spent("graphql", now.Add(2*time.Minute))}
		}))
		time.Sleep(time.Minute)
		run(a.m, a.limit(func(s *core.RateStatus) { s.Quotas[0] = spent("core", time.Time{}) }))
		a.check(t, "core lifted, graphql holds", 0)
		time.Sleep(time.Minute)
		run(a.m, a.limit(func(s *core.RateStatus) { s.Quotas[1] = spent("graphql", time.Time{}) }))
		a.check(t, "graphql lifted too", 1)
	})
}

// TestLimitLiftWhileOfflineOrFailing checks that a limit lifting while
// GitHub can't be reached, or fails with server errors, wakes nothing:
// the reads would fail again, and waking the polls would undo their
// backoff.
func TestLimitLiftWhileOfflineOrFailing(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		down func(a *linkApp)
	}{
		{"offline", func(a *linkApp) { a.offline() }},
		{"failing", func(a *linkApp) {
			run(a.m, a.limit(func(s *core.RateStatus) { s.Answered, s.Failing = time.Now(), time.Now() }))
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				a := newLinkApp(t)
				until := time.Now().Add(time.Minute)
				run(a.m, a.limit(func(s *core.RateStatus) { s.Quotas = []core.Quota{spent("core", until)} }))
				tt.down(a)
				time.Sleep(time.Minute)
				run(a.m, a.limit(func(s *core.RateStatus) { s.Quotas = []core.Quota{spent("core", time.Time{})} }))
				a.check(t, "lifted while "+tt.name, 0)
			})
		})
	}
}

// TestLimitLiftsWakeOnceAGap checks that a limit that holds and lifts
// again within onlineGap of the last wake waits for the gap's end.
func TestLimitLiftsWakeOnceAGap(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		a := newLinkApp(t)
		hold := func(until time.Time) tea.Cmd {
			return a.limit(func(s *core.RateStatus) { s.Quotas = []core.Quota{spent("core", until)} })
		}
		run(a.m, hold(time.Now().Add(time.Second)))
		time.Sleep(time.Second)
		run(a.m, hold(time.Time{}))
		a.check(t, "first lift", 1)

		run(a.m, hold(time.Now().Add(time.Second)))
		time.Sleep(time.Second)
		tick := hold(time.Time{})
		if tick == nil {
			t.Fatal("a lift within the gap should wait for its end")
		}
		a.check(t, "second lift within the gap", 1)
		run(a.m, tick)
		a.check(t, "end of the gap", 2)
		if gap := a.wakes[1].Sub(a.wakes[0]); gap != onlineGap {
			t.Errorf("woke again %v after the first lift, want %v", gap, onlineGap)
		}
	})
}

// TestFailingLapsesDoNotWake checks that failing that only goes quiet,
// as when GraphQL fails at each of its polls a minute apart while REST
// answers well in between, wakes neither the polls nor the sections,
// which would undo the backoff of the failing poll, that the bar tells
// the same start each time it shows, and that it is logged once.
func TestFailingLapsesDoNotWake(t *testing.T) {
	// Not parallel: it replaces the default logger, which the whole process shares.
	synctest.Test(t, func(t *testing.T) {
		var buf bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
		t.Cleanup(func() { slog.SetDefault(prev) })

		a := newLinkApp(t)
		a.m.Update(tea.WindowSizeMsg{Width: 200, Height: 12})
		since := time.Now()
		want := "GitHub failing since " + ui.Clock(since, time.Now())
		for range 10 {
			s := a.rates.RateStatus()
			s.Answered, s.Failing, s.At = time.Now(), since, time.Now()
			a.rates.set(s)
			run(a.m, a.sync())
			if line := ansi.Strip(lastLine(a.m)); !strings.Contains(line, want) {
				t.Errorf("while failing the bar says %q, want %q", line, want)
			}
			time.Sleep(35 * time.Second)
			s = a.rates.RateStatus()
			s.Answered, s.Failing, s.At = time.Now(), time.Time{}, time.Now()
			a.rates.set(s)
			run(a.m, a.sync())
			time.Sleep(25 * time.Second)
		}
		a.check(t, "after ten lapses", 0)
		if n := strings.Count(buf.String(), `"state":"failing"`); n != 1 {
			t.Errorf("logged failing %d times, want once", n)
		}
	})
}

// TestOnlineWakesWhileFailing checks that GitHub answering again after
// an outage wakes the polls and the sections even while a resource still
// fails with server errors, and so does the wait for onlineGap.
func TestOnlineWakesWhileFailing(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		a := newLinkApp(t)
		s := a.rates.RateStatus()
		s.Failing = time.Now()
		a.rates.set(s)
		a.offline()
		run(a.m, a.online())
		a.check(t, "answer after an outage while failing", 1)

		time.Sleep(time.Second)
		a.offline()
		tick := a.online()
		if tick == nil {
			t.Fatal("a flip within the gap should wait for its end")
		}
		run(a.m, tick)
		a.check(t, "end of the gap while failing", 2)
	})
}

// TestFailingSteadyDoesNotWake checks that while GitHub keeps failing,
// as when GraphQL fails while REST answers well, answers coming in don't
// wake the polls and the sections, and the bar keeps telling since when
// it failed.
func TestFailingSteadyDoesNotWake(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		a := newLinkApp(t)
		a.m.Update(tea.WindowSizeMsg{Width: 200, Height: 12})
		since := time.Now()
		for range 30 {
			s := a.rates.RateStatus()
			s.Answered, s.Failing, s.At = time.Now(), since, time.Now()
			a.rates.set(s)
			run(a.m, a.sync())
			time.Sleep(10 * time.Second)
		}
		want := "GitHub failing since " + ui.Clock(since, time.Now())
		if line := ansi.Strip(lastLine(a.m)); !strings.Contains(line, want) {
			t.Errorf("after five minutes of failing the bar says %q, want %q", line, want)
		}
		a.check(t, "while failing", 0)
	})
}

// TestOnlineFlapsWakeOnceAGap checks that a connection going on and off
// line wakes the polls and the sections at most once each onlineGap, and
// only if GitHub still answers when the gap ends.
func TestOnlineFlapsWakeOnceAGap(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		a := newLinkApp(t)
		a.offline()
		run(a.m, a.online())
		a.check(t, "first answer", 1)

		// Two more flips within the gap wait for its end, as one.
		time.Sleep(2 * time.Second)
		a.offline()
		tick := a.online()
		if tick == nil {
			t.Fatal("a flip within the gap should wait for its end")
		}
		time.Sleep(2 * time.Second)
		a.offline()
		if cmd := a.online(); cmd != nil {
			t.Error("a third flip within the gap should join the wait")
		}
		a.check(t, "flips within the gap", 1)
		run(a.m, tick)
		a.check(t, "end of the gap, online", 2)
		if gap := a.wakes[1].Sub(a.wakes[0]); gap != onlineGap {
			t.Errorf("woke again %v after the first answer, want %v", gap, onlineGap)
		}

		// Offline again when the gap ends: nothing to wake, and the next
		// answer wakes them at once.
		time.Sleep(time.Second)
		a.offline()
		tick = a.online()
		a.offline()
		run(a.m, tick)
		a.check(t, "end of the gap, offline", 2)
		time.Sleep(onlineGap)
		run(a.m, a.online())
		a.check(t, "answer after the gap", 3)
	})
}

// TestConnectionLogged checks that each change of the connection is
// logged once, with the host: offline since when, online again after how
// long, and the token rejected.
func TestConnectionLogged(t *testing.T) {
	// Not parallel: it replaces the default logger, which the whole process shares.
	synctest.Test(t, func(t *testing.T) {
		var buf bytes.Buffer
		prev := slog.Default()
		slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
		t.Cleanup(func() { slog.SetDefault(prev) })

		a := newLinkApp(t)
		a.offline()
		since := a.rates.RateStatus().Failed
		a.offline()
		time.Sleep(90 * time.Second)
		run(a.m, a.online())
		run(a.m, a.online())
		s := a.rates.RateStatus()
		s.Rejected = time.Now()
		a.rates.set(s)
		a.sync()
		a.sync()

		var got []map[string]any
		for line := range strings.Lines(buf.String()) {
			var r map[string]any
			if err := json.Unmarshal([]byte(line), &r); err != nil {
				t.Fatal(err)
			}
			if r["msg"] == "connection" {
				got = append(got, r)
			}
		}
		if len(got) != 3 {
			t.Fatalf("connection records = %v, want 3", got)
		}
		for i, want := range []string{"offline", "online", "rejected"} {
			if got[i]["state"] != want || got[i]["host"] != "github.com" {
				t.Errorf("record %d = %v, want state %s on github.com", i, got[i], want)
			}
		}
		if at, _ := time.Parse(time.RFC3339Nano, got[0]["since"].(string)); !at.Equal(since) {
			t.Errorf("offline since %v, want %v", got[0]["since"], since)
		}
		if got[1]["offline_s"] != 90.0 {
			t.Errorf("offline for %vs, want 90s", got[1]["offline_s"])
		}
	})
}
