package tui

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

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

// TestOnlineFlapsWakeOnceAGap checks that a connection going on and off
// line wakes the polls and the sections at most once each onlineGap, and
// only if GitHub still answers when the gap ends.
func TestOnlineFlapsWakeOnceAGap(t *testing.T) {
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
