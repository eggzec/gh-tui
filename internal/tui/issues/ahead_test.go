package issues

import (
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/threads"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Changed does nothing: the fake's issues don't age.
func (*fakeService) Changed(core.RepoRef, int, time.Time) {}

// firstComments returns the numbers whose first comments were read.
func (f *fakeService) firstComments() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []int
	for _, q := range f.commentQueries {
		if q == commentsQuery(q.Repo, q.Number) {
			out = append(out, q.Number)
		}
	}
	return out
}

func sorted(s []int) []int {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}

func TestPrefetchFirstRows(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService(sampleIssues(12))
		// 999 is cached already.
		svc.cached[999] = issue(svc, 999)
		svc.commented[commentsQuery(testRepo, 999)] = true
		started(t, svc, 80, 30, readingAhead(2, 150*time.Millisecond, false))

		// The first three rows but the cached one. The row under the
		// cursor is among them, so it isn't read again.
		want := []int{998, 1000}
		if got := sorted(svc.getCalls()); !slices.Equal(got, want) {
			t.Errorf("read issues %v, want %v", got, want)
		}
		if got := sorted(svc.firstComments()); !slices.Equal(got, want) {
			t.Errorf("read first comments of %v, want %v", got, want)
		}
	})
}

func TestPrefetchHover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService(sampleIssues(12))
		h := started(t, svc, 80, 30, readingAhead(0, 150*time.Millisecond, false))
		if got := svc.getCalls(); !slices.Equal(got, []int{1000}) {
			t.Errorf("read issues %v, want the row under the cursor", got)
		}
		start := time.Now()
		press(t, h, "down")
		if waited := time.Since(start); waited != 150*time.Millisecond {
			t.Errorf("read after %v, want the delay", waited)
		}
		if got := svc.getCalls(); !slices.Equal(got, []int{1000, 999}) {
			t.Errorf("read issues %v, want the row moved to", got)
		}
		press(t, h, "up")
		if got := svc.getCalls(); len(got) != 2 {
			t.Errorf("read issues %v, want no more", got)
		}
	})
}

func TestPrefetchCancelledByRepo(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 30, readingAhead(2, time.Millisecond, false))
	svc.mu.Lock()
	ctxs := slices.Clone(svc.getCtxs)
	svc.mu.Unlock()
	if len(ctxs) == 0 {
		t.Fatal("nothing was read ahead")
	}
	run(t, h, h.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}}))
	for _, ctx := range ctxs {
		if ctx.Err() == nil {
			t.Error("a read ahead of the old repository wasn't cancelled")
		}
	}
}

func TestPrefetchStopsAtRateLimit(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.getErr = &core.RateLimitError{Reset: testNow}
	h := started(t, svc, 80, 30, readingAhead(4, time.Millisecond, false))
	n := len(svc.getCalls())
	if n == 0 || n > 3 {
		t.Errorf("read %d issues before the rate limit stopped it, want 1 to 3", n)
	}
	press(t, h, "down", "down")
	if got := len(svc.getCalls()); got != n {
		t.Errorf("read %d more issues under the rate limit", got-n)
	}
	svc.mu.Lock()
	svc.getErr = nil
	svc.mu.Unlock()
	// Once the limit lifts, the window is read again, before the cursor
	// moves.
	run(t, h, h.Update(ui.OnlineMsg{}))
	if got := len(svc.getCalls()); got == n {
		t.Error("nothing was read ahead once the rate limit lifted")
	}
}

func TestPrefetchedModalOpensAtOnce(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, sampleComments(3)...)
	h := started(t, svc, 80, 40, readingAhead(4, time.Millisecond, false))
	press(t, h, "down")
	seq, ok := sequence(h.Update(keyMsg("enter"))())
	if !ok || len(seq) != 2 {
		t.Fatal("enter should open the modal, then start its loads")
	}
	run(t, h, seq[0])
	view := ansi.Strip(h.modal().View())
	for _, want := range []string{"Look at issue 999", "I can reproduce this", "Thanks! Fixed on main."} {
		if !strings.Contains(view, want) {
			t.Errorf("modal lacks %q before any load:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Loading") {
		t.Errorf("modal waits for what was read ahead:\n%s", view)
	}
}

func TestNoPrefetchByDefault(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 30)
	press(t, h, "down")
	if got := svc.getCalls(); len(got) != 0 {
		t.Errorf("read issues %v without WithPrefetch", got)
	}
}

// issue returns issue number of svc.
func issue(svc *fakeService, number int) core.Issue {
	i := slices.IndexFunc(svc.issues, func(it core.Issue) bool { return it.Number == number })
	return svc.issues[i]
}

// TestNotificationReadAheadOpensAtOnce reads an issue ahead from a
// notification, and opens it from the notification: it shows at once, from
// what was read ahead.
func TestNotificationReadAheadOpensAtOnce(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.addComments(997, sampleComments(3)...)
	h := started(t, svc, 80, 40)
	n := core.Notification{Repo: testRepo, Subject: core.Subject{Type: core.SubjectIssue, Number: 997}, UpdatedAt: time.Now()}
	o := threads.New(t.Context(), threads.WithIssues(svc), threads.WithPrefetch(prefetchOf(0, time.Millisecond)))
	run(t, h, o.ReadAhead(func(i int) (core.Notification, bool) { return n, i == 0 }, 0))
	if got := svc.getCalls(); !slices.Contains(got, 997) {
		t.Fatalf("read %v ahead, want #997", got)
	}

	seq, ok := sequence(h.Update(o.Open(n)())())
	if !ok || len(seq) != 2 {
		t.Fatal("the notification should open the modal, then start its loads")
	}
	run(t, h, seq[0])
	view := ansi.Strip(h.modal().View())
	for _, want := range []string{"Look at issue 997", "I can reproduce this", "Thanks! Fixed on main."} {
		if !strings.Contains(view, want) {
			t.Errorf("modal lacks %q before any load:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Loading") {
		t.Errorf("modal waits for what was read ahead:\n%s", view)
	}
}

// rested runs cmd, which moved the cursor, until the delay reports that it
// rested, and returns the read that s then starts.
func rested(t *testing.T, s *Section, cmd tea.Cmd) tea.Cmd {
	t.Helper()
	var find func(tea.Cmd) (ui.AheadMsg, bool)
	find = func(c tea.Cmd) (ui.AheadMsg, bool) {
		if c == nil {
			return ui.AheadMsg{}, false
		}
		switch msg := c().(type) {
		case ui.AheadMsg:
			return msg, true
		case tea.BatchMsg:
			for _, c := range msg {
				if m, ok := find(c); ok {
					return m, true
				}
			}
		}
		return ui.AheadMsg{}, false
	}
	msg, ok := find(cmd)
	if !ok {
		t.Fatal("moving the cursor started no delay")
	}
	read := s.Update(msg)
	if read == nil {
		t.Fatal("resting on the row read nothing")
	}
	return read
}

func TestModalPausesReadAhead(t *testing.T) {
	tests := []struct {
		name string
		// settle runs the loads of the modal, or closes it.
		settle func(t *testing.T, h *host, loads tea.Cmd)
	}{
		{"until its detail loads", func(t *testing.T, h *host, loads tea.Cmd) {
			t.Helper()
			run(t, h, loads)
		}},
		{"until it closes", func(t *testing.T, h *host, _ tea.Cmd) {
			t.Helper()
			press(t, h, "esc")
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				svc := newFakeService(sampleIssues(12))
				h := started(t, svc, 80, 40, readingAhead(0, 150*time.Millisecond, false))
				seq, ok := sequence(h.Update(keyMsg("enter"))())
				if !ok || len(seq) != 2 {
					t.Fatal("enter should open the modal, then start its loads")
				}
				run(t, h, seq[0])
				// The list reads #999 ahead behind the modal.
				read := rested(t, h.Section, h.Section.Update(keyMsg("down")))
				done := make(chan struct{})
				go func() {
					read()
					close(done)
				}()
				synctest.Wait()
				if slices.Contains(svc.getCalls(), 999) {
					t.Error("read #999 ahead while the modal loads")
				}
				tt.settle(t, h, seq[1])
				<-done
				if !slices.Contains(svc.getCalls(), 999) {
					t.Error("didn't read #999 ahead once the modal settled")
				}
			})
		})
	}
}
