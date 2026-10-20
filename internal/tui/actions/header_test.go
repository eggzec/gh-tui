package actions

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The runs are headed by their columns, and the dates start under their
// title, whatever the width.
func TestRunsHeader(t *testing.T) {
	t.Parallel()
	for _, w := range []int{120, 80, 24} {
		m, _ := newModal(t, newFake(), w, 30)
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		head, name := -1, -1
		for i, l := range lines {
			switch {
			case head < 0 && strings.Contains(l, "Run "):
				head = i
			case name < 0 && head >= 0 && strings.Contains(l, testRuns()[0].Name):
				name = i
			}
		}
		if head < 0 || name < 0 {
			t.Fatalf("width %d: header %d, first run %d in\n%s", w, head, name, strings.Join(lines, "\n"))
		}
		i := strings.Index(lines[head], "Created")
		if w == 24 {
			if i >= 0 {
				t.Errorf("width %d: the date column stays in %q", w, lines[head])
			}
			continue
		}
		if i < 0 {
			t.Fatalf("width %d: no Created in %q", w, lines[head])
		}
		col := ansi.StringWidth(lines[head][:i])
		row := lines[name]
		if got := ansi.Cut(row, col-1, col+1); !strings.HasPrefix(got, " ") || strings.HasSuffix(got, " ") {
			t.Errorf("width %d: the date is not left-aligned under %q: %q", w, lines[head], row)
		}
	}
}
