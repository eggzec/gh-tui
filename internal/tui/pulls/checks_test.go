package pulls

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

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
	// job makes the failing check a job of GitHub Actions with a log, so
	// that it opens into its log and can be re-run.
	job bool
	// pending adds a check that is still running, which the step polls.
	pending bool
	// commits are the SHAs whose checks were read as a commit's.
	commits []string
}

// numbers returns the numbers whose checks were read, in order.
func (f *fakeChecks) numbers() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.read)
}

func (f *fakeChecks) value() core.Checks {
	failing := core.Check{ID: 1, Name: "codecov", Status: core.RunCompleted, Conclusion: core.ConclusionFailure, Summary: "Coverage fell"}
	if f.job {
		failing.JobID, failing.RunID = 5, 9
	}
	c := core.Checks{SHA: "abc", Total: 2, Runs: []core.Check{
		failing,
		{ID: 2, Name: "dco", Status: core.RunCompleted, Conclusion: core.ConclusionSuccess},
	}}
	if f.pending {
		c.Total++
		c.Runs = append(c.Runs, core.Check{ID: 3, Name: "slow", Status: core.RunInProgress})
	}
	return c
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
	if f.job {
		return core.Run{ID: 9, Attempt: 1, Name: "CI", Status: core.RunCompleted, Conclusion: core.ConclusionFailure}, nil
	}
	return core.Run{}, nil
}
func (f *fakeChecks) CachedAllJobs(actionssvc.JobsQuery) (core.Page[core.Job], bool) {
	return core.Page[core.Job]{}, false
}
func (f *fakeChecks) AllJobs(context.Context, actionssvc.JobsQuery) (core.Page[core.Job], error) {
	if !f.job {
		return core.Page[core.Job]{}, nil
	}
	return core.Page[core.Job]{Items: []core.Job{{ID: 5, RunID: 9, Attempt: 1, Name: "codecov", Status: core.RunCompleted, Conclusion: core.ConclusionFailure}}}, nil
}
func (f *fakeChecks) CachedLog(core.RepoRef, int64) (core.Log, bool) { return core.Log{}, false }
func (f *fakeChecks) Log(context.Context, core.RepoRef, int64) (core.Log, error) {
	return core.Log{Lines: []core.LogLine{{Text: "go test ./...", Step: 1}, {Text: "FAIL", Step: 1}}}, nil
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

// commitChecks returns the checks of a commit: one failing job of GitHub
// Actions.
func (f *fakeChecks) commitChecks(sha string) core.CommitChecks {
	return core.CommitChecks{SHA: sha, Total: 1, Runs: []core.CommitCheck{{
		ID: 5, JobID: 5, RunID: 9, Name: "build", Workflow: "CI", RunNumber: 1,
		Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
	}}}
}

func (f *fakeChecks) CachedCommitChecks(actionssvc.CommitChecksQuery) (core.CommitChecks, bool) {
	return core.CommitChecks{}, false
}

func (f *fakeChecks) CommitChecks(_ context.Context, q actionssvc.CommitChecksQuery) (core.CommitChecks, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commits = append(f.commits, q.SHA)
	return f.commitChecks(q.SHA), nil
}

func (f *fakeChecks) Invalidate(core.RepoRef) {}
func (f *fakeChecks) RerunFailedJobs(core.RepoRef, int64) *optimistic.Op {
	return optimistic.New(func(context.Context) error { return nil })
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
