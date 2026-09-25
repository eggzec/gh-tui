package pulls

import (
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/threads"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Changed does nothing: the fake's details don't age.
func (*fakeService) Changed(core.RepoRef, int, time.Time) {}

// firstComments returns the numbers whose first comments were read.
func (f *fakeService) firstComments() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []int
	for _, q := range f.comments {
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
		svc := newFakeService()
		// #135 is cached already.
		svc.cached[135] = true
		svc.commented[commentsQuery(repo, 135)] = true
		started(t, svc, 80, 30, WithPrefetch(3, 150*time.Millisecond))

		// The first three rows but the cached one. The row under the
		// cursor is among them, so it isn't read again.
		want := []int{128, 142}
		if got := sorted(svc.got()); !slices.Equal(got, want) {
			t.Errorf("read details %v, want %v", got, want)
		}
		if got := sorted(svc.firstComments()); !slices.Equal(got, want) {
			t.Errorf("read first comments of %v, want %v", got, want)
		}
	})
}

func TestPrefetchHover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		h := started(t, svc, 80, 30, WithPrefetch(0, 150*time.Millisecond))
		if got := svc.got(); !slices.Equal(got, []int{142}) {
			t.Errorf("read details %v, want the row under the cursor", got)
		}
		start := time.Now()
		press(t, h, "down")
		if waited := time.Since(start); waited != 150*time.Millisecond {
			t.Errorf("read after %v, want the delay", waited)
		}
		if got := svc.got(); !slices.Equal(got, []int{142, 135}) {
			t.Errorf("read details %v, want the row moved to", got)
		}
		// Coming back to a cached row reads nothing.
		press(t, h, "up")
		if got := svc.got(); len(got) != 2 {
			t.Errorf("read details %v, want no more", got)
		}
	})
}

func TestPrefetchCancelledByRepo(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 30, WithPrefetch(3, time.Millisecond))
	svc.mu.Lock()
	ctxs := slices.Clone(svc.getCtxs)
	svc.mu.Unlock()
	if len(ctxs) == 0 {
		t.Fatal("nothing was read ahead")
	}
	drain(t, h, h.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}}))
	for _, ctx := range ctxs {
		if ctx.Err() == nil {
			t.Error("a read ahead of the old repository wasn't cancelled")
		}
	}
}

func TestPrefetchStopsAtRateLimit(t *testing.T) {
	svc := newFakeService()
	svc.getErr = &core.RateLimitError{Reset: clock}
	h := started(t, svc, 80, 30, WithPrefetch(5, time.Millisecond))
	n := len(svc.got())
	if n == 0 || n > 3 {
		t.Errorf("read %d details before the rate limit stopped it, want 1 to 3", n)
	}
	press(t, h, "down")
	press(t, h, "down")
	if got := len(svc.got()); got != n {
		t.Errorf("read %d more details under the rate limit", got-n)
	}
	// Another repository reads ahead again.
	svc.mu.Lock()
	svc.getErr = nil
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	for i := range svc.pulls {
		svc.pulls[i].Repo = other
	}
	svc.mu.Unlock()
	drain(t, h, h.Update(ui.RepoMsg{Repo: other}))
	if got := len(svc.got()); got == n {
		t.Error("nothing was read ahead for another repository")
	}
}

func TestPrefetchedModalOpensAtOnce(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 40, WithPrefetch(5, time.Millisecond))
	press(t, h, "down")
	seq, ok := sequence(h.Update(keyMsg("enter"))())
	if !ok || len(seq) != 2 {
		t.Fatal("enter should open the modal, then start its loads")
	}
	drain(t, h, seq[0])
	view := modalScreen(t, h)
	for _, want := range []string{"Retry GraphQL requests", "Cold starts read every page", "Does this survive a crash"} {
		if !strings.Contains(view, want) {
			t.Errorf("modal lacks %q before any load:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Loading…") {
		t.Errorf("modal waits for its detail:\n%s", view)
	}
}

func TestNoPrefetchByDefault(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 30)
	press(t, h, "down")
	if got := svc.got(); len(got) != 0 {
		t.Errorf("read details %v without WithPrefetch", got)
	}
}

// TestNotificationReadAheadOpensAtOnce reads a pull request ahead from a
// notification, of another repository than the one shown, and opens it
// from the notification: it shows at once, from what was read ahead.
func TestNotificationReadAheadOpensAtOnce(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 40)
	other := core.RepoRef{Owner: "charmbracelet", Name: "glow"}
	n := core.Notification{Repo: other, Subject: core.Subject{Type: core.SubjectPullRequest, Number: 142}, UpdatedAt: clock}
	o := threads.New(t.Context(), threads.WithPulls(svc), threads.WithPrefetch(1, time.Millisecond))
	drain(t, h, o.ReadAhead(func(i int) (core.Notification, bool) { return n, i == 0 }, n, false))
	if got := svc.got(); !slices.Equal(got, []int{142}) {
		t.Fatalf("read %v ahead, want #142", got)
	}

	seq, ok := sequence(h.Update(o.Open(n)())())
	if !ok || len(seq) != 2 {
		t.Fatal("the notification should open the modal, then start its loads")
	}
	drain(t, h, seq[0])
	view := modalScreen(t, h)
	for _, want := range []string{"Cold starts read every page", "Does this survive a crash", "It writes to a temporary file"} {
		if !strings.Contains(view, want) {
			t.Errorf("modal lacks %q before any load:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Loading…") {
		t.Errorf("modal waits for what was read ahead:\n%s", view)
	}
}
