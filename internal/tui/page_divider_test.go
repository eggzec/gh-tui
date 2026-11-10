package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// pageFake is a dashboard that records the split the root asks for.
type pageFake struct {
	fakeSection
	split int
}

func (p *pageFake) SetSplit(w int) { p.split = w }
func (p *pageFake) OnePane() bool  { return false }

func newPageApp(t *testing.T, w, h int) (*Model, *pageFake) {
	t.Helper()
	dash := &pageFake{}
	dash.title = ui.DashboardTitle
	layout := Layout{
		Files: &fakeSection{title: "Files"}, Pulls: &fakeSection{title: "Pull requests"},
		Issues: &fakeSection{title: "Issues"}, Notifications: &fakeSection{title: "Notifications"},
		Dashboard: dash,
	}
	m := New(t.Context(), config.Default(), layout)
	rankFakes(m)
	m.toast.SetDuration(0)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	run(m, m.Init())
	return m, dash
}

func TestPageDividerMoves(t *testing.T) {
	t.Parallel()
	m, dash := newPageApp(t, 120, 40)
	if want := 120 * 11 / 20; dash.split != want {
		t.Fatalf("unmoved split = %d, want %d", dash.split, want)
	}
	run(m, m.key(ctrlRight))
	if want := 120*11/20 + m.dividers.Step; dash.split != want {
		t.Errorf("split after ctrl+right = %d, want %d", dash.split, want)
	}
	run(m, m.key(ctrlLeft))
	run(m, m.key(ctrlLeft))
	if want := 120*11/20 - m.dividers.Step; dash.split != want {
		t.Errorf("split after ctrl+left = %d, want %d", dash.split, want)
	}
	run(m, m.key(press("=")))
	if want := 120 * 11 / 20; dash.split != want || m.dividers.Moved() {
		t.Errorf("split after reset = %d, want %d", dash.split, want)
	}
}

func TestPageDividerStopsAtBoundsAndResizes(t *testing.T) {
	t.Parallel()
	m, dash := newPageApp(t, 120, 40)
	for range 100 {
		run(m, m.key(ctrlRight))
	}
	if want := 120 - panes.MinWidth; dash.split != want {
		t.Errorf("split at the right limit = %d, want %d", dash.split, want)
	}
	for range 100 {
		run(m, m.key(ctrlLeft))
	}
	if dash.split != panes.MinWidth {
		t.Errorf("split at the left limit = %d, want %d", dash.split, panes.MinWidth)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if dash.split != panes.MinWidth {
		t.Errorf("split after a resize = %d, want %d", dash.split, panes.MinWidth)
	}
}

func TestPageDividerNarrow(t *testing.T) {
	t.Parallel()
	m, _ := newPageApp(t, 80, 24)
	run(m, m.key(ctrlRight))
	if m.canDivide() || m.dividers.Moved() {
		t.Error("a narrow terminal has a page divider")
	}
	if k := m.keys.state(m); k.DividerLeft.Enabled() || k.DividerRight.Enabled() {
		t.Error("the divider keys are enabled on a narrow terminal")
	}
}
