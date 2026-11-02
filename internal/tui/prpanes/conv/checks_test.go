package conv

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
)

// fakeChecks reads the checks of a commit, one passing job, and counts the
// reads.
type fakeChecks struct {
	reads []string
	// kept answers the cached reads.
	kept map[string]core.CommitChecks
}

func (f *fakeChecks) CachedCommitChecks(q checktree.Query) (core.CommitChecks, bool) {
	c, ok := f.kept[q.SHA]
	return c, ok
}

func (f *fakeChecks) CommitChecks(_ context.Context, q checktree.Query) (core.CommitChecks, error) {
	f.reads = append(f.reads, q.SHA)
	return jobChecks(q.SHA), nil
}

func jobChecks(sha string) core.CommitChecks {
	return core.CommitChecks{SHA: sha, Total: 1, Runs: []core.CommitCheck{{
		ID: 5, JobID: 5, RunID: 9, Name: "build", Workflow: "CI",
		Status: core.RunCompleted, Conclusion: core.ConclusionSuccess,
		StartedAt: now.Add(-time.Minute), CompletedAt: now, RunNumber: 3,
	}}}
}

// checksDriver is a conversation of one push of four commits, with checks
// read by f.
func checksDriver(t *testing.T, f *fakeChecks, states ...core.CheckState) *driver {
	t.Helper()
	read := func() core.PullCommits { return pullCommits(states...) }
	if len(states) == 0 {
		read = nil
	}
	s := pushSource(read, nil)
	if f != nil {
		s.Checks, s.Repo = f, core.RepoRef{Owner: "o", Name: "r"}
	}
	d := &driver{t: t, m: New(s, WithSize(100, 40), WithOptions(opts0()), WithTreeKeys(pushKeys()))}
	d.q = []tea.Cmd{d.m.Init()}
	d.all()
	d.m.Focus()
	return d
}

var (
	keyRight = tea.KeyPressMsg{Code: tea.KeyRight}
	keyLeft  = tea.KeyPressMsg{Code: tea.KeyLeft}
	keyDown  = tea.KeyPressMsg{Code: tea.KeyDown}
)

// openFirstCommit opens the push and its first commit.
func openFirstCommit(d *driver) {
	d.send(keyRight)
	d.send(keyDown)
	d.send(keyRight)
}

func TestCommitChecksLazy(t *testing.T) {
	t.Parallel()
	f := &fakeChecks{}
	d := checksDriver(t, f, core.CheckPassing, core.CheckFailing, core.CheckPending, core.CheckNone)
	if len(f.reads) != 0 {
		t.Fatalf("reads before anything opened = %v", f.reads)
	}
	d.send(keyRight)
	d.send(keyDown)
	d.all()
	if len(f.reads) != 0 {
		t.Fatalf("reads with the push open = %v, want none until a commit opens", f.reads)
	}

	d.send(keyRight)
	if got := ansi.Strip(d.m.View()); !strings.Contains(got, "Loading checks") {
		t.Errorf("an opened commit shows no loading note:\n%s", got)
	}
	d.all()
	if !slices.Equal(f.reads, []string{sha(1)}) {
		t.Fatalf("reads = %v, want the one commit that opened, by its full SHA", f.reads)
	}
	if got := ansi.Strip(d.m.View()); !strings.Contains(got, "build") || strings.Contains(got, "Loading checks") {
		t.Errorf("an opened commit shows no checks once read:\n%s", got)
	}

	d.send(keyLeft)
	d.send(keyRight)
	d.all()
	if len(f.reads) != 1 {
		t.Errorf("reads after closing and opening again = %v, want still one", f.reads)
	}
	if got := ansi.Strip(d.m.View()); !strings.Contains(got, "build") {
		t.Errorf("the checks are gone after opening again:\n%s", got)
	}
}

func TestCommitChecksKept(t *testing.T) {
	t.Parallel()
	f := &fakeChecks{kept: map[string]core.CommitChecks{sha(1): jobChecks(sha(1))}}
	d := checksDriver(t, f, core.CheckPassing, core.CheckPassing, core.CheckPassing, core.CheckPassing)
	openFirstCommit(d)
	if got := ansi.Strip(d.m.View()); !strings.Contains(got, "build") {
		t.Errorf("kept checks are not shown at once:\n%s", got)
	}
	d.all()
	if len(f.reads) != 0 {
		t.Errorf("reads = %v, want none for kept checks", f.reads)
	}
}

func TestCommitNoChecks(t *testing.T) {
	t.Parallel()
	row := func(d *driver, id string) foldtreeRow {
		t.Helper()
		for _, r := range d.m.views[""].rows {
			if r.ID == id {
				return foldtreeRow{r.Branch, r.Branch, r.Closed}
			}
		}
		t.Fatalf("no row %s", id)
		return foldtreeRow{}
	}
	d := checksDriver(t, &fakeChecks{}, core.CheckPassing, core.CheckFailing, core.CheckPending, core.CheckNone)
	for i := 1; i <= 3; i++ {
		if got := row(d, "k:"+sha(i)); got != (foldtreeRow{true, true, true}) {
			t.Errorf("commit %d row = %+v, want a closed fold with an arrow", i, got)
		}
	}
	if got := row(d, "k:"+sha(4)); got.fold {
		t.Errorf("a commit with no checks folds: %+v", got)
	}

	if got := row(checksDriver(t, &fakeChecks{}), "k:"+sha(1)); !got.fold {
		t.Error("a commit whose roll-up is unknown doesn't fold")
	}
	if got := row(checksDriver(t, nil, core.CheckPassing), "k:"+sha(1)); got.fold {
		t.Error("a commit folds with no way to read its checks")
	}
}

// foldtreeRow is the fold state of a row.
type foldtreeRow struct{ fold, arrow, closed bool }

func TestEnterCheckTarget(t *testing.T) {
	t.Parallel()
	f := &fakeChecks{}
	d := checksDriver(t, f, core.CheckPassing, core.CheckPassing, core.CheckPassing, core.CheckPassing)
	openFirstCommit(d)
	d.all()
	v := d.m.views[""]
	var id string
	for k, msg := range v.targets {
		if _, ok := msg.(checktree.OpenJobMsg); ok {
			id = k
		}
	}
	if id == "" {
		t.Fatalf("no row of a job has a target: %v", v.targets)
	}
	if !strings.HasPrefix(id, "k:"+sha(1)+"/") {
		t.Errorf("row ID = %q, want the commit's ID as its prefix", id)
	}
	cmd := v.enter(id)
	if cmd == nil {
		t.Fatal("enter on a job sent nothing")
	}
	got, ok := cmd().(checktree.OpenJobMsg)
	if !ok || got.SHA != sha(1) || got.RunID != 9 || got.JobID != 5 {
		t.Errorf("enter on a job sent %+v, want its job of the commit %s", got, sha(1))
	}
	if msg, ok := v.enter("k:" + sha(1))().(EnterMsg); !ok || msg.Kind != KindCommit || msg.SHA != sha(1) {
		t.Errorf("enter on a commit sent %+v, want its EnterMsg", msg)
	}
}
