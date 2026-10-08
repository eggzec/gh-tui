package search

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
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
	mu        sync.Mutex
	gets      []details.Key
	comments  int
	commented []details.Key
	ctxs      []context.Context
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

func (f *detailFake) comment(k details.Key) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comments++
	f.commented = append(f.commented, k)
}

func (f *detailFake) hasComments(k details.Key) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.commented, k)
}

func (f *detailFake) has(k details.Key) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.gets, k)
}

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

func (f fakePulls) Comments(_ context.Context, q pulls.CommentsQuery) (core.Page[core.Comment], error) {
	f.comment(details.Key{Pull: true, Repo: q.Repo, Number: q.Number})
	return core.Page[core.Comment]{}, nil
}

func (f fakePulls) CurrentGet(repo core.RepoRef, number int) bool {
	return f.has(details.Key{Pull: true, Repo: repo, Number: number})
}

func (f fakePulls) CurrentComments(q pulls.CommentsQuery) bool {
	return f.hasComments(details.Key{Pull: true, Repo: q.Repo, Number: q.Number})
}

type fakeIssues struct{ *detailFake }

func (f fakeIssues) Get(ctx context.Context, repo core.RepoRef, number int) (core.Issue, error) {
	return core.Issue{}, f.get(ctx, details.Key{Repo: repo, Number: number})
}

func (f fakeIssues) Comments(_ context.Context, q issuesvc.CommentsQuery) (core.Page[core.Comment], error) {
	f.comment(details.Key{Repo: q.Repo, Number: q.Number})
	return core.Page[core.Comment]{}, nil
}

func (f fakeIssues) CurrentGet(repo core.RepoRef, number int) bool {
	return f.has(details.Key{Repo: repo, Number: number})
}

func (f fakeIssues) CurrentComments(q issuesvc.CommentsQuery) bool {
	return f.hasComments(details.Key{Repo: q.Repo, Number: q.Number})
}

// aheadSection returns a page that reads the results ahead into f, with
// the pull requests for tea on view and the kinds focused.
func aheadSection(t *testing.T, f *detailFake) *Section {
	t.Helper()
	p := config.Default().Prefetch
	p.Rest = 150 * time.Millisecond
	// The other kinds are read at once, as newSection reads them.
	p.Search.OtherKinds.Rest = new(time.Duration(0))
	s := newSection(t, newFake(), 120, 30, WithDetails(fakePulls{f}, fakeIssues{f}), WithPrefetch(p))
	typeText(t, s, "tea")
	// The query has the focus, in normal mode, on the pull requests.
	press(t, s, "esc", "1", "]", "]")
	return s
}

func TestResultsReadAheadOnHover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &detailFake{}
		s := aheadSection(t, f)
		if got := f.numbers(); len(got) != 0 {
			t.Fatalf("read %v ahead before the cursor rested on a result, want nothing", got)
		}
		// The result under the cursor, once the results have the focus.
		press(t, s, "2")
		if got := f.numbers(); !slices.Equal(got, []int{1402}) || f.comments != 1 {
			t.Fatalf("read %v and %d comments ahead, want #1402 and its comments", got, f.comments)
		}
		press(t, s, "down", "up", "down")
		if got := f.numbers(); !slices.Equal(got, []int{1402, 1388}) {
			t.Errorf("read %v ahead, want #1388 once", got)
		}
		// An issue, from its own service, and never a repository.
		press(t, s, "1", "[", "2")
		press(t, s, "1", "[", "2", "down")
		if got := f.numbers(); !slices.Equal(got, []int{1402, 1388, 1203}) {
			t.Errorf("read %v ahead, want the issue and no repository", got)
		}
	})
}

// A result the cursor rests on while the query has the focus, or off the page, reads nothing.
func TestResultsReadNothingAheadUnfocused(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &detailFake{}
		s := aheadSection(t, f)
		press(t, s, "2")
		before := len(f.numbers())
		rest := s.Update(keyPress("down"))
		press(t, s, "1")
		run(t, s, rest)
		rest = s.Update(keyPress("2"))
		s.Blur()
		run(t, s, rest)
		if got := f.numbers(); len(got) != before {
			t.Errorf("read %v ahead off the results", got[before:])
		}
	})
}

// Leaving the page cancels the read in flight.
func TestResultsStopReadingAheadOnBlur(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &detailFake{hold: make(chan struct{})}
		s := aheadSection(t, f)
		reads := s.Update(keyPress("2"))
		done := make(chan struct{})
		go func() {
			run(t, s, reads)
			close(done)
		}()
		// The cursor rests, and the read is sent.
		time.Sleep(time.Second)
		synctest.Wait()
		f.mu.Lock()
		ctxs := slices.Clone(f.ctxs)
		f.mu.Unlock()
		if len(ctxs) != 1 {
			t.Fatalf("%d reads in flight, want the result under the cursor", len(ctxs))
		}
		s.Blur()
		<-done
		if ctxs[0].Err() == nil {
			t.Error("the read goes on off the page")
		}
	})
}

// A pull request opened from the results holds the reads ahead while its
// modal loads, and opening one read ahead counts as its use.
func TestResultsOpenPausesAndCounts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := countStats(t)
		f := &detailFake{}
		s := aheadSection(t, f)
		press(t, s, "2")
		app := press(t, s, "enter")
		open, ok := app[0].(ui.OpenPullMsg)
		if len(app) != 1 || !ok || open.Number != 1402 || open.Pause == nil {
			t.Fatalf("enter sent %#v, want #1402 opened with the reads ahead to hold", app)
		}
		got := stats.Summary().Prefetch
		if i := slices.IndexFunc(got, func(p obs.PrefetchStats) bool { return p.Kind == "search_hit" }); i < 0 || got[i].Opened != 1 {
			t.Errorf("summary = %+v, want #1402 opened", got)
		}
		resume := ui.PauseAll(open.Pause)
		rest := s.Update(keyPress("down"))
		done := make(chan struct{})
		go func() {
			run(t, s, rest)
			close(done)
		}()
		// The cursor rests, and the read waits.
		time.Sleep(time.Second)
		synctest.Wait()
		if slices.Contains(f.numbers(), 1388) {
			t.Error("read #1388 ahead while the modal loads")
		}
		resume()
		<-done
		if !slices.Contains(f.numbers(), 1388) {
			t.Error("didn't read #1388 ahead once the modal loaded")
		}
	})
}
