package actions

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// groupCount matches the count a group header ends with, as in "CI (3)".
var groupCount = regexp.MustCompile(`\(\d+\)$`)

// No group header is left alone at the bottom of the runs, with its rows
// out of view, wherever the cursor goes.
func TestNoOrphanHeaderAtTheBottom(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{narrowW, narrowH}, {120, 40}} {
		m, h := newGroupedModal(t, newFake(), size[0], size[1])
		h.keys("enter")
		for i := range 12 {
			lines := strings.Split(ansi.Strip(strings.Join(wideRunsBox(m), "\n")), "\n")
			// The last line is the bottom border of the box.
			for j := len(lines) - 2; j >= 0; j-- {
				row := strings.TrimSpace(strings.Trim(strings.TrimSpace(lines[j]), "│"))
				if row == "" {
					continue
				}
				if groupCount.MatchString(row) {
					t.Fatalf("%dx%d after %d moves: the header %q is the last row:\n%s", size[0], size[1], i, row, screen(m))
				}
				break
			}
			h.keys("j")
		}
	}
}

// The cursor stays on its job while the jobs of the run load again.
func TestCursorKeptThroughAJobLoad(t *testing.T) {
	t.Parallel()
	f := newFake()
	m, h := newModal(t, f, wideW, wideH)
	name := testJobs()[failedRun][1].Name
	h.toJob(t, name)
	f.setJobs(failedRun, testJobs()[failedRun])
	h.keys("r")
	if j, ok := m.cursorJob(); !ok || j.Name != name {
		t.Fatalf("the cursor left the job %q: %q, %v", name, j.Name, ok)
	}
}

// Enter, l, h and * on each kind of row of the tree.
func TestKeysPerRowKind(t *testing.T) {
	t.Parallel()
	t.Run("folded run", func(t *testing.T) {
		t.Parallel()
		m, h := newModal(t, newFake(), wideW, wideH)
		if m.runs.Unfolded() {
			t.Fatal("a run starts unfolded")
		}
		h.keys("l")
		if !m.runs.Unfolded() {
			t.Fatal("l did not unfold the run")
		}
		h.keys("h")
		if m.runs.Unfolded() || m.cursorOnJobs() {
			t.Fatal("h did not fold the run")
		}
		h.keys("enter")
		if !m.runs.Unfolded() {
			t.Fatal("enter did not unfold the run")
		}
	})
	t.Run("unfolded run", func(t *testing.T) {
		t.Parallel()
		m, h := newModal(t, newFake(), wideW, wideH)
		h.keys("l", "l")
		if !m.cursorOnJobs() {
			t.Fatal("l on an unfolded run did not step onto its first job")
		}
		h.keys("h")
		if !m.runs.Unfolded() {
			t.Fatal("h on a job folded its run instead of stepping up")
		}
		h.keys("enter")
		if m.runs.Unfolded() {
			t.Fatal("enter did not fold the run")
		}
	})
	t.Run("job", func(t *testing.T) {
		t.Parallel()
		m, h := newModal(t, newFake(), wideW, wideH)
		h.toJobs(t)
		h.keys("l")
		if !m.cursorOnJobs() || h.log != nil {
			t.Fatal("l on a job moved the cursor or opened the log")
		}
		h.keys("h")
		if m.cursorOnJobs() || !m.runs.Unfolded() {
			t.Fatal("h on a job did not step up to its run")
		}
		h.toJobs(t)
		h.keys("enter")
		if h.log == nil {
			t.Fatal("enter on a job did not open its log")
		}
	})
	t.Run("backspace on a job", func(t *testing.T) {
		t.Parallel()
		m, h := newModal(t, newFake(), wideW, wideH)
		h.toJobs(t)
		h.keys("backspace")
		if m.cursorOnJobs() || !m.runs.Unfolded() {
			t.Fatal("backspace on a job did not step up to its run")
		}
	})
	t.Run("header", func(t *testing.T) {
		t.Parallel()
		m, h := newGroupedModal(t, newFake(), wideW, wideH)
		h.keys("up")
		if _, ok := m.runs.Selected(); ok {
			t.Fatal("up did not reach the header")
		}
		h.keys("h")
		if strings.Contains(screen(m), "#4812") {
			t.Fatal("h did not fold the group")
		}
		h.keys("l")
		if !strings.Contains(screen(m), "#4812") {
			t.Fatal("l did not open the group")
		}
	})
	// The jobs of the duck run: row 7 of the jobs is a folded group.
	toGroup := func(t *testing.T) (*Modal, *host) {
		t.Helper()
		m, h := newGroupedModal(t, duckFake(), wideW, wideH)
		h.toJobs(t)
		h.keys("j", "j", "j", "j", "j", "j", "j")
		if !m.onGroupRow() || m.groupOpen() {
			t.Fatal("the cursor is not on a folded group")
		}
		return m, h
	}
	t.Run("group", func(t *testing.T) {
		t.Parallel()
		m, h := toGroup(t)
		h.keys("h")
		if m.cursorOnJobs() || !m.runs.Unfolded() {
			t.Fatal("h on a folded group did not step up to its run")
		}
	})
	t.Run("open group", func(t *testing.T) {
		t.Parallel()
		m, h := toGroup(t)
		h.keys("l")
		if !m.groupOpen() {
			t.Fatalf("l did not open the group:\n%s", strings.Join(jobsPaneRows(m), "\n"))
		}
		h.keys("l")
		if !m.onGroupRow() || !m.groupOpen() {
			t.Fatal("l on an open group folded it or left it")
		}
		h.keys("h")
		if m.groupOpen() {
			t.Fatal("h did not fold the open group")
		}
	})
	t.Run("backspace on a group", func(t *testing.T) {
		t.Parallel()
		m, h := toGroup(t)
		h.keys("backspace")
		if m.cursorOnJobs() || !m.runs.Unfolded() {
			t.Fatal("backspace on a group did not step up to its run")
		}
		m, h = toGroup(t)
		h.keys("l", "backspace")
		if m.cursorOnJobs() || !m.runs.Unfolded() {
			t.Fatal("backspace on an open group did not step up to its run")
		}
	})
	t.Run("star", func(t *testing.T) {
		t.Parallel()
		m, h := newGroupedModal(t, newFake(), wideW, wideH)
		// The key opens every group if one is closed, else folds them all, so
		// a second press may be needed to reach the folded state.
		var folded bool
		for range 2 {
			h.keys("*")
			if !strings.Contains(screen(m), "#4812") {
				folded = true
				break
			}
		}
		if !folded {
			t.Fatal("* did not fold every group")
		}
		h.keys("*")
		if !strings.Contains(screen(m), "#4812") {
			t.Fatal("* did not open every group again")
		}
	})
}
