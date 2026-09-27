package dashboard

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// detailFake records the reads of details and first comments, for the
// pull requests and issues services, and counts a read detail as cached.
type detailFake struct {
	mu       sync.Mutex
	gets     []details.Key
	comments int
	ctxs     []context.Context
	// hold, if set, holds every read of a detail until it is closed or the
	// read is canceled.
	hold chan struct{}
}

func (f *detailFake) get(ctx context.Context, k details.Key) error {
	f.mu.Lock()
	f.ctxs = append(f.ctxs, ctx)
	hold := f.hold
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, k)
	return nil
}

func (f *detailFake) comment() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comments++
}

func (f *detailFake) has(k details.Key) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.gets, k)
}

// numbers returns the numbers of the details read, in order.
func (f *detailFake) numbers() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	ns := make([]int, len(f.gets))
	for i, k := range f.gets {
		ns[i] = k.Number
	}
	return ns
}

type fakePulls struct{ *detailFake }

func (f fakePulls) Get(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error) {
	return core.PullRequestDetail{}, f.get(ctx, details.Key{Pull: true, Repo: repo, Number: number})
}

func (f fakePulls) Comments(context.Context, pulls.CommentsQuery) (core.Page[core.Comment], error) {
	f.comment()
	return core.Page[core.Comment]{}, nil
}

func (f fakePulls) Current(q pulls.CommentsQuery) bool {
	return f.has(details.Key{Pull: true, Repo: q.Repo, Number: q.Number})
}

type fakeIssues struct{ *detailFake }

func (f fakeIssues) Get(ctx context.Context, repo core.RepoRef, number int) (core.Issue, error) {
	return core.Issue{}, f.get(ctx, details.Key{Repo: repo, Number: number})
}

func (f fakeIssues) Comments(context.Context, issuesvc.CommentsQuery) (core.Page[core.Comment], error) {
	f.comment()
	return core.Page[core.Comment]{}, nil
}

func (f fakeIssues) Current(q issuesvc.CommentsQuery) bool {
	return f.has(details.Key{Repo: q.Repo, Number: q.Number})
}

// manyReviews is the work with six review requests, #1 to #6.
func manyReviews() core.Work {
	w := work()
	w.ReviewRequested.Items = nil
	for n := 1; n <= 6; n++ {
		w.ReviewRequested.Items = append(w.ReviewRequested.Items, hit(core.SearchPulls, "cli/cli", n, "Review me", false, time.Duration(n)*time.Hour))
	}
	w.ReviewRequested.Count = 6
	return w
}

// aheadSection returns a dashboard over the work of manyReviews that reads
// it ahead into f.
func aheadSection(t *testing.T, f *detailFake) *Section {
	t.Helper()
	svc := newFake()
	svc.work = manyReviews()
	return newSection(t, svc, nil, 140, 38, WithPrefetch(fakePulls{f}, fakeIssues{f}, 150*time.Millisecond))
}

func TestWorkReadsAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &detailFake{}
		s := aheadSection(t, f)
		if got := f.numbers(); len(got) != 0 {
			t.Fatalf("read %v ahead while the repositories have the focus, want nothing", got)
		}
		// The first three rows, once the pane has the focus.
		press(t, s, "3")
		if got := slices.Sorted(slices.Values(f.numbers())); !slices.Equal(got, []int{1, 2, 3}) {
			t.Fatalf("read %v ahead, want the first three rows", got)
		}
		if f.comments != 3 {
			t.Errorf("read %d first comments, want one per detail", f.comments)
		}
		// The row under the cursor, once it rests, and only once.
		press(t, s, "down", "down", "down", "up", "down")
		if got := f.numbers(); len(got) != 4 || got[3] != 4 {
			t.Errorf("read %v, want the fourth row last", got)
		}
		// The first rows of another list, once it is on view.
		press(t, s, "]")
		if got := f.numbers(); len(got) != 6 || !slices.Contains(got, 12) || !slices.Contains(got, 3) {
			t.Errorf("read %v, want the pull requests of the viewer too", got)
		}
		// An issue, from its own service.
		press(t, s, "]")
		if !f.has(details.Key{Repo: core.RepoRef{Owner: "octocat", Name: "hello-world"}, Number: 40}) {
			t.Errorf("read %v, want the assigned issue", f.numbers())
		}
	})
}

// Nothing is read ahead while another pane has the focus, or the dashboard
// is off screen, and a delay that fires then reads nothing.
func TestWorkReadsNothingAheadUnfocused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &detailFake{}
		s := aheadSection(t, f)
		press(t, s, "3", "down", "down", "down")
		before := len(f.numbers())
		// The cursor moves, and the pane loses the focus before it rests.
		rest := s.Update(keyPress("down"))
		press(t, s, "1")
		run(t, s, rest)
		if got := f.numbers(); len(got) != before {
			t.Errorf("read %v ahead after the pane lost the focus", got[before:])
		}
		// Back on the pane, the cursor's row waits to rest again.
		rest = s.Update(keyPress("3"))
		s.Blur()
		run(t, s, rest)
		if got := f.numbers(); len(got) != before {
			t.Errorf("read %v ahead off screen", got[before:])
		}
	})
}

// Leaving the dashboard cancels the reads ahead in flight, and coming back
// reads what they didn't.
func TestWorkStopsReadingAheadOnBlur(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &detailFake{hold: make(chan struct{})}
		s := aheadSection(t, f)
		reads := s.Update(keyPress("3"))
		done := make(chan struct{})
		go func() {
			run(t, s, reads)
			close(done)
		}()
		synctest.Wait()
		f.mu.Lock()
		ctxs := slices.Clone(f.ctxs)
		f.mu.Unlock()
		if len(ctxs) != 3 {
			t.Fatalf("%d reads in flight, want the first three rows", len(ctxs))
		}
		s.Blur()
		<-done
		for i, ctx := range ctxs {
			if ctx.Err() == nil {
				t.Errorf("read %d goes on off screen", i)
			}
		}
		close(f.hold)
		s.Focus()
		run(t, s, s.Update(ui.SyncMsg{Key: "other"}))
		if got := slices.Sorted(slices.Values(f.numbers())); !slices.Equal(got, []int{1, 2, 3}) {
			t.Errorf("read %v back on screen, want the first three rows", got)
		}
	})
}

// A pull request opened from the work holds the reads ahead of the work
// while its modal loads, and opening one read ahead counts as its use.
func TestWorkOpenPausesAndCounts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := obs.NewStats()
		prev := obs.SetDefault(stats)
		t.Cleanup(func() { obs.SetDefault(prev) })
		f := &detailFake{}
		s := aheadSection(t, f)
		press(t, s, "3")
		app := press(t, s, "enter")
		if len(app) != 1 {
			t.Fatalf("enter sent %v, want the pull request opened", app)
		}
		open, ok := app[0].(ui.OpenPullMsg)
		if !ok || open.Number != 1 || open.Pause == nil {
			t.Fatalf("enter sent %#v, want #1 opened with the reads ahead to hold", app[0])
		}
		if got := stats.Summary().Prefetch; len(got) != 1 || got[0].Kind != "work" || got[0].Opened != 1 {
			t.Errorf("summary = %+v, want #1 opened", got)
		}
		// The modal holds the reads while it loads, as the pull requests
		// section does.
		resume := ui.PauseAll(open.Pause)
		press(t, s, "down", "down")
		rest := s.Update(keyPress("down"))
		done := make(chan struct{})
		go func() {
			run(t, s, rest)
			close(done)
		}()
		// The cursor rests, and the read waits.
		time.Sleep(time.Second)
		synctest.Wait()
		if slices.Contains(f.numbers(), 4) {
			t.Error("read #4 ahead while the modal loads")
		}
		resume()
		<-done
		if !slices.Contains(f.numbers(), 4) {
			t.Error("didn't read #4 ahead once the modal loaded")
		}
	})
}
