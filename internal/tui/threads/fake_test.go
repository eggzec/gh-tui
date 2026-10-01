package threads

import (
	"context"
	"log/slog"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// TestMain drops what the reads ahead log.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

var now = time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)

// fakeReads records what was read of each kind, as "pull o/r#1", and
// counts a read as current from then on, unless a later change reported
// is newer.
type fakeReads struct {
	mu sync.Mutex
	// reads are the detail reads, and comments those of first comments.
	reads    []string
	comments []string
	changes  []string
	// at is when each item was read, and ctxs the contexts of the reads.
	at   map[string]time.Time
	ctxs []context.Context
	// err fails every read.
	err error
	// hold, if set, holds each read until it is closed or the read is
	// cancelled.
	hold chan struct{}
}

func newReads() *fakeReads {
	return &fakeReads{at: map[string]time.Time{}}
}

func (f *fakeReads) read(ctx context.Context, name string) error {
	f.mu.Lock()
	f.reads = append(f.reads, name)
	f.ctxs = append(f.ctxs, ctx)
	hold, err := f.hold, f.err
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.at[name] = now
	return nil
}

func (f *fakeReads) comment(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.comments = append(f.comments, name)
}

// commented reports whether the first comments of name were read.
func (f *fakeReads) commented(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.comments, name)
}

func (f *fakeReads) current(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.at[name]
	return ok
}

// changed forgets a read older than updated, as the services mark it
// stale.
func (f *fakeReads) changed(name string, updated time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.changes = append(f.changes, name+" "+updated.Format(time.TimeOnly))
	if at, ok := f.at[name]; ok && at.Before(updated) {
		delete(f.at, name)
	}
}

func (f *fakeReads) got() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reads)
}

func (f *fakeReads) contexts() []context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.ctxs)
}

func itemName(kind string, repo core.RepoRef, n int64) string {
	return kind + " " + repo.String() + "#" + strconv.FormatInt(n, 10)
}

type fakePulls struct{ *fakeReads }

func (f fakePulls) Get(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error) {
	return core.PullRequestDetail{}, f.read(ctx, itemName("pull", repo, int64(number)))
}

func (f fakePulls) Comments(_ context.Context, q pulls.CommentsQuery) (core.Page[core.Comment], error) {
	if q != (pulls.CommentsQuery{Repo: q.Repo, Number: q.Number}) {
		panic("the first comments are read as the modal reads them")
	}
	f.comment(itemName("pull", q.Repo, int64(q.Number)))
	return core.Page[core.Comment]{}, nil
}

func (f fakePulls) CurrentGet(repo core.RepoRef, number int) bool {
	return f.current(itemName("pull", repo, int64(number)))
}

func (f fakePulls) CurrentComments(q pulls.CommentsQuery) bool {
	return f.commented(itemName("pull", q.Repo, int64(q.Number)))
}

func (f fakePulls) Changed(repo core.RepoRef, number int, updated time.Time) {
	f.changed(itemName("pull", repo, int64(number)), updated)
}

type fakeIssues struct{ *fakeReads }

func (f fakeIssues) Get(ctx context.Context, repo core.RepoRef, number int) (core.Issue, error) {
	return core.Issue{}, f.read(ctx, itemName("issue", repo, int64(number)))
}

func (f fakeIssues) Comments(_ context.Context, q issuesvc.CommentsQuery) (core.Page[core.Comment], error) {
	if q != (issuesvc.CommentsQuery{Repo: q.Repo, Number: q.Number}) {
		panic("the first comments are read as the modal reads them")
	}
	f.comment(itemName("issue", q.Repo, int64(q.Number)))
	return core.Page[core.Comment]{}, nil
}

func (f fakeIssues) CurrentGet(repo core.RepoRef, number int) bool {
	return f.current(itemName("issue", repo, int64(number)))
}

func (f fakeIssues) CurrentComments(q issuesvc.CommentsQuery) bool {
	return f.commented(itemName("issue", q.Repo, int64(q.Number)))
}

func (f fakeIssues) Changed(repo core.RepoRef, number int, updated time.Time) {
	f.changed(itemName("issue", repo, int64(number)), updated)
}

type fakeReleases struct{ *fakeReads }

func (f fakeReleases) Get(ctx context.Context, repo core.RepoRef, id int64) (core.Release, error) {
	return core.Release{}, f.read(ctx, itemName("release", repo, id))
}

func (f fakeReleases) Current(repo core.RepoRef, id int64) bool {
	return f.current(itemName("release", repo, id))
}

// newOpener returns an opener over f for every kind, which reads the row
// under the cursor and the after rows below it ahead once the cursor rests
// for rest.
func newOpener(tb testing.TB, f *fakeReads, after int, rest time.Duration) *Opener {
	tb.Helper()
	p := config.Default().Prefetch
	p.Window, p.Rest = config.Window{After: after}, rest
	return New(tb.Context(),
		WithPulls(fakePulls{f}), WithIssues(fakeIssues{f}), WithReleases(fakeReleases{f}),
		WithPrefetch(p),
	)
}

var glow = core.RepoRef{Owner: "charmbracelet", Name: "glow"}

// note returns a notification of glow about typ, numbered n, updated ago
// before now.
func note(typ core.SubjectType, n int, ago time.Duration) core.Notification {
	sub := core.Subject{Type: typ, Title: "thread " + strconv.Itoa(n), WebURL: "https://github.com/charmbracelet/glow/" + strconv.Itoa(n)}
	switch typ {
	case core.SubjectIssue, core.SubjectPullRequest:
		sub.Number = n
	case core.SubjectRelease:
		sub.ReleaseID = int64(n)
	case core.SubjectCommit:
		sub.SHA = "c0ffee" + strconv.Itoa(n)
	default:
	}
	return core.Notification{ID: strconv.Itoa(n), Repo: glow, Subject: sub, Unread: true, UpdatedAt: now.Add(-ago)}
}

// list lets ReadAhead read ns by index.
func list(ns []core.Notification) func(int) (core.Notification, bool) {
	return func(i int) (core.Notification, bool) {
		if i < 0 || i >= len(ns) {
			return core.Notification{}, false
		}
		return ns[i], true
	}
}

// run runs cmd until nothing is left, and gives each rest to o, as a
// section does. It returns the other messages.
func run(o *Opener, cmd tea.Cmd) []tea.Msg {
	var out []tea.Msg
	queue := []tea.Cmd{cmd}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case nil:
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case ui.AheadMsg:
			queue = append(queue, o.Rested(msg))
		default:
			out = append(out, msg)
		}
	}
	return out
}
