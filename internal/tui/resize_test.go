package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

var (
	ctrlLeft  = tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl}
	ctrlRight = tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModCtrl}
)

// column is the files column's width, frame included: the section is sized
// inside the frame, two cells narrower.
func column(f *splitFake) int { return f.width + 2 }

func TestDividerKeyNames(t *testing.T) {
	t.Parallel()
	if got := ctrlLeft.String(); got != "ctrl+left" {
		t.Errorf("ctrl+left presses as %q", got)
	}
	if got := ctrlRight.String(); got != "ctrl+right" {
		t.Errorf("ctrl+right presses as %q", got)
	}
}

func TestDividerGrowsAndShrinks(t *testing.T) {
	t.Parallel()
	for _, focus := range []string{"1", "2"} {
		m, files := newSplitApp(t, 120, 40)
		run(m, m.key(press(focus)))
		def := filesWidth(120)
		if column(files) != def {
			t.Fatalf("files width = %d, want the default %d", column(files), def)
		}
		run(m, m.key(ctrlRight))
		if column(files) != def+2 {
			t.Errorf("focus %s: after ctrl+right files width = %d, want %d", focus, column(files), def+2)
		}
		run(m, m.key(ctrlLeft))
		run(m, m.key(ctrlLeft))
		if column(files) != def-2 {
			t.Errorf("focus %s: after two ctrl+left files width = %d, want %d", focus, column(files), def-2)
		}
	}
}

func TestDividerStopsAtBounds(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	for range 60 {
		run(m, m.key(ctrlLeft))
	}
	if column(files) != panes.MinWidth {
		t.Fatalf("files width = %d, want the minimum %d", column(files), panes.MinWidth)
	}
	run(m, m.key(ctrlRight))
	if column(files) != panes.MinWidth+2 {
		t.Errorf("one press back from the minimum = %d, want %d", column(files), panes.MinWidth+2)
	}
	for range 100 {
		run(m, m.key(ctrlRight))
	}
	if want := 120 - panes.MinWidth; column(files) != want {
		t.Fatalf("files width = %d, want the maximum %d", column(files), want)
	}
	run(m, m.key(ctrlLeft))
	if want := 120 - panes.MinWidth - 2; column(files) != want {
		t.Errorf("one press back from the maximum = %d, want %d", column(files), want)
	}
}

func TestDividerReset(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	run(m, m.key(press("=")))
	if m.dividers.Moved() {
		t.Fatal("reset moved an unmoved divider")
	}
	run(m, m.key(ctrlRight))
	run(m, m.key(ctrlRight))
	run(m, m.key(press("=")))
	if column(files) != filesWidth(120) || m.dividers.Moved() {
		t.Errorf("after reset files width = %d, want %d", column(files), filesWidth(120))
	}
}

func TestDividerNarrowAndResize(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	run(m, m.key(ctrlRight))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if k := m.keys.state(m); k.DividerLeft.Enabled() || k.DividerRight.Enabled() {
		t.Error("the divider keys are enabled on a narrow terminal")
	}
	run(m, m.key(ctrlRight))
	if m.canDivide() {
		t.Error("a narrow terminal has a divider")
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if want := filesWidth(120) + 2; column(files) != want {
		t.Errorf("files width after growing back = %d, want %d", column(files), want)
	}
	m.Update(tea.WindowSizeMsg{Width: 160, Height: 40})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if want := filesWidth(100) + 2; column(files) != want {
		t.Errorf("files width at 100 = %d, want %d", column(files), want)
	}
}

func TestDividerKeysGoToWhatCapturesThem(t *testing.T) {
	t.Parallel()
	for _, open := range []string{":", "?"} {
		m, files := newSplitApp(t, 120, 40)
		run(m, m.key(press(open)))
		run(m, m.key(ctrlRight))
		if m.dividers.Moved() || column(files) != filesWidth(120) {
			t.Errorf("%q open: ctrl+right moved the divider", open)
		}
	}
}

func TestDividerFrame(t *testing.T) {
	t.Parallel()
	m, _ := newSplitApp(t, 120, 40)
	run(m, m.key(ctrlRight))
	run(m, m.key(ctrlRight))
	golden.RequireEqual(t, m.View().Content)
}

// preview is the width of the preview box the section was last given.
func preview(f *splitFake) int { return f.sizes[len(f.sizes)-1][0] }

func TestZoomedDividerMoves(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	run(m, m.key(press("z")))
	def := preview(files)
	run(m, m.key(ctrlRight))
	if got := preview(files); got != def-2 {
		t.Errorf("after ctrl+right the preview is %d wide, want %d", got, def-2)
	}
	run(m, m.key(ctrlLeft))
	run(m, m.key(ctrlLeft))
	if got := preview(files); got != def+2 {
		t.Errorf("after two ctrl+left the preview is %d wide, want %d", got, def+2)
	}
	if m.dividers.Files.Moved() {
		t.Error("the zoomed divider moved the files column")
	}
}

// The search shares the list divider, so a move made there reaches the
// zoomed list under it without any other key.
func TestSearchMovesZoomedDivider(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	run(m, m.key(press("z")))
	def := preview(files)
	run(m, m.key(press("S")))
	for range 3 {
		run(m, m.key(ctrlRight))
	}
	run(m, m.key(tea.KeyPressMsg{Code: tea.KeyEscape}))
	if !m.dividers.List.Moved() {
		t.Fatal("the search did not move the shared divider")
	}
	if got := preview(files); got != def-6 {
		t.Errorf("after three presses in the search the preview is %d wide, want %d", got, def-6)
	}
}

func TestZoomedDividerStopsAtBounds(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	run(m, m.key(press("z")))
	for range 60 {
		run(m, m.key(ctrlLeft))
	}
	if column(files) != panes.MinWidth {
		t.Fatalf("list width = %d, want the minimum %d", column(files), panes.MinWidth)
	}
	run(m, m.key(ctrlLeft))
	if column(files) != panes.MinWidth {
		t.Error("a press at the limit changed the layout")
	}
	for range 100 {
		run(m, m.key(ctrlRight))
	}
	if want := 120 - panes.MinWidth; column(files) != want {
		t.Fatalf("list width = %d, want the maximum %d", column(files), want)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if want := 100 - panes.MinWidth; column(files) != want {
		t.Errorf("list width after shrinking = %d, want %d", column(files), want)
	}
}

func TestDividerResetBoth(t *testing.T) {
	t.Parallel()
	m, files := newSplitApp(t, 120, 40)
	run(m, m.key(ctrlRight))
	run(m, m.key(press("z")))
	run(m, m.key(ctrlRight))
	if !m.dividers.Files.Moved() || !m.dividers.List.Moved() {
		t.Fatal("both dividers should be moved")
	}
	run(m, m.key(press("=")))
	if m.dividers.Moved() || column(files) != panes.DefaultListWidth {
		t.Errorf("after reset moved=%v list=%d", m.dividers.Moved(), column(files))
	}
}

func TestZoomedDividerDisabledWhenNarrow(t *testing.T) {
	t.Parallel()
	m, _ := newSplitApp(t, 80, 24)
	run(m, m.key(press("z")))
	if k := m.keys.state(m); k.DividerLeft.Enabled() || k.DividerRight.Enabled() {
		t.Error("the divider keys are enabled on a narrow zoom")
	}
}
