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
	if got := bottom(); !strings.Contains(got, "[2] Results") {
		t.Errorf("results focused, bottom edge %q lacks [2] Results", got)
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
	press(t, s, "tab")
	if s.preview || s.area != inputArea {
		t.Errorf("tab from the preview: preview %v, area %d, want the query", s.preview, s.area)
	}
}
