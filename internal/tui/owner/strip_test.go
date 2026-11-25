package owner

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// The strip's forms run from the longest to the shortest, and the last one
// is not wider than the first.
func TestStripForms(t *testing.T) {
	tests := []struct {
		name  string
		keys  []string
		focus paneID
	}{
		{"first pane", []string{"z", "1"}, pinnedPane},
		{"repositories", []string{"z", "2"}, listPane},
		{"readme", []string{"z", "3"}, readmePane},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSection(t, newFake(), "octocat", 120, 40)
			press(t, s, tt.keys...)
			if s.page.focus != tt.focus {
				t.Fatalf("focus is pane %d, want %d", s.page.focus, tt.focus)
			}
			forms := s.stripForms()
			if len(forms) == 0 {
				t.Fatal("no forms")
			}
			for i := 1; i < len(forms); i++ {
				if a, b := ansi.StringWidth(forms[i-1]), ansi.StringWidth(forms[i]); a < b {
					t.Errorf("form %d is %d wide and the longer-ranked form %d is %d", i, b, i-1, a)
				}
			}
			long := ansi.Strip(forms[0])
			if forms[0] == long {
				t.Errorf("longest form %q marks no pane as the focused one", long)
			}
			for _, want := range []string{"1 ", "2 ", "3 "} {
				if !strings.Contains(long, want) {
					t.Errorf("longest form %q lacks the number %q", long, want)
				}
			}
		})
	}
}

// The strip is in the bottom edge of the frame of the one pane that shows,
// in a form that fits, and the page keeps its height.
func TestStripInBottomEdge(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		focus         string
		wantLong      bool
	}{
		{"wide", 120, 38, "2", true},
		{"narrow", 80, 22, "2", true},
		{"very narrow", 40, 22, "3", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSection(t, newFake(), "octocat", tt.width, tt.height)
			press(t, s, "z", tt.focus)
			if !s.onePane() {
				t.Fatal("the page is not zoomed")
			}
			lines := strings.Split(ansi.Strip(s.View()), "\n")
			if len(lines) != tt.height {
				t.Fatalf("view is %d rows, want %d", len(lines), tt.height)
			}
			bottom := lines[len(lines)-1]
			if w := ansi.StringWidth(bottom); w != tt.width {
				t.Errorf("bottom row is %d wide, want %d: %q", w, tt.width, bottom)
			}
			if !strings.HasPrefix(bottom, "╰") && !strings.HasPrefix(bottom, "└") {
				t.Errorf("bottom row %q is not the frame's edge", bottom)
			}
			if !strings.Contains(bottom, "["+tt.focus+"]") && !strings.Contains(bottom, tt.focus+" ") {
				t.Errorf("bottom row %q lacks the focused pane's number", bottom)
			}
			long := ansi.Strip(s.stripForms()[0])
			if got := strings.Contains(bottom, long); got != tt.wantLong {
				t.Errorf("bottom row %q shows the longest form %q: %v, want %v", bottom, long, got, tt.wantLong)
			}
		})
	}
}

// The label of a pane has its number while the panes show together, and
// shrinks to the room its edge has.
func TestLabelForms(t *testing.T) {
	tests := []struct {
		name  string
		keys  []string
		pane  paneID
		width int
		num   bool // the label is numbered
		fit   bool // every form but the first fits the room
	}{
		{"wide numbered", nil, listPane, 120, true, true},
		{"narrow box", nil, listPane, 24, true, true},
		{"zoomed has no number", []string{"z"}, listPane, 120, false, true},
		{"pinned", nil, pinnedPane, 60, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSection(t, newFake(), "octocat", 120, 40)
			press(t, s, tt.keys...)
			forms := s.labelForms(tt.pane, tt.width)
			if len(forms) == 0 {
				t.Fatal("no forms")
			}
			if got := strings.HasPrefix(ansi.Strip(forms[0]), "["); got != tt.num {
				t.Errorf("form %q numbered: %v, want %v", forms[0], got, tt.num)
			}
			if tt.fit && len(forms) > 1 {
				if last := forms[len(forms)-1]; ansi.StringWidth(last) > tt.width-5 {
					t.Errorf("last form %q is wider than the room %d", last, tt.width-5)
				}
			}
			if !slices.IsSorted(widths(forms)) && len(forms) > 1 && widths(forms)[0] < widths(forms)[len(forms)-1] {
				t.Errorf("forms %q are not longest first", forms)
			}
		})
	}
}

func widths(forms []string) []int {
	w := make([]int, len(forms))
	for i, f := range forms {
		w[i] = ansi.StringWidth(f)
	}
	return w
}
