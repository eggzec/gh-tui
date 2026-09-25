package issues

import (
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

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
		started(t, svc, 80, 30, WithPrefetch(3, 150*time.Millisecond))

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
		h := started(t, svc, 80, 30, WithPrefetch(0, 150*time.Millisecond))
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
	h := started(t, svc, 80, 30, WithPrefetch(3, time.Millisecond))
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
	h := started(t, svc, 80, 30, WithPrefetch(5, time.Millisecond))
	n := len(svc.getCalls())
	if n == 0 || n > 3 {
		t.Errorf("read %d issues before the rate limit stopped it, want 1 to 3", n)
	}
	press(t, h, "down", "down")
	if got := len(svc.getCalls()); got != n {
		t.Errorf("read %d more issues under the rate limit", got-n)
	}
}

func TestPrefetchedModalOpensAtOnce(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	svc.addComments(999, sampleComments(3)...)
	h := started(t, svc, 80, 40, WithPrefetch(5, time.Millisecond))
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
	o := threads.New(t.Context(), threads.WithIssues(svc), threads.WithPrefetch(1, time.Millisecond))
	run(t, h, o.ReadAhead(func(i int) (core.Notification, bool) { return n, i == 0 }, n, false))
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
