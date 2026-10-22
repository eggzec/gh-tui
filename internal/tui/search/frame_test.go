package search

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func TestFrameSizes(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{80, 24}, {100, 30}, {140, 40}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			t.Parallel()
			s := newSection(t, newFake(), size[0], size[1])
			typeText(t, s, "tea")
			press(t, s, "esc")
			golden.RequireEqual(t, s.View())
		})
	}
}

func TestFootShowsFocusedPaneKeys(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 140, 40)
	typeText(t, s, "tea")
	press(t, s, "esc")
	bottom := func() string {
		lines := strings.Split(s.View(), "\n")
		return lines[len(lines)-1]
	}
	if got := bottom(); !strings.Contains(got, "[tab] Preview") && !strings.Contains(got, "tab Preview") {
		t.Errorf("results focused, bottom edge %q lacks the tab to the preview", got)
	}
	press(t, s, "tab")
	if got := bottom(); !strings.Contains(got, "[tab] Preview") {
		t.Errorf("preview focused, bottom edge %q lacks [tab] Preview", got)
	}
}

func TestSingleFallbackWhenSmall(t *testing.T) {
	t.Parallel()
	for _, size := range [][2]int{{99, 40}, {140, 29}} {
		s := newSection(t, newFake(), size[0], size[1])
		press(t, s, "2")
		if s.split() || s.preview {
			t.Errorf("%dx%d: want a single pane", size[0], size[1])
		}
	}
}

func TestTabSwitchesPane(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 80, 24)
	press(t, s, "2")
	press(t, s, "tab")
	if !s.preview || !strings.Contains(s.View(), "Preview") {
		t.Errorf("tab from the results left preview %v, view:\n%s", s.preview, s.View())
	}
	for _, k := range []string{"tab", "shift+tab", "shift+tab", "tab", "tab", "shift+tab"} {
		press(t, s, k)
		if s.area != resultsArea || s.typing {
			t.Fatalf("%s: area %d, typing %v, want the results or the preview", k, s.area, s.typing)
		}
	}
	press(t, s, "tab")
	if s.preview {
		t.Errorf("tab from the preview left preview %v, want the results", s.preview)
	}
}

// A fresh page has a kind to show, so its results are titled by it.
func TestResultsTitledByKindBeforeSearch(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 120, 30)
	if got := s.resultsLabel(); got == "Start" || got != kindTitles[s.kind] {
		t.Errorf("label %q, want %q", got, kindTitles[s.kind])
	}
}
