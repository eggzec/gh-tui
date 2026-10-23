package actions

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/core"
)

func notesText(m *Modal) string {
	w, h := m.notesInner()
	return ansi.Strip(strings.Join(m.notesLines(max(w, 100), max(h, 20)), "\n"))
}

// The annotations of the failed jobs list in the notes box, with where they
// point, and the job they belong to.
func TestNotesList(t *testing.T) {
	t.Parallel()
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	h.keys("tab", "tab")
	v := notesText(m)
	for _, want := range []string{"Annotations", "tea_test.go:54", "want a frame, got none", "Process completed"} {
		if !strings.Contains(v, want) {
			t.Errorf("notes box lacks %q:\n%s", want, v)
		}
	}
	if len(f.noteReads) == 0 {
		t.Error("no annotations were read")
	}
}

func TestNotesEmpty(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.notes = map[int64][]core.Annotation{}
	m, h := newModal(t, f, wideW, wideH)
	h.keys("tab")
	if v := notesText(m); !strings.Contains(v, "No annotations on this run") {
		t.Errorf("empty state missing:\n%s", v)
	}
}

func TestNotesLoading(t *testing.T) {
	t.Parallel()
	m, h := newModal(t, newFake(), wideW, wideH)
	h.keys("tab")
	for _, j := range m.noteJobs() {
		m.ann[j.ID] = &noteRes{loading: true}
	}
	if v := notesText(m); !strings.Contains(v, "Loading annotations") {
		t.Errorf("loading state missing:\n%s", v)
	}
}

func TestNotesFailed(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.notesErr = fmt.Errorf("list annotations: %w", core.ErrOffline)
	m, h := newModal(t, f, wideW, wideH)
	h.keys("tab", "tab")
	v := notesText(m)
	if strings.Contains(v, "No annotations on this run") || !strings.Contains(v, "to retry") {
		t.Errorf("failure not shown:\n%s", v)
	}
}

// Enter on an annotation opens the log of its job.
func TestNotesEnterOpensLog(t *testing.T) {
	t.Parallel()
	m, h := newModal(t, newFake(), wideW, wideH)
	h.keys("tab", "tab", "enter")
	if h.log == nil {
		t.Fatal("enter did not open the log")
	}
	_ = m
}

func TestViewNotes(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		width, height int
	}{{"80 columns", narrowW, narrowH}, {"120 columns", 120, 40}} {
		t.Run(tt.name, func(t *testing.T) {
			m, h := newModal(t, newFake(), tt.width, tt.height)
			h.keys("tab", "tab")
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}
