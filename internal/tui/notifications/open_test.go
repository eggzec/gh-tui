package notifications

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/threads"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fakePulls records the pull requests read ahead.
type fakePulls struct {
	mu    sync.Mutex
	reads []int
}

func (f *fakePulls) Get(_ context.Context, _ core.RepoRef, number int) (core.PullRequestDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, number)
	return core.PullRequestDetail{}, nil
}

func (f *fakePulls) Comments(context.Context, pulls.CommentsQuery) (core.Page[core.Comment], error) {
	return core.Page[core.Comment]{}, nil
}

func (f *fakePulls) Current(q pulls.CommentsQuery) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.reads, q.Number)
}

func (*fakePulls) Changed(core.RepoRef, int, time.Time) {}

func (f *fakePulls) got() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reads)
}

func newSectionWith(tb testing.TB, svc Service, o *threads.Opener) *Section {
	tb.Helper()
	s := New(tb.Context(), svc, config.Default().Keys, WithNow(func() time.Time { return now }), WithOpener(o))
	s.SetSize(100, 12)
	s.Focus()
	run(tb, s, s.Init())
	return s
}

func TestMarkReadOnOpenOff(t *testing.T) {
	svc := newFake(inbox()...)
	s := newSectionWith(t, svc, threads.New(t.Context(), threads.WithMarkRead(false)))
	app := press(t, s, "enter")
	if len(svc.reads) != 0 {
		t.Errorf("marked %q read, want none with mark_read_on_open off", svc.reads)
	}
	if !slices.Contains(app, any(ui.OpenPullMsg{Repo: inbox()[0].Repo, Number: 1})) {
		t.Errorf("enter sent %v, want the pull request opened", app)
	}
	if got := s.keys.Select.Help().Desc; got != "open" {
		t.Errorf("select is described as %q, want open", got)
	}

	on := newSectionWith(t, newFake(inbox()...), nil)
	if got := on.keys.Select.Help().Desc; got != "open & read" {
		t.Errorf("select is described as %q, want open & read", got)
	}
}

func TestPrefetchFirstThreadsAndHover(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ps := &fakePulls{}
		ts := []core.Notification{
			thread("1", "charmbracelet/bubbletea", core.SubjectPullRequest, "a", "mention", true, time.Minute),
			thread("2", "charmbracelet/bubbletea", core.SubjectDiscussion, "b", "mention", true, time.Minute),
			thread("3", "charmbracelet/bubbletea", core.SubjectPullRequest, "c", "mention", true, time.Minute),
			thread("4", "charmbracelet/bubbletea", core.SubjectPullRequest, "d", "mention", true, time.Minute),
		}
		s := newSectionWith(t, newFake(ts...), threads.New(t.Context(), threads.WithPulls(ps), threads.WithPrefetch(3, 150*time.Millisecond)))
		// The first three rows: the discussion has nothing to read.
		if got := slices.Sorted(slices.Values(ps.got())); !slices.Equal(got, []int{1, 3}) {
			t.Errorf("read %v ahead, want 1 and 3", got)
		}
		start := time.Now()
		press(t, s, "down", "down", "down")
		if waited := time.Since(start); waited != 150*time.Millisecond {
			t.Errorf("read after %v, want the delay once the cursor rests", waited)
		}
		if got := ps.got(); len(got) != 3 || got[2] != 4 {
			t.Errorf("read %v, want the row under the cursor last", got)
		}
	})
}

// heldPulls holds every read ahead until it is cancelled, and keeps its
// context.
type heldPulls struct {
	fakePulls
	ctxs chan context.Context
}

func (f *heldPulls) Get(ctx context.Context, _ core.RepoRef, _ int) (core.PullRequestDetail, error) {
	f.ctxs <- ctx
	<-ctx.Done()
	return core.PullRequestDetail{}, ctx.Err()
}

// Leaving the screen cancels the reads ahead in flight.
func TestBlurCancelsReadsAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ps := &heldPulls{ctxs: make(chan context.Context, 4)}
		svc := newFake()
		s := newSectionWith(t, svc, threads.New(t.Context(), threads.WithPulls(ps), threads.WithPrefetch(3, time.Hour)))
		svc.mu.Lock()
		svc.threads = []core.Notification{thread("1", "charmbracelet/bubbletea", core.SubjectPullRequest, "a", "mention", true, time.Minute)}
		svc.mu.Unlock()
		done := make(chan struct{})
		go func() {
			defer close(done)
			run(t, s, s.Update(ui.SyncMsg{Key: SyncKey}))
		}()
		synctest.Wait()
		var ctx context.Context
		select {
		case ctx = <-ps.ctxs:
		default:
			t.Fatal("nothing was read ahead")
		}
		s.Blur()
		<-done
		if ctx.Err() == nil {
			t.Error("the read ahead wasn't cancelled when the section left the screen")
		}
	})
}

// A change to the inbox while another screen is on view reads nothing
// ahead; the reads resume once the section is back.
func TestNoReadsAheadOffScreen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ps := &fakePulls{}
		svc := newFake()
		s := newSectionWith(t, svc, threads.New(t.Context(), threads.WithPulls(ps), threads.WithPrefetch(3, time.Hour)))
		s.Blur()
		svc.mu.Lock()
		svc.threads = []core.Notification{thread("1", "charmbracelet/bubbletea", core.SubjectPullRequest, "a", "mention", true, time.Minute)}
		svc.mu.Unlock()
		run(t, s, s.Update(ui.SyncMsg{Key: SyncKey}))
		if got := ps.got(); len(got) != 0 {
			t.Fatalf("read %v ahead off screen, want nothing", got)
		}
		s.Focus()
		run(t, s, s.Update(ui.SyncMsg{Key: "other"}))
		if got := ps.got(); !slices.Equal(got, []int{1}) {
			t.Errorf("read %v ahead back on screen, want 1", got)
		}
	})
}
