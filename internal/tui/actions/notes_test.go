package actions

import (
	"fmt"
	"strings"
	"testing"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
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
	h.keys("tab")
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
	h.keys("tab")
	v := notesText(m)
	if strings.Contains(v, "No annotations on this run") || !strings.Contains(v, "to retry") {
		t.Errorf("failure not shown:\n%s", v)
	}
}

// Enter on an annotation opens the log of its job.
func TestNotesEnterOpensLog(t *testing.T) {
	t.Parallel()
	m, h := newModal(t, newFake(), wideW, wideH)
	h.keys("tab")
	want := m.noteEntries()[0].job.ID
	h.keys("enter")
	if h.log == nil {
		t.Fatal("enter did not open the log")
	}
	if want != ubuntuJob {
		t.Fatalf("first annotation is of job %d, want %d", want, ubuntuJob)
	}
	if got := h.log.job.ID; got != want {
		t.Errorf("opened the log of job %d, want %d", got, want)
	}
}

// A failed read shows its error beside the annotations of the other jobs.
func TestNotesFailedAmongRows(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.notesErrs = map[int64]error{macosJob: fmt.Errorf("list annotations: %w", core.ErrOffline)}
	m, h := newModal(t, f, wideW, wideH)
	h.keys("tab")
	v := notesText(m)
	if !strings.Contains(v, "tea_test.go:54") || !strings.Contains(v, "to retry") {
		t.Errorf("row or error missing:\n%s", v)
	}
}

// A job with more than a page of annotations shows its count with a plus.
func TestNotesMorePages(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.notesNext = map[int64]string{ubuntuJob: "2"}
	m, h := newModal(t, f, wideW, wideH)
	h.keys("tab")
	if v := ansi.Strip(m.View()); !strings.Contains(v, " 2+ annotations ") {
		t.Errorf("count lacks the plus:\n%s", v)
	}
}

// With the ASCII icons the notes box, in the frame and on its own, is ASCII
// alone.
func TestNotesASCII(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		width, height int
	}{{"80 columns", narrowW, narrowH}, {"120 columns", 120, 40}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, h := newModal(t, newFake(), tt.width, tt.height, WithIcons(ui.NewIcons(config.IconsASCII)))
			h.keys("tab")
			for _, v := range []string{ansi.Strip(m.View()), notesText(m)} {
				if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
					t.Errorf("notes view isn't ASCII:\n%s", v)
				}
			}
		})
	}
}

func TestViewNotes(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		width, height int
	}{{"80 columns", narrowW, narrowH}, {"120 columns", 120, 40}} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m, h := newModal(t, newFake(), tt.width, tt.height)
			h.keys("tab")
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}
