package checktree

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

const sha2 = "fedcba9876543210fedcba9876543210fedcba98"

// fakeReader answers reads from what it is given and counts them.
type fakeReader struct {
	mu     sync.Mutex
	cached map[string]core.CommitChecks
	errs   map[string]error
	reads  []string
	ctx    context.Context // of the last read
}

func (f *fakeReader) CachedCommitChecks(q Query) (core.CommitChecks, bool) {
	c, ok := f.cached[q.SHA]
	return c, ok
}

func (f *fakeReader) CommitChecks(ctx context.Context, q Query) (core.CommitChecks, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, q.SHA)
	f.ctx = ctx
	if err := f.errs[q.SHA]; err != nil {
		return core.CommitChecks{}, err
	}
	c := commit([]core.CommitCheck{job(1, 1, "build", core.ConclusionSuccess)})
	c.SHA = q.SHA
	return c, nil
}

func (f *fakeReader) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.reads)
}

var repo = core.RepoRef{Owner: "o", Name: "r"}

// run runs cmd and returns its message.
func run(tb testing.TB, cmd tea.Cmd) tea.Msg {
	tb.Helper()
	if cmd == nil {
		tb.Fatal("no command")
	}
	return cmd()
}

func TestLoaderHandles(t *testing.T) {
	t.Parallel()
	l, cmd := NewLoader(&fakeReader{}, repo).Want(sha, false)
	msg := run(t, cmd)
	if !l.Handles(msg) {
		t.Error("the loader doesn't handle the answer to its own read")
	}
	other, _ := NewLoader(&fakeReader{}, repo).Want(sha, false)
	if other.Handles(msg) {
		t.Error("a loader handles the answer to the read of another")
	}
	if l.Handles(ui.OnlineMsg{}) {
		t.Error("the loader handles a message that isn't an answer")
	}
}

func TestLoaderWantReadsOnce(t *testing.T) {
	t.Parallel()
	f := &fakeReader{}
	l, cmd := NewLoader(f, repo).Want(sha, false)
	l, again := l.Want(sha, false)
	if again != nil {
		t.Error("a commit that loads asked again")
	}
	if rows, _ := l.Rows(sha, opts(), styles(config.IconsASCII)); len(rows) != 1 || !rows[0].Skip {
		t.Errorf("rows while loading = %v, want one skipped note", first(rows))
	}
	l, next := l.Update(run(t, cmd))
	if next != nil {
		t.Error("a reply asked for more")
	}
	if l, again = l.Want(sha, false); again != nil {
		t.Error("a loaded commit asked again")
	}
	if n := f.count(); n != 1 {
		t.Errorf("reads = %d, want 1", n)
	}
	rows, _ := l.Rows(sha, opts(), styles(config.IconsASCII))
	if len(rows) == 0 || rows[0].Skip {
		t.Errorf("rows once loaded = %v, want the tree", first(rows))
	}
}

func TestLoaderCachedNeedsNoRead(t *testing.T) {
	t.Parallel()
	f := &fakeReader{cached: map[string]core.CommitChecks{sha: commit([]core.CommitCheck{job(1, 1, "build", core.ConclusionFailure)})}}
	l, cmd := NewLoader(f, repo).Want(sha, true)
	if cmd != nil {
		t.Error("a kept commit returned a command")
	}
	if rows, _ := l.Rows(sha, opts(), styles(config.IconsASCII)); len(rows) == 0 {
		t.Error("a kept commit has no rows")
	}
	if n := f.count(); n != 0 {
		t.Errorf("reads = %d, want 0", n)
	}
}

func TestLoaderNeverAskedHasNoRows(t *testing.T) {
	t.Parallel()
	rows, targets := NewLoader(&fakeReader{}, repo).Rows(sha, opts(), styles(config.IconsASCII))
	if rows != nil || targets != nil {
		t.Errorf("rows = %v, targets = %v, want none", rows, targets)
	}
}

func TestLoaderReopenedTakesUpWhatIsKept(t *testing.T) {
	t.Parallel()
	f := &fakeReader{}
	l, cmd := NewLoader(f, repo).Want(sha, false)
	l, _ = l.Update(run(t, cmd))
	text := func(l Loader) string {
		rows, _ := l.Rows(sha, opts(), styles(config.IconsASCII))
		var b strings.Builder
		for _, r := range rows {
			b.WriteString(strings.Join(r.Lines, "\n"))
		}
		return b.String()
	}
	before := text(l)

	// Another modal changed the checks of the commit while this one was hidden.
	f.cached = map[string]core.CommitChecks{sha: commit([]core.CommitCheck{job(1, 1, "build", core.ConclusionFailure)})}
	if l, again := l.Want(sha, false); again != nil || text(l) != before {
		t.Fatal("a loaded commit changed without a reopen")
	}
	l, cmd = l.Update(ui.ReopenedMsg{})
	if cmd != nil {
		t.Error("a reopen asked for a read")
	}
	if after := text(l); after == before {
		t.Errorf("rows after the reopen = %q, want the kept checks", after)
	}
	if n := f.count(); n != 1 {
		t.Errorf("reads = %d, want 1", n)
	}
}

func TestLoaderOnlineReadsAgainWhatWasNotReached(t *testing.T) {
	t.Parallel()
	f := &fakeReader{errs: map[string]error{
		sha:  fmt.Errorf("read: %w", core.ErrOffline),
		sha2: core.ErrNotFound,
	}}
	l := NewLoader(f, repo)
	for _, s := range []string{sha, sha2} {
		var cmd tea.Cmd
		l, cmd = l.Want(s, false)
		l, _ = l.Update(run(t, cmd))
	}
	if n := f.count(); n != 2 {
		t.Fatalf("reads = %d, want 2", n)
	}

	delete(f.errs, sha)
	delete(f.errs, sha2)
	l, cmd := l.Update(ui.OnlineMsg{})
	msg := run(t, cmd)
	if got, ok := msg.(loadedMsg); !ok || got.sha != sha {
		t.Fatalf("message = %#v, want the reply for the commit that was not reached", msg)
	}
	if n := f.count(); n != 3 {
		t.Errorf("reads = %d, want 3", n)
	}
	if rows, _ := l.Rows(sha, opts(), styles(config.IconsASCII)); len(rows) != 1 || rows[0].ID != "c1/loading" {
		t.Errorf("rows while read again = %v, want the note that loads", first(rows))
	}
	l, _ = l.Update(msg)
	if rows, _ := l.Rows(sha, opts(), styles(config.IconsASCII)); len(rows) == 0 || rows[0].Skip {
		t.Errorf("rows once read again = %v, want the tree", first(rows))
	}
	// The refusal stays: a second OnlineMsg reads nothing.
	if _, cmd := l.Update(ui.OnlineMsg{}); cmd != nil {
		t.Error("a refusal was read again")
	}
}

func TestLoaderIgnoresOtherReplies(t *testing.T) {
	t.Parallel()
	f := &fakeReader{}
	l, cmd := NewLoader(f, repo).Want(sha, false)
	msg := run(t, cmd).(loadedMsg)

	other := msg
	other.id++
	if got, _ := l.Update(other); !got.entries[sha].loading {
		t.Error("a reply of another loader was taken")
	}

	stale := msg
	stale.sha = sha2
	if got, _ := l.Update(stale); len(got.entries) != 1 {
		t.Error("a reply for a commit that was never asked for was taken")
	}

	l, _ = l.Update(msg)
	late := msg
	late.checks = core.CommitChecks{}
	late.err = core.ErrOffline
	if got, _ := l.Update(late); got.entries[sha].err != nil {
		t.Error("a second reply replaced the one that ended the read")
	}
}

func TestLoaderUpdateKeepsCopiesApart(t *testing.T) {
	t.Parallel()
	l := NewLoader(&fakeReader{}, repo)
	next, _ := l.Want(sha, false)
	if len(l.entries) != 0 || len(next.entries) != 1 {
		t.Errorf("entries = %d before and %d after, want 0 and 1", len(l.entries), len(next.entries))
	}
}

func TestLoaderStopCancelsReads(t *testing.T) {
	t.Parallel()
	f := &fakeReader{}
	l, cmd := NewLoader(f, repo).Want(sha, false)
	run(t, cmd)
	if err := f.ctx.Err(); err != nil {
		t.Fatalf("context ended before Stop: %v", err)
	}
	l.Stop()
	if err := f.ctx.Err(); err == nil {
		t.Error("Stop left the read running")
	}
}

func TestLoaderGolden(t *testing.T) {
	t.Parallel()
	f := &fakeReader{errs: map[string]error{
		sha:  core.ErrOffline,
		sha2: core.ErrNotFound,
	}}
	st := styles(config.IconsASCII)
	const fresh = "0000000000000000000000000000000000000000"
	loading, _ := NewLoader(f, repo).Want(fresh, false)
	l := NewLoader(f, repo)
	for _, s := range []string{sha, sha2} {
		var cmd tea.Cmd
		l, cmd = l.Want(s, false)
		l, _ = l.Update(run(t, cmd))
	}
	for _, width := range []int{80, 40} {
		o := Options{Now: now, Prefix: "c1/", Depth: 1, Width: width}
		var out strings.Builder
		for _, c := range []struct {
			title string
			l     Loader
			sha   string
		}{{"loading", loading, fresh}, {"offline", l, sha}, {"refused", l, sha2}} {
			rows, _ := c.l.Rows(c.sha, o, st)
			fmt.Fprintf(&out, "%s at %d:\n%s\n", c.title, width, text(t, rows, width, 6))
		}
		t.Run(strconv.Itoa(width), func(t *testing.T) {
			golden.RequireEqual(t, out.String())
		})
	}
}
