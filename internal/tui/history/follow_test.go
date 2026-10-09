package history

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
)

// clock is a host that runs every command in a goroutine of its own, as
// the program does, so that timers run on the fake clock of synctest while
// the test moves the cursor.
type clock struct {
	m    *Modal
	msgs chan tea.Msg
}

func newClock(m *Modal) *clock {
	return &clock{m: m, msgs: make(chan tea.Msg, 256)}
}

func (c *clock) exec(cmd tea.Cmd) {
	if cmd == nil {
		return
	}
	go func() { c.msgs <- cmd() }()
}

// settle applies every message that arrives until all the commands left
// wait on a timer or a held read.
func (c *clock) settle() {
	for {
		synctest.Wait()
		select {
		case msg := <-c.msgs:
			c.handle(msg)
		default:
			return
		}
	}
}

func (c *clock) handle(msg tea.Msg) {
	switch msg := msg.(type) {
	case nil, spinner.TickMsg:
		return
	case tea.BatchMsg:
		for _, cmd := range msg {
			c.exec(cmd)
		}
		return
	}
	if cmds, ok := sequence(msg); ok {
		for _, cmd := range cmds {
			c.exec(cmd)
		}
		return
	}
	c.exec(c.m.Update(msg))
}

func (c *clock) press(k string) {
	c.exec(c.m.Update(press(k)))
	c.settle()
}

func TestFollowWaitsForTheCursorToRest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		around := testPrefetch(func(p *config.PrefetchLayers) {
			p.History.Rest = new(150 * time.Millisecond)
			p.History.Window = config.Span{Before: new(1), After: new(1)}
		})
		m := New(t.Context(), f, repo, "main", baseNone, testKeys(), WithConfig(testConfig()), around, withClock(testNow))
		m.SetTheme(testTheme())
		// Narrow, so that no patch is highlighted: the highlighter keeps a
		// clock of its own running, which synctest takes for a leak.
		m.SetSize(60, 18)
		c := newClock(m)
		c.exec(m.Init())
		c.settle()
		time.Sleep(200 * time.Millisecond)
		c.settle()
		f.took()

		// Moving on before the delay reads nothing but what the cursor
		// passes, which the graph shows from its rows.
		for range 4 {
			c.press("j")
			time.Sleep(100 * time.Millisecond)
			c.settle()
		}
		if got := f.took(); len(got) != 0 {
			t.Fatalf("calls while moving = %q, want none", got)
		}
		if m.commit.c.SHA != sha("main", 4) || m.commit.loaded || !m.commit.loading {
			t.Fatalf("commit pane shows %q, loaded %v; want the commit under the cursor loading", short(m.commit.c.SHA), m.commit.loaded)
		}
		// Once it rests, the commit under the cursor is read, and the one
		// on each side of it.
		time.Sleep(100 * time.Millisecond)
		c.settle()
		want := commitCalls(sha("main", 4), sha("main", 5), sha("main", 3))
		// They run at once, in any order.
		if got := f.took(); !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
			t.Errorf("calls after the rest = %q, want %q", got, want)
		}
		if !m.commit.loaded || m.commit.c.SHA != sha("main", 4) {
			t.Error("the commit under the cursor didn't show")
		}
	})
}

func TestReadsAroundAreBoundedAndCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newFake()
		around := testPrefetch(func(p *config.PrefetchLayers) { p.History.Window = config.Span{Before: new(5), After: new(5)} })
		m := New(t.Context(), f, repo, "main", baseNone, testKeys(), WithConfig(testConfig()), around, withClock(testNow))
		m.SetTheme(testTheme())
		// Narrow, so that no patch is highlighted: the highlighter keeps a
		// clock of its own running, which synctest takes for a leak.
		m.SetSize(60, 18)
		c := newClock(m)
		f.hold = make(chan struct{})
		c.exec(m.Init())
		c.settle()
		// The commit under the cursor and at most three ahead are read at
		// once; the rest wait.
		if got := f.took(); len(got) != 2+1+3 {
			t.Fatalf("calls = %q, want the commit and three read ahead", got)
		}
		// Another branch cancels the reads ahead of the last.
		c.press("backspace")
		c.press("j")
		c.press("enter")
		f.mu.Lock()
		cancelled := slices.Clone(f.cancelled)
		f.mu.Unlock()
		if len(cancelled) < 3 {
			t.Errorf("cancelled %q, want the reads ahead on main", cancelled)
		}
		close(f.hold)
		c.settle()
	})
}
