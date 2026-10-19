package pulls

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	actionssvc "github.com/eggzec/gh-tui/internal/service/actions"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/appcheck"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// noteChecks reads the annotations of a check run from a fixed list, and
// counts the reads.
type noteChecks struct {
	*fakeChecks
	items []core.Annotation
	err   error
	reads atomic.Int32
}

func (n *noteChecks) CachedAnnotations(actionssvc.AnnotationsQuery) (core.Page[core.Annotation], bool) {
	return core.Page[core.Annotation]{}, false
}

func (n *noteChecks) Annotations(context.Context, actionssvc.AnnotationsQuery) (core.Page[core.Annotation], error) {
	n.reads.Add(1)
	if n.err != nil {
		return core.Page[core.Annotation]{}, n.err
	}
	return core.Page[core.Annotation]{Items: n.items}, nil
}

// appRun is a check run of an App that failed with two annotations.
func appRun() core.CommitCheck {
	return core.CommitCheck{
		ID: 7, Name: "lint", Title: "2 problems", Summary: "The linter found problems.",
		App: &core.CheckApp{Name: "Linter"}, Status: core.RunCompleted, Conclusion: core.ConclusionFailure,
		Annotations: 2,
	}
}

func appNotes() []core.Annotation {
	return []core.Annotation{
		{Path: "main.go", StartLine: 3, EndLine: 3, Level: core.AnnotationFailure, Message: "unused variable"},
		{Path: "util.go", StartLine: 9, EndLine: 9, Level: core.AnnotationWarning, Message: "shadowed name"},
	}
}

// appPanes returns a Merge pane over a head with the check run of an App, with
// the preview shown, and the service that reads its annotations.
func appPanes(t *testing.T, svc *noteChecks) *panesModal {
	t.Helper()
	m, _ := appPanesOf(t, svc, appRun())
	return m
}

// appPanesOf is appPanes for the check run run, and also returns the checks
// that the pane keeps, so a test can change them.
func appPanesOf(t *testing.T, svc *noteChecks, run core.CommitCheck) (*panesModal, *checksFake) {
	t.Helper()
	m, _ := mergePanes(t, nil)
	c := &checksFake{kept: map[string]core.CommitChecks{"abc123": {SHA: "abc123", Total: 1, Runs: []core.CommitCheck{run}}}}
	m.checks = svc
	m.merge.watch = (&commitWatches{}).watch
	withChecks(m, c)
	m.zoom = true
	m.View()
	return m, c
}

// hoverApp moves the Merge cursor to the row of the check run, feeding the
// model the answers of the reads it started.
func hoverApp(t *testing.T, m *panesModal) {
	t.Helper()
	for range 12 {
		if m.hover[2].id == "check/7" {
			return
		}
		feedNotes(m, m.Update(keyMsg("j")))
	}
	t.Fatalf("hovered %q, want the row of the check run", m.hover[2].id)
}

// feedNotes hands the model the annotations that cmd read, if it read some.
func feedNotes(m *panesModal, cmd tea.Cmd) {
	if msg, ok := find[mergeNotesMsg](cmd); ok {
		m.Update(msg)
	}
}

func TestHoveringAnAppCheckReadsItsNotesOnce(t *testing.T) {
	t.Parallel()
	svc := &noteChecks{fakeChecks: &fakeChecks{}, items: appNotes()}
	m := appPanes(t, svc)
	hoverApp(t, m)
	if got := svc.reads.Load(); got != 1 {
		t.Fatalf("the annotations were read %d times, want once", got)
	}
	if body := ansi.Strip(m.View()); !strings.Contains(body, "unused variable") {
		t.Errorf("the preview of the check run lacks its annotation:\n%s", body)
	}
	for range 3 {
		feedNotes(m, m.Update(keyMsg("k")))
		feedNotes(m, m.Update(keyMsg("j")))
	}
	if got := svc.reads.Load(); got != 1 {
		t.Errorf("the annotations were read %d times after hovering again, want once", got)
	}
}

// TestAnnotationsThatComeLaterAreRead hovers a running check run that has no
// annotations yet. When the poll then brings some, they are read and shown.
func TestAnnotationsThatComeLaterAreRead(t *testing.T) {
	t.Parallel()
	svc := &noteChecks{fakeChecks: &fakeChecks{}, items: appNotes()}
	run := appRun()
	run.Annotations = 0
	m, c := appPanesOf(t, svc, run)
	hoverApp(t, m)
	if got := svc.reads.Load(); got != 0 {
		t.Fatalf("the annotations were read %d times, want none for a check run without any", got)
	}
	run.Annotations = 2
	c.mu.Lock()
	c.kept["abc123"] = core.CommitChecks{SHA: "abc123", Total: 1, Runs: []core.CommitCheck{run}}
	c.mu.Unlock()
	m.merge.rebuild(m, false)
	feedNotes(m, m.loadNotes())
	m.View()
	if got := svc.reads.Load(); got != 1 {
		t.Fatalf("the annotations were read %d times after the poll, want once", got)
	}
	if body := ansi.Strip(m.View()); !strings.Contains(body, "unused variable") {
		t.Errorf("the preview lacks the annotations that came later:\n%s", body)
	}
}

func TestFailedNotesAreNotReadAgain(t *testing.T) {
	t.Parallel()
	svc := &noteChecks{fakeChecks: &fakeChecks{}, err: errors.New("boom")}
	m := appPanes(t, svc)
	hoverApp(t, m)
	for range 3 {
		feedNotes(m, m.Update(keyMsg("k")))
		feedNotes(m, m.Update(keyMsg("j")))
	}
	if got := svc.reads.Load(); got != 1 {
		t.Errorf("the annotations were read %d times, want a failed read kept", got)
	}
	if body := ansi.Strip(m.View()); !strings.Contains(body, "couldn't be read") {
		t.Errorf("the preview doesn't say the annotations couldn't be read:\n%s", body)
	}
}

// TestAppCheckPreview holds the preview of a check run of an App in both
// themes, with its annotations, while they load, and when they failed.
func TestAppCheckPreview(t *testing.T) {
	t.Parallel()
	states := map[string]func(*noteChecks) func(*panesModal){
		"notes": func(*noteChecks) func(*panesModal) { return func(*panesModal) {} },
		"loading": func(*noteChecks) func(*panesModal) {
			// The read is started but its answer never arrives.
			return func(m *panesModal) { m.merge.notes[7].state, m.merge.notes[7].notes = appcheck.NotesLoading, nil }
		},
		"failed": func(*noteChecks) func(*panesModal) {
			return func(m *panesModal) { m.merge.notes[7].state, m.merge.notes[7].notes = appcheck.NotesFailed, nil }
		},
	}
	for state, settle := range states {
		for name, dark := range map[string]bool{"dark": true, "light": false} {
			t.Run(state+"/"+name, func(t *testing.T) {
				t.Parallel()
				svc := &noteChecks{fakeChecks: &fakeChecks{}, items: appNotes()}
				m := appPanes(t, svc)
				p, err := config.Default().Palette(dark)
				if err != nil {
					t.Fatal(err)
				}
				m.SetTheme(ui.NewTheme(p, dark))
				hoverApp(t, m)
				settle(svc)(m)
				m.merge.version++
				m.SetSize(100, 30)
				t.Log(ansi.Strip(m.View()))
				golden.RequireEqual(t, m.View())
			})
		}
	}
}
