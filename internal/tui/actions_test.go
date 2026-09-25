package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// tabbedModal is a fake modal with tabs.
type tabbedModal struct {
	fakeModal
	names  []string
	active int
}

func (t *tabbedModal) Tabs() (names []string, active int) { return t.names, t.active }

// actionsOpener opens a fake modal and records the repositories it was
// opened on.
type actionsOpener struct {
	modal  *tabbedModal
	opened []core.RepoRef
	loads  int
}

func (a *actionsOpener) open(_ context.Context, repo core.RepoRef) (ui.Modal, tea.Cmd) {
	a.opened = append(a.opened, repo)
	a.modal = &tabbedModal{title: "Actions · " + repo.String(), names: []string{"All", "Failing", "Running", "Mine"}, active: 1}
	return a.modal, func() tea.Msg { a.loads++; return nil }
}

func TestActionsKeyOpensTheActions(t *testing.T) {
	a := &actionsOpener{}
	m, _ := newTestApp(t, WithActions(a.open))
	run(m, m.Init())
	if s := onScreen(m); !strings.Contains(s, "a actions") {
		t.Errorf("help lacks the actions key:\n%s", s)
	}
	run(m, m.key(press("a")))
	if m.topModal() != a.modal || a.loads != 1 || len(a.opened) != 1 || a.opened[0] != testRepo {
		t.Fatalf("a opened %v on %v and loaded %d times, want the actions of %s loaded once", m.topModal(), a.opened, a.loads, testRepo)
	}
	if a.modal.width == 0 || !a.modal.themed {
		t.Error("the modal wasn't sized and themed before it was drawn")
	}
}

func TestActionsKeyNeedsARepoAndItsScreen(t *testing.T) {
	a := &actionsOpener{}
	m, _ := newApp(t, core.RepoRef{}, WithActions(a.open))
	run(m, m.key(press("a")))
	if s := onScreen(m); strings.Contains(s, "a actions") {
		t.Errorf("help offers the actions without a repository:\n%s", s)
	}
	m, _ = newTestApp(t, WithActions(a.open))
	run(m, m.key(press("n")))
	run(m, m.key(press("a")))
	if len(a.opened) != 0 || m.topModal() != nil {
		t.Errorf("a opened the actions %d times off the repository screen", len(a.opened))
	}
	m, _ = newTestApp(t)
	run(m, m.key(press("a")))
	if m.topModal() != nil {
		t.Error("a opened a modal without actions")
	}
}

func TestFrameShowsTheTabs(t *testing.T) {
	a := &actionsOpener{}
	m, _ := newTestApp(t, WithActions(a.open))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	run(m, m.key(press("a")))
	top := topEdge(m)
	if !strings.Contains(top, "Actions · eggzec/gh-tui") || !strings.HasSuffix(top, "All · Failing · Running · Mine ─╮") {
		t.Errorf("the top edge is %q, want the title and the tabs", top)
	}
	if w, fw := ansi.StringWidth(top), 120-2*12; w != fw {
		t.Errorf("the top edge is %d wide, want the frame's %d", w, fw)
	}
	// The tabs give way to the title when both don't fit.
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 30})
	if top := topEdge(m); strings.Contains(top, "Mine") || !strings.Contains(top, "Actions") {
		t.Errorf("at 50 columns the top edge is %q, want the title alone", top)
	}
}

// topEdge is the top edge of the modal's frame, without styles.
func topEdge(m *Model) string {
	for l := range strings.SplitSeq(ansi.Strip(m.View().Content), "\n") {
		if i := strings.Index(l, "╭─ Actions"); i >= 0 {
			rest := l[i:]
			if j := strings.Index(rest, "╮"); j >= 0 {
				return rest[:j+len("╮")]
			}
		}
	}
	return ""
}
