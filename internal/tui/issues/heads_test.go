package issues

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// screenLines is the section's view without styles, one string per line.
func screenLines(h *host) []string {
	return strings.Split(ansi.Strip(h.View()), "\n")
}

func TestHeadsNameTheColumns(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		width       int
		has, hasNot []string
	}{
		{width: 140, has: []string{"#", "Title", "Author", "Updated"}},
		{width: 50, has: []string{"#", "Title"}, hasNot: []string{"Labels", "Author"}},
	} {
		h := started(t, newFakeService(sampleIssues(12)), tc.width, 10)
		lines := screenLines(h)
		if len(lines) != 10 {
			t.Fatalf("width %d: view has %d lines, want 10", tc.width, len(lines))
		}
		heads := lines[1]
		for _, w := range tc.has {
			if !strings.Contains(heads, w) {
				t.Errorf("width %d: header %q lacks %q", tc.width, heads, w)
			}
		}
		for _, w := range tc.hasNot {
			if strings.Contains(heads, w) {
				t.Errorf("width %d: header %q has %q", tc.width, heads, w)
			}
		}
	}
}

func TestHeadsAgeLinesUpWithRows(t *testing.T) {
	t.Parallel()
	h := started(t, newFakeService(sampleIssues(12)), 140, 10)
	lines := screenLines(h)
	at := strings.Index(lines[1], "Updated")
	if at < 0 {
		t.Fatalf("no Updated in %q", lines[1])
	}
	// The comment icon is wider in bytes than in cells.
	col := ansi.StringWidth(lines[1][:at])
	row := lines[2]
	if ansi.Cut(row, col, col+1) == " " || ansi.Cut(row, col-1, col) != " " {
		t.Errorf("age does not start at cell %d:\n%s\n%s", col, lines[1], row)
	}
}

func TestHeadsKeepTheLastRowVisible(t *testing.T) {
	t.Parallel()
	h := started(t, newFakeService(sampleIssues(12)), 100, 8)
	press(t, h, "G")
	lines := screenLines(h)
	if len(lines) != 8 {
		t.Fatalf("view has %d lines, want 8", len(lines))
	}
	if strings.TrimSpace(lines[len(lines)-1]) == "" {
		t.Errorf("last line is blank after G:\n%s", strings.Join(lines, "\n"))
	}
}
