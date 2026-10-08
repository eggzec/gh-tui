package search

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// pump runs the commands of a page the way the app does, each on its own
// goroutine, so that a wait such as the debounce only delivers once the
// clock of the synctest bubble reaches it.
type pump struct {
	s    *Section
	msgs chan tea.Msg
}

func newPump(s *Section) *pump {
	// Room for every message a test sends, so that no command blocks on
	// the channel once the test stops reading it.
	return &pump{s: s, msgs: make(chan tea.Msg, 1024)}
}

func (p *pump) start(cmd tea.Cmd) {
	if cmd != nil {
		go func() { p.msgs <- cmd() }()
	}
}

// settle gives the page every message its commands have sent by now, and
// starts the commands it returns, until only waits are left.
func (p *pump) settle(t *testing.T) {
	t.Helper()
	for {
		synctest.Wait()
		select {
		case msg := <-p.msgs:
			switch msg := msg.(type) {
			case nil, spinner.TickMsg, codeTickMsg:
			case tea.BatchMsg:
				for _, c := range msg {
					p.start(c)
				}
			case ui.OpenMsg, ui.NotifyMsg, ui.RepoMsg, ui.OpenPullMsg, ui.OpenIssueMsg, ui.OpenFileMsg:
			default:
				p.start(p.s.Update(msg))
			}
		default:
			return
		}
	}
}

// key presses k and settles.
func (p *pump) key(t *testing.T, k string) {
	t.Helper()
	p.start(p.s.Update(keyPress(k)))
	p.settle(t)
}

// wait lets d pass and settles.
func (p *pump) wait(t *testing.T, d time.Duration) {
	t.Helper()
	time.Sleep(d)
	p.settle(t)
}

// prefetched returns the contexts of the prefetches svc was asked for.
func (f *fakeService) prefetched() []context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ctxs []context.Context
	for _, ctx := range f.ctxs {
		if obs.IsPrefetch(ctx) {
			ctxs = append(ctxs, ctx)
		}
	}
	return ctxs
}

// countStats counts into a Stats of their own while the test runs.
func countStats(t *testing.T) *obs.Stats {
	t.Helper()
	stats := obs.NewStats()
	prev := obs.SetDefault(stats)
	t.Cleanup(func() { obs.SetDefault(prev) })
	return stats
}

// Typing searches only the kind on view; once the query rests long enough,
// the other kinds are read ahead, once.
func TestOthersWaitForTheQueryToRest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := countStats(t)
		svc := newFake()
		s := newSection(t, svc, 120, 30, WithDebounce(DefaultDebounce), withOthersWait(defaultOthersWait))
		p := newPump(s)
		// Pauses longer than the debounce, and shorter than the rest.
		for _, k := range []string{"i", "t", "e", "a"} {
			p.key(t, k)
			p.wait(t, 300*time.Millisecond)
		}
		if n, _ := svc.stats(); n != 3 {
			t.Errorf("%d searches while typing, want one per pause", n)
		}
		if n := len(svc.prefetched()); n != 0 {
			t.Fatalf("%d reads of the other kinds while typing, want none", n)
		}
		p.wait(t, defaultOthersWait-300*time.Millisecond)
		ctxs := svc.prefetched()
		if len(ctxs) != 2 || svc.prefetches != 2 {
			t.Fatalf("%d reads of the other kinds once the query rested, want 2", len(ctxs))
		}
		for i, ctx := range ctxs {
			if ctx.Err() != nil {
				t.Errorf("read %d of the other kinds was canceled", i)
			}
		}
		// Enter reads nothing more, and nor does another rest.
		p.key(t, "enter")
		p.wait(t, 2*defaultOthersWait)
		if n := len(svc.prefetched()); n != 2 {
			t.Errorf("%d reads of the other kinds, want them read once", n)
		}
		if got := stats.Summary().Prefetch; len(got) != 1 || got[0].Kind != "search" || got[0].Sent != 2 || got[0].Read != 2 {
			t.Errorf("summary = %+v, want 2 searches sent and read", got)
		}
		// Showing one of them counts it as used.
		p.key(t, "]")
		if got := stats.Summary().Prefetch[0]; got.Opened != 1 {
			t.Errorf("summary = %+v, want the issues opened", got)
		}
	})
}

// Enter reads the other kinds at once, without waiting for the rest.
func TestOthersOnEnter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFake()
		s := newSection(t, svc, 120, 30, WithDebounce(DefaultDebounce), withOthersWait(defaultOthersWait))
		p := newPump(s)
		for _, k := range []string{"i", "t", "e", "a"} {
			p.key(t, k)
			p.wait(t, 50*time.Millisecond)
		}
		p.key(t, "enter")
		if n, _ := svc.stats(); n != 1 {
			t.Errorf("%d searches, want one for the query entered", n)
		}
		if n := len(svc.prefetched()); n != 2 {
			t.Fatalf("%d reads of the other kinds on enter, want 2", n)
		}
		p.wait(t, 2*defaultOthersWait)
		if n := len(svc.prefetched()); n != 2 {
			t.Errorf("%d reads of the other kinds, want them read once", n)
		}
	})
}

// Leaving the page cancels the reads of the other kinds, and not the
// search of the kind on view.
func TestLeavingCancelsOthers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFake()
		svc.hold = make(chan struct{})
		s := newSection(t, svc, 120, 30, WithDebounce(DefaultDebounce), withOthersWait(defaultOthersWait))
		p := newPump(s)
		for _, k := range []string{"i", "t", "e", "a"} {
			p.key(t, k)
		}
		p.wait(t, defaultOthersWait)
		ctxs := svc.prefetched()
		if len(ctxs) != 2 {
			t.Fatalf("%d reads of the other kinds, want 2 in flight", len(ctxs))
		}
		s.Blur()
		p.settle(t)
		for i, ctx := range ctxs {
			if ctx.Err() == nil {
				t.Errorf("read %d of the other kinds goes on off the page", i)
			}
		}
		svc.mu.Lock()
		search := slices.IndexFunc(svc.ctxs, func(ctx context.Context) bool { return !obs.IsPrefetch(ctx) })
		if svc.ctxs[search].Err() != nil {
			t.Error("leaving canceled the search of the kind on view")
		}
		svc.mu.Unlock()
		// Back on the page, on the results, enter in the query reads them
		// again.
		s.Focus()
		close(svc.hold)
		p.key(t, "1")
		p.key(t, "enter")
		if n := len(svc.prefetched()); n != 4 {
			t.Errorf("%d reads of the other kinds, want 2 more on enter", n)
		}
	})
}

// Leaving the page before the query rests reads nothing of the other
// kinds.
func TestLeavingBeforeTheRest(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFake()
		s := newSection(t, svc, 120, 30, WithDebounce(DefaultDebounce), withOthersWait(defaultOthersWait))
		p := newPump(s)
		for _, k := range []string{"i", "t", "e", "a"} {
			p.key(t, k)
		}
		p.wait(t, defaultOthersWait/2)
		s.Blur()
		p.wait(t, 2*defaultOthersWait)
		if n := len(svc.prefetched()); n != 0 {
			t.Errorf("%d reads of the other kinds after leaving, want none", n)
		}
	})
}

// defaultOthersWait is how long the query rests by default before the
// other kinds are read.
var defaultOthersWait = ui.Resolve(config.Default().Prefetch, "search", "other_kinds").Rest
