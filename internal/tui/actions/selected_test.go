package actions

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The selected run is bold on both its lines, its title takes the accent in
// a focused pane only, and a run that isn't selected is not bold.
func TestSelectedRun(t *testing.T) {
	t.Parallel()
	m, _ := newModal(t, newFake(), wideW, wideH)
	r := testRuns()[0]
	const bold = "\x1b[1"

	plain := m.renderRun(r, false, wideW)
	m.focus = runsPane
	focused := m.renderRun(r, true, wideW)
	m.focus = jobsPane
	blurred := m.renderRun(r, true, wideW)

	if strings.Contains(plain, bold) {
		t.Errorf("a run that isn't selected has bold cells: %q", plain)
	}
	for name, got := range map[string]string{"focused": focused, "blurred": blurred} {
		lines := strings.Split(got, "\n")
		if len(lines) != 2 {
			t.Fatalf("%s run has %d lines, want 2", name, len(lines))
		}
		for i, l := range lines {
			if !strings.Contains(l, bold) {
				t.Errorf("%s run: line %d has no bold cell: %q", name, i, l)
			}
		}
		if got, want := ansi.Strip(got), ansi.Strip(plain); got != want {
			t.Errorf("%s run reads %q, want the text of the plain row %q", name, got, want)
		}
	}
	if focused == blurred {
		t.Errorf("the run looks the same in a focused and a blurred pane")
	}
}
