package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

var (
	ctrlUp   = tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl}
	ctrlDown = tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl}
)

// newStackApp returns the repository screen with fake pull requests over
// issues, and the two fakes; a section is two lines shorter than its frame.
func newStackApp(t *testing.T, w, h int) (m *Model, pulls, issues *fakeSection) {
	t.Helper()
	files := &fakeSection{title: "Files"}
	pulls = &fakeSection{title: "Pull requests"}
	issues = &fakeSection{title: "Issues"}
	notes := &fakeSection{title: "Notifications"}
	m = New(t.Context(), config.Default(), Layout{Files: files, Pulls: pulls, Issues: issues, Notifications: notes}, WithRepo(testRepo))
	rankFakes(m)
	m.toast.SetDuration(0)
	m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m, pulls, issues
}

func TestStackKeyNames(t *testing.T) {
	t.Parallel()
	if ctrlUp.String() != "ctrl+up" || ctrlDown.String() != "ctrl+down" {
		t.Errorf("presses as %q and %q", ctrlUp.String(), ctrlDown.String())
	}
}

func TestStackGrowsAndShrinks(t *testing.T) {
	t.Parallel()
	m, pulls, issues := newStackApp(t, 120, 40)
	run(m, m.key(press("2")))
	total := pulls.height + issues.height
	top := pulls.height
	step := m.dividers.Step
	run(m, m.key(ctrlUp))
	if pulls.height != top+step || issues.height != total-pulls.height {
		t.Errorf("after ctrl+up pulls=%d issues=%d, want pulls %d", pulls.height, issues.height, top+step)
	}
	run(m, m.key(ctrlDown))
	run(m, m.key(ctrlDown))
	if pulls.height != top-step {
		t.Errorf("after two ctrl+down pulls=%d, want %d", pulls.height, top-step)
	}
	run(m, m.key(press("=")))
	if pulls.height != top || m.dividers.Moved() {
		t.Errorf("after reset pulls=%d, want %d", pulls.height, top)
	}
}

func TestStackLastPaneGrowsFromAbove(t *testing.T) {
	t.Parallel()
	m, pulls, issues := newStackApp(t, 120, 40)
	run(m, m.key(press("3")))
	bottom := issues.height
	run(m, m.key(ctrlUp))
	if issues.height != bottom+m.dividers.Step || pulls.height+issues.height != m.contentHeight()-4 {
		t.Errorf("after ctrl+up on issues: pulls=%d issues=%d", pulls.height, issues.height)
	}
}

func TestStackStopsAtMinHeight(t *testing.T) {
	t.Parallel()
	m, pulls, issues := newStackApp(t, 120, 40)
	run(m, m.key(press("2")))
	for range 60 {
		run(m, m.key(ctrlDown))
	}
	if pulls.height+2 != panes.MinHeight {
		t.Fatalf("pulls frame = %d, want %d", pulls.height+2, panes.MinHeight)
	}
	run(m, m.key(ctrlDown))
	if pulls.height+2 != panes.MinHeight {
		t.Error("a press at the limit changed the layout")
	}
	run(m, m.key(ctrlUp))
	if pulls.height+2 != panes.MinHeight+m.dividers.Step {
		t.Errorf("one press back = %d", pulls.height+2)
	}
	for range 100 {
		run(m, m.key(ctrlUp))
	}
	if issues.height+2 != panes.MinHeight {
		t.Errorf("issues frame = %d, want %d", issues.height+2, panes.MinHeight)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if issues.height+2 < panes.MinHeight || pulls.height+2+issues.height+2 != m.contentHeight() {
		t.Errorf("after resize pulls=%d issues=%d", pulls.height, issues.height)
	}
}

func TestStackDisabledWhereNoDividerApplies(t *testing.T) {
	t.Parallel()
	m, pulls, _ := newStackApp(t, 80, 24)
	before := pulls.height
	run(m, m.key(ctrlUp))
	if m.dividers.Moved() || pulls.height != before {
		t.Error("ctrl+up moved a narrow terminal")
	}
	if k := m.keys.state(m); k.DividerUp.Enabled() || k.DividerDown.Enabled() {
		t.Error("stack keys enabled on a narrow terminal")
	}
	m, _, _ = newStackApp(t, 120, 40)
	run(m, m.key(press("2")))
	if k := m.keys.state(m); !k.DividerUp.Enabled() {
		t.Error("stack keys disabled on a wide terminal")
	}
	run(m, m.key(press("z")))
	run(m, m.key(ctrlUp))
	if m.dividers.Moved() || m.keys.state(m).DividerUp.Enabled() {
		t.Error("the stack keys work while zoomed")
	}
	run(m, m.key(press("z")))
	run(m, m.key(press("1")))
	if m.canStack() {
		t.Error("the files pane has a stack")
	}
}
