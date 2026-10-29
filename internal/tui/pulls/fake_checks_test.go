package pulls

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/appcheck"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/files"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

const checkSHA = "0123456789abcdef0123456789abcdef01234567"

func TestOpenJobMsgOpensTheActionsOnTheJob(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, 160)
	for _, focus := range []bool{false, true} {
		m.zoom.SetZoom(focus) // the pane that sent it doesn't matter
		cmd := m.Update(checktree.OpenJobMsg{SHA: checkSHA, RunID: 7, JobID: 9})
		if cmd == nil {
			t.Fatal("OpenJobMsg returned no command")
		}
		got, ok := cmd().(ui.OpenActionsMsg)
		if !ok {
			t.Fatalf("OpenJobMsg asked for %T, want OpenActionsMsg", cmd())
		}
		if got.Repo != m.repo || got.Filter != (core.RunFilter{HeadSHA: checkSHA}) ||
			got.At != (ui.ActionsAt{Run: 7, Job: 9}) || got.Over != ui.Modal(m) {
			t.Errorf("OpenActionsMsg = %+v, want the job of the commit, back to the modal", got)
		}
	}
}

// The real service of the actions is what the section reads the checks from,
// and it must offer the re-request that the page of an App's run sends.
var _ ChecksService = (*actionssvc.Service)(nil)

// appChecks serves a run of an App, with two annotations on a file of the
// diff, and the checks of every commit.
func appChecks() *fakeChecks {
	path := patched()[0].Path
	return &fakeChecks{
		app: &core.CommitCheck{
			ID: 21, Name: "coverage", Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
			Title: "Coverage fell", Summary: "Patch coverage is **61%**.", Annotations: 2,
			SuiteID: 7, App: &core.CheckApp{Name: "Codecov", Slug: "codecov"},
		},
		notes: []core.Annotation{
			{Path: path, StartLine: 2, EndLine: 2, Level: core.AnnotationFailure, Message: "Uncovered line."},
			{Path: path, StartLine: 3, EndLine: 3, Level: core.AnnotationWarning, Message: "Partly covered."},
		},
	}
}

// appCheckPanes opens the page of the run that appChecks serves, over the
// panes of a pull request whose files carry patches.
func appCheckPanes(t *testing.T) (*host, *panesModal, *fakeChecks, *appCheckOverlay) {
	t.Helper()
	fc := appChecks()
	h, m, _ := rangePanes(t, sampleCommits(nil), func(svc *fakeService) { svc.changed = patched() }, WithChecks(fc))
	drain(t, h, m.Update(checktree.OpenCheckMsg{SHA: checkSHA, CheckID: 21, SuiteID: 7}))
	o, ok := m.top().(*appCheckOverlay)
	if !ok {
		t.Fatalf("top overlay = %T, want the page of the check run", m.top())
	}
	modalScreen(t, h) // draws the page, which sizes it
	return h, m, fc, o
}

func TestOpenCheckMsgOpensTheAppRun(t *testing.T) {
	t.Parallel()
	_, m, _, o := appCheckPanes(t)
	if got, want := o.title(), "Codecov · coverage"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
	if a, ok := o.v.Selected(); !ok || a.Message != "Uncovered line." {
		t.Errorf("selected = %+v, %v; want the first annotation", a, ok)
	}
	if len(m.overlays) != 1 {
		t.Errorf("%d overlays open, want only the page", len(m.overlays))
	}
}

func TestOpenCheckMsgIgnoresARunItDoesNotKnow(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, 160)
	if cmd := m.Update(checktree.OpenCheckMsg{SHA: checkSHA, CheckID: 1, SuiteID: 2}); cmd != nil || m.top() != nil {
		t.Errorf("a run with no service opened %T and returned %v", m.top(), cmd)
	}
	h, m, _ := rangePanes(t, sampleCommits(nil), nil, WithChecks(appChecks()))
	drain(t, h, m.Update(checktree.OpenCheckMsg{SHA: checkSHA, CheckID: 99}))
	if m.top() != nil {
		t.Errorf("an unknown run opened %T", m.top())
	}
}

func TestAppCheckEnterOpensTheDiffAtTheAnnotation(t *testing.T) {
	t.Parallel()
	h, m, _, o := appCheckPanes(t)
	o.v.Move(1) // the second annotation
	want := patched()[0].Path
	drain(t, h, m.Update(keyMsg("enter")))
	d, ok := m.top().(*diffOverlay)
	if !ok || d.path != want {
		t.Fatalf("top = %T, want the diff of %s", m.top(), want)
	}
	if len(m.overlays) != 1 {
		t.Errorf("%d overlays open, want the diff alone", len(m.overlays))
	}
	wantPos := diff.Pos{Path: want, Side: diff.NewSide, Line: 3}
	if got, ok := d.d.Position(); !ok || got != wantPos {
		t.Errorf("cursor at %+v, %v; want the new-side line %+v", got, ok, wantPos)
	}
}

func TestAppCheckEnterWithoutALineLandsOnTheFile(t *testing.T) {
	t.Parallel()
	h, m, fc, o := appCheckPanes(t)
	path := patched()[0].Path
	fc.notes[0].StartLine = 0
	o.notes = fc.notes
	o.show()
	drain(t, h, m.Update(keyMsg("enter")))
	d, ok := m.top().(*diffOverlay)
	if !ok || d.path != path {
		t.Fatalf("top = %T, want the diff of %s", m.top(), path)
	}
	if f, ok := d.d.CurrentFile(); !ok || f.Path != path {
		t.Errorf("cursor in %+v, %v; want the file %s", f, ok, path)
	}
	if got, ok := d.d.Position(); ok {
		t.Errorf("cursor on the line %+v, want the file's header", got)
	}
}

func TestAppCheckEnterOnAFileOutsideThePullRequest(t *testing.T) {
	t.Parallel()
	h, m, fc, o := appCheckPanes(t)
	fc.notes[0].Path = "docs/not-in-the-pull-request.md"
	o.notes = fc.notes
	o.show()
	msgs := drain(t, h, m.Update(keyMsg("enter")))
	if m.top() != o {
		t.Fatalf("top = %T, want the page to stay", m.top())
	}
	want := ui.NotifyMsg{Level: toast.Info, Text: "the file docs/not-in-the-pull-request.md is not in this diff"}
	if !slices.Contains(msgs, tea.Msg(want)) {
		t.Errorf("messages %v lack the hint %v", msgs, want)
	}
}

func TestAppCheckEnterResetsARangeWithoutTheFile(t *testing.T) {
	t.Parallel()
	h, m, _, _ := appCheckPanes(t)
	f := &m.files
	f.rng = files.Range{First: 1, Last: 1}
	f.read.files = nil // the commit of that range changes nothing
	path := patched()[0].Path
	drain(t, h, m.Update(keyMsg("enter")))
	if d, ok := m.top().(*diffOverlay); !ok || d.path != path {
		t.Fatalf("top = %T, want the diff of %s", m.top(), path)
	}
	if want := files.All(len(f.commits.Commits)); f.rng != want {
		t.Errorf("range = %+v, want %+v", f.rng, want)
	}
}

func TestAppCheckRerequest(t *testing.T) {
	t.Parallel()
	t.Run("asks, then sends", func(t *testing.T) {
		t.Parallel()
		h, m, fc, _ := appCheckPanes(t)
		drain(t, h, m.Update(keyMsg("R")))
		c, ok := m.top().(*confirmOverlay)
		if !ok || c.c.Question != "Re-request check suite?" {
			t.Fatalf("top = %T, want the question", m.top())
		}
		if got := fc.suitesAsked(); len(got) != 0 {
			t.Fatalf("asked %v before the answer", got)
		}
		drain(t, h, m.Update(keyMsg("y")))
		if got := fc.suitesAsked(); !slices.Equal(got, []int64{7}) {
			t.Errorf("asked suites %v, want [7]", got)
		}
	})
	t.Run("a failure shows a toast", func(t *testing.T) {
		t.Parallel()
		h, m, fc, _ := appCheckPanes(t)
		fc.suiteErr = errors.New("suite gone")
		drain(t, h, m.Update(keyMsg("R")))
		msgs := drain(t, h, m.Update(keyMsg("y")))
		if !slices.ContainsFunc(msgs, func(msg tea.Msg) bool {
			f, ok := msg.(ui.FailMsg)
			return ok && f.Err != nil && strings.Contains(f.Err.Error(), "suite gone")
		}) {
			t.Errorf("messages %v lack the failure", msgs)
		}
	})
	t.Run("a run with no suite says so", func(t *testing.T) {
		t.Parallel()
		h, m, fc, o := appCheckPanes(t)
		o.check.SuiteID = 0
		msgs := drain(t, h, m.Update(keyMsg("R")))
		want := ui.NotifyMsg{Level: toast.Info, Text: "coverage has no check suite to re-request"}
		if !slices.Contains(msgs, tea.Msg(want)) {
			t.Errorf("messages %v lack the hint %v", msgs, want)
		}
		if _, ok := m.top().(*confirmOverlay); ok || len(fc.suitesAsked()) != 0 {
			t.Errorf("a run with no suite asked a question or sent a request")
		}
	})
	t.Run("the annotations arrive under the question", func(t *testing.T) {
		t.Parallel()
		h, m, _, o := appCheckPanes(t)
		o.state = appcheck.NotesLoading
		drain(t, h, m.Update(keyMsg("R")))
		if _, ok := m.top().(*confirmOverlay); !ok {
			t.Fatalf("top = %T, want the question", m.top())
		}
		drain(t, h, m.Update(appCheckNotesMsg{o: o, notes: []core.Annotation{{Path: "a.go", StartLine: 1, Message: "late"}}}))
		if o.state != appcheck.NotesLoaded || len(o.notes) != 1 {
			t.Errorf("state %v with %d notes, want the annotations loaded", o.state, len(o.notes))
		}
	})
	t.Run("declined sends nothing", func(t *testing.T) {
		t.Parallel()
		h, m, fc, _ := appCheckPanes(t)
		drain(t, h, m.Update(keyMsg("R")))
		drain(t, h, m.Update(keyMsg("n")))
		if got := fc.suitesAsked(); len(got) != 0 {
			t.Errorf("asked suites %v after no", got)
		}
	})
}
