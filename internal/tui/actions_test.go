package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

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
	modal   *tabbedModal
	opened  []core.RepoRef
	filters []core.RunFilter
	loads   int
}

func (a *actionsOpener) open(_ context.Context, repo core.RepoRef, f core.RunFilter) (ui.Modal, tea.Cmd) {
	a.opened = append(a.opened, repo)
	a.filters = append(a.filters, f)
	a.modal = &tabbedModal{title: "Actions · " + repo.String(), names: []string{"All", "Failing", "Running", "Mine"}, active: 1}
	return a.modal, func() tea.Msg { a.loads++; return nil }
}

func TestActionsKeyOpensTheActions(t *testing.T) {
	t.Parallel()
	a := &actionsOpener{}
	m, _ := newTestApp(t, WithActions(a.open))
	run(m, m.Init())
	if s := onScreen(m); !strings.Contains(s, "A actions") {
		t.Errorf("help lacks the actions key:\n%s", s)
	}
	run(m, m.key(press("A")))
	if m.topModal() != a.modal || a.loads != 1 || len(a.opened) != 1 || a.opened[0] != testRepo {
		t.Fatalf("A opened %v on %v and loaded %d times, want the actions of %s loaded once", m.topModal(), a.opened, a.loads, testRepo)
	}
	if a.modal.width == 0 || !a.modal.themed {
		t.Error("the modal wasn't sized and themed before it was drawn")
	}
}

func TestActionsKeyNeedsARepoAndItsScreen(t *testing.T) {
	t.Parallel()
	a := &actionsOpener{}
	m, _ := newApp(t, core.RepoRef{}, WithActions(a.open))
	run(m, m.key(press("A")))
	if s := onScreen(m); strings.Contains(s, "A actions") {
		t.Errorf("help offers the actions without a repository:\n%s", s)
	}
	m, _ = newTestApp(t, WithActions(a.open))
	run(m, m.key(press("I")))
	run(m, m.key(press("A")))
	if len(a.opened) != 0 || m.topModal() != nil {
		t.Errorf("A opened the actions %d times off the repository screen", len(a.opened))
	}
	m, _ = newTestApp(t)
	run(m, m.key(press("A")))
	if m.topModal() != nil {
		t.Error("A opened a modal without actions")
	}
}

func TestFrameShowsTheTabs(t *testing.T) {
	t.Parallel()
	a := &actionsOpener{}
	m, _ := newTestApp(t, WithActions(a.open))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	run(m, m.key(press("A")))
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

// A long title gives way to the tabs at 80 columns, down to a stretch of
// it, so the tabs still show.
func TestFrameShortensALongTitle(t *testing.T) {
	t.Parallel()
	a := &actionsOpener{}
	m, _ := newTestApp(t, WithActions(a.open))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.Update(ui.OpenActionsMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea-app-template"}})
	top := topEdge(m)
	if !strings.Contains(top, "Actions · charmbrace") || !strings.Contains(top, "…") || !strings.HasSuffix(top, "All · Failing · Running · Mine ─╮") {
		t.Errorf("the top edge is %q, want the title shortened and the tabs", top)
	}
	golden.RequireEqual(t, m.View().Content)
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

func TestOpenActionsMsgOpensTheRunsOfAnyRepo(t *testing.T) {
	t.Parallel()
	a := &actionsOpener{}
	m, _ := newTestApp(t, WithActions(a.open))
	run(m, m.key(press("I")))
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	f := core.RunFilter{Branch: "feat/x", Status: "failure"}
	m.Update(ui.OpenActionsMsg{Repo: other, Filter: f})
	if m.topModal() != a.modal || len(a.opened) != 1 || a.opened[0] != other || a.filters[0] != f {
		t.Errorf("opened %v with %v, want the failed runs of the branch of %s", a.opened, a.filters, other)
	}
}

func TestOpenReleaseMsgOpensTheRelease(t *testing.T) {
	t.Parallel()
	type opened struct {
		repo core.RepoRef
		id   int64
		url  string
	}
	var got []opened
	mod := &fakeModal{title: "v3.0.0 · charmbracelet/glow"}
	loads := 0
	open := func(_ context.Context, repo core.RepoRef, id int64, url string) (ui.Modal, tea.Cmd) {
		got = append(got, opened{repo, id, url})
		return mod, func() tea.Msg { loads++; return nil }
	}
	msg := ui.OpenReleaseMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "glow"}, ID: 368759772, URL: "https://github.com/charmbracelet/glow/releases"}

	m, _ := newTestApp(t)
	m.Update(msg)
	if m.topModal() != nil {
		t.Error("a release opened without a release modal")
	}

	m, _ = newTestApp(t, WithRelease(open))
	run(m, m.key(press("I")))
	_, cmd := m.Update(msg)
	run(m, cmd)
	if m.topModal() != mod || loads != 1 || len(got) != 1 || got[0] != (opened{msg.Repo, msg.ID, msg.URL}) {
		t.Errorf("opened %v and loaded %d times, want the release of the message loaded once", got, loads)
	}
	if mod.width == 0 || !mod.themed {
		t.Error("the modal wasn't sized and themed before it was drawn")
	}
}
