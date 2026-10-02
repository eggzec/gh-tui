package pulls

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fakeChecks serves the checks of every pull request, with a failing
// check of another app, so that nothing more is read. It keeps the
// numbers it read and the contexts it read them with, and fresh are the
// numbers whose checks it reports fresh, of the head its checks name.
// While hold is set, a read waits until its context ends.
type fakeChecks struct {
	mu    sync.Mutex
	read  []int
	ctxs  []context.Context
	fresh map[int]bool
	hold  bool
}

// numbers returns the numbers whose checks were read, in order.
func (f *fakeChecks) numbers() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.read)
}

func (f *fakeChecks) value() core.Checks {
	return core.Checks{SHA: "abc", Total: 2, Runs: []core.Check{
		{ID: 1, Name: "codecov", Status: core.RunCompleted, Conclusion: core.ConclusionFailure, Summary: "Coverage fell"},
		{ID: 2, Name: "dco", Status: core.RunCompleted, Conclusion: core.ConclusionSuccess},
	}}
}

func (f *fakeChecks) CachedChecks(q actionssvc.ChecksQuery) (core.Checks, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.value(), slices.Contains(f.read, q.Number)
}
func (f *fakeChecks) FreshChecks(q actionssvc.ChecksQuery) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.fresh[q.Number] && (q.SHA == "" || q.SHA == f.value().SHA)
}
func (f *fakeChecks) Checks(ctx context.Context, q actionssvc.ChecksQuery) (core.Checks, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.read = append(f.read, q.Number)
	f.ctxs = append(f.ctxs, ctx)
	if f.hold {
		f.mu.Unlock()
		<-ctx.Done()
		f.mu.Lock()
		return core.Checks{}, ctx.Err()
	}
	if f.fresh == nil {
		f.fresh = map[int]bool{}
	}
	f.fresh[q.Number] = true
	return f.value(), nil
}
func (f *fakeChecks) CachedRun(core.RepoRef, int64) (core.Run, bool) { return core.Run{}, false }
func (f *fakeChecks) Run(context.Context, core.RepoRef, int64) (core.Run, error) {
	return core.Run{}, nil
}
func (f *fakeChecks) CachedAllJobs(actionssvc.JobsQuery) (core.Page[core.Job], bool) {
	return core.Page[core.Job]{}, false
}
func (f *fakeChecks) AllJobs(context.Context, actionssvc.JobsQuery) (core.Page[core.Job], error) {
	return core.Page[core.Job]{}, nil
}
func (f *fakeChecks) CachedLog(core.RepoRef, int64) (core.Log, bool) { return core.Log{}, false }
func (f *fakeChecks) Log(context.Context, core.RepoRef, int64) (core.Log, error) {
	return core.Log{}, nil
}
func (f *fakeChecks) CachedPartialLog(core.RepoRef, int64) (core.PartialLog, bool) {
	return core.PartialLog{}, false
}
func (f *fakeChecks) PartialLog(context.Context, core.RepoRef, int64) (core.PartialLog, error) {
	return core.PartialLog{}, core.ErrLogPending
}
func (f *fakeChecks) WatchLog(core.RepoRef, int64, int64) func() { return func() {} }
func (f *fakeChecks) CachedAnnotations(actionssvc.AnnotationsQuery) (core.Page[core.Annotation], bool) {
	return core.Page[core.Annotation]{}, false
}
func (f *fakeChecks) Annotations(context.Context, actionssvc.AnnotationsQuery) (core.Page[core.Annotation], error) {
	return core.Page[core.Annotation]{}, nil
}
func (f *fakeChecks) Invalidate(core.RepoRef) {}
func (f *fakeChecks) RerunFailedJobs(core.RepoRef, int64) *optimistic.Op {
	return optimistic.New(func(context.Context) error { return nil })
}

func modalText(h *host) string {
	return strings.Join(strings.Fields(ansi.Strip(h.modals[len(h.modals)-1].View())), " ")
}

func TestChecksKeyOnARowOpensTheChecks(t *testing.T) {
	h := started(t, newFakeService(), 100, 30, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)))
	press(t, h, "C")
	m := h.modal()
	if m == nil || m.number != 142 || m.checks == nil {
		t.Fatalf("C opened %v, want #142 on its checks", m)
	}
	if v := modalText(h); !strings.Contains(v, "Checks ✗ 1 failing, ✓ 1 passed") || !strings.Contains(v, "codecov") {
		t.Errorf("the modal doesn't show the checks:\n%s", v)
	}
	// esc steps back to the detail, whose header counts them too.
	press(t, h, "esc")
	if m.checks != nil || h.modal() != m {
		t.Fatal("esc from the checks didn't step back to the detail")
	}
	if v := modalText(h); !strings.Contains(v, "CI ✗ 1 failing, ✓ 1 passed · C for details") {
		t.Errorf("the header doesn't count the checks:\n%s", v)
	}
	press(t, h, "C")
	if m.checks == nil {
		t.Fatal("C in the detail didn't open the checks")
	}
	press(t, h, "esc")
	press(t, h, "esc")
	if h.modal() != nil {
		t.Error("esc from the detail didn't close the modal")
	}
}

func TestOpenPullOnItsChecks(t *testing.T) {
	h := started(t, newFakeService(), 100, 30, WithChecks(&fakeChecks{}), WithIcons(ui.NewIcons(config.IconsUnicode)))
	drain(t, h, h.Update(ui.OpenPullMsg{Repo: repo, Number: 135, Checks: true}))
	if m := h.modal(); m == nil || m.number != 135 || m.checks == nil {
		t.Fatalf("OpenPullMsg opened %v, want #135 on its checks", m)
	}
	// The step's messages go through the modal: enter shows the detail of
	// the failing check.
	press(t, h, "enter")
	if v := modalText(h); !strings.Contains(v, "Coverage fell") {
		t.Errorf("enter didn't show the check:\n%s", v)
	}
}

func TestChecksKeyWithoutChecks(t *testing.T) {
	h := started(t, newFakeService(), 100, 30)
	press(t, h, "C")
	if h.modal() != nil {
		t.Error("C opened a modal without checks")
	}
	press(t, h, "enter")
	press(t, h, "C")
	if m := h.modal(); m == nil || m.checks != nil {
		t.Error("C opened checks in the detail without them")
	}
}

var _ ChecksService = (*fakeChecks)(nil)

// readingChecks returns the option that reads only the checks ahead, in
// their default window, once the cursor rests for 150ms.
func readingChecks() Option {
	p := prefetchOf(4, 150*time.Millisecond)
	p.Pulls.Details.Enabled, p.Pulls.Comments.Enabled = new(false), new(false)
	p.Pulls.Checks.Enabled = new(true)
	return WithPrefetch(p)
}

func TestPrefetchChecks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, c := newFakeService(), &fakeChecks{fresh: map[int]bool{135: true}}
		h := started(t, svc, 80, 30, WithChecks(c), readingChecks())
		// The row under the cursor and the two below it, but #135, whose
		// checks are fresh; none above, and no details.
		if got, want := sorted(c.numbers()), []int{128, 142}; !slices.Equal(got, want) {
			t.Errorf("read the checks of %v, want %v", got, want)
		}
		if got := svc.got(); len(got) != 0 {
			t.Errorf("read details %v, want none while they are off", got)
		}
		start := time.Now()
		press(t, h, "down")
		if waited := time.Since(start); waited != 150*time.Millisecond {
			t.Errorf("read after %v, want the rest", waited)
		}
		if got, want := c.numbers()[2:], []int{121}; !slices.Equal(got, want) {
			t.Errorf("moving down read the checks of %v, want %v", got, want)
		}
	})
}

func TestNoChecksPrefetchByDefault(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc, c := newFakeService(), &fakeChecks{}
		h := started(t, svc, 80, 30, WithChecks(c), readingAhead(2, 150*time.Millisecond, false))
		press(t, h, "down")
		if got := c.numbers(); len(got) != 0 {
			t.Errorf("read the checks of %v while prefetch.pulls.checks is off", got)
		}
		if len(svc.got()) == 0 {
			t.Error("read no details ahead")
		}
	})
}

// TestChecksPrefetchWithoutChecks checks that a section that shows no
// checks reads none ahead, whatever the settings say.
func TestChecksPrefetchWithoutChecks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := started(t, newFakeService(), 80, 30, readingChecks())
		if h.ahead.On() {
			t.Error("reads checks ahead without WithChecks")
		}
	})
}

// TestChecksPrefetchCancelledByRepo holds a read of checks ahead in
// flight and switches the repository: the read ends, cancelled.
func TestChecksPrefetchCancelledByRepo(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &fakeChecks{}
		h := started(t, newFakeService(), 80, 30, WithChecks(c), readingChecks())
		c.mu.Lock()
		c.hold = true
		c.mu.Unlock()
		// The first rows are read; moving down reads #121, which waits.
		read := rested(t, h.Section, h.Section.Update(keyMsg("down")))
		done := make(chan struct{})
		go func() {
			read()
			close(done)
		}()
		synctest.Wait()
		c.mu.Lock()
		held := c.ctxs[len(c.ctxs)-1]
		c.mu.Unlock()
		if got := c.numbers(); got[len(got)-1] != 121 || held.Err() != nil {
			t.Fatalf("read the checks of %v, want #121 in flight", got)
		}
		drain(t, h, h.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}}))
		<-done
		if held.Err() == nil {
			t.Error("the read of checks ahead of the old repository wasn't cancelled")
		}
	})
}

// TestChecksOfAnotherHeadAreReadAgain checks that fresh checks of a head
// the row no longer shows, since a push moved it, are read again.
func TestChecksOfAnotherHeadAreReadAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		// The checks of #135 and #128 are fresh, but #135 was pushed to
		// since: its row shows another head than they name.
		svc.pulls[1].HeadSHA, svc.pulls[2].HeadSHA = "def", "abc"
		c := &fakeChecks{fresh: map[int]bool{135: true, 128: true}}
		started(t, svc, 80, 30, WithChecks(c), readingChecks())
		if got, want := sorted(c.numbers()), []int{135, 142}; !slices.Equal(got, want) {
			t.Errorf("read the checks of %v, want %v", got, want)
		}
	})
}

// TestChecksReadAheadShowInTheModal opens a pull request whose checks
// were read ahead: the header of its modal counts them at once, without
// reading them again.
func TestChecksReadAheadShowInTheModal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := &fakeChecks{}
		h := started(t, newFakeService(), 100, 40, WithChecks(c), readingChecks(), WithIcons(ui.NewIcons(config.IconsUnicode)))
		n := len(c.numbers())
		press(t, h, "enter")
		if v := modalText(h); !strings.Contains(v, "CI ✗ 1 failing, ✓ 1 passed") {
			t.Errorf("the header doesn't count the checks read ahead:\n%s", v)
		}
		if got := len(c.numbers()); got != n {
			t.Errorf("opening the modal read %d more checks, want none", got-n)
		}
	})
}

// TestChecksHeadsApartByRepository switches to another repository whose
// pull requests have the same numbers: a read ahead of the first one's
// still in flight doesn't take the head of the other's.
func TestChecksHeadsApartByRepository(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService()
		svc.pulls[0].HeadSHA = "abc"
		h := started(t, svc, 80, 30, WithChecks(&fakeChecks{}), readingChecks())
		if q := h.checksQuery(detailKey(repo, 142)); q.SHA != "abc" {
			t.Fatalf("the checks of #142 are read at %q, want abc", q.SHA)
		}
		other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
		svc.mu.Lock()
		for i := range svc.pulls {
			svc.pulls[i].Repo, svc.pulls[i].HeadSHA = other, "def"
		}
		svc.mu.Unlock()
		drain(t, h, h.Update(ui.RepoMsg{Repo: other}))
		if q := h.checksQuery(detailKey(other, 142)); q.SHA != "def" {
			t.Errorf("the checks of the other #142 are read at %q, want def", q.SHA)
		}
		if q := h.checksQuery(detailKey(repo, 142)); q.SHA == "def" {
			t.Error("the checks of the first #142 are read at the head of the other's")
		}
	})
}
