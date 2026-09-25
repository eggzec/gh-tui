package tui

import (
	"context"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// historyOpener opens a fake modal and records what it was opened with.
type historyOpener struct {
	modal  *fakeModal
	opened []historyArgs
	loads  int
}

type historyArgs struct {
	repo   core.RepoRef
	branch string
	base   ui.BaseMsg
}

func (h *historyOpener) open(_ context.Context, repo core.RepoRef, branch string, base ui.BaseMsg) (ui.Modal, tea.Cmd) {
	h.opened = append(h.opened, historyArgs{repo, branch, base})
	h.modal = &fakeModal{title: "History"}
	return h.modal, func() tea.Msg { h.loads++; return nil }
}

func TestHistoryKeyOpensTheHistory(t *testing.T) {
	h := &historyOpener{}
	info := func(context.Context, core.RepoRef) (core.Repo, error) { return core.Repo{DefaultBranch: "main"}, nil }
	m, _ := newTestApp(t, WithHistory(h.open), WithRepoInfo(info))
	run(m, m.Init())
	if s := onScreen(m); !strings.Contains(s, "B history") {
		t.Errorf("help lacks the history key:\n%s", s)
	}
	run(m, m.key(press("B")))
	if m.topModal() != h.modal || h.loads != 1 {
		t.Fatalf("B opened %v and loaded %d times, want the history loaded once", m.topModal(), h.loads)
	}
	if want := (historyArgs{repo: testRepo, branch: "main"}); len(h.opened) != 1 || h.opened[0] != want {
		t.Errorf("opened with %+v, want %+v", h.opened, want)
	}
	if h.modal.width == 0 || !h.modal.themed {
		t.Error("the history wasn't sized and themed before it was drawn")
	}
}

func TestHistoryKeyNeedsARepoAndItsScreen(t *testing.T) {
	h := &historyOpener{}
	m, _ := newApp(t, core.RepoRef{}, WithHistory(h.open))
	run(m, m.key(press("B")))
	if s := onScreen(m); strings.Contains(s, "B history") {
		t.Errorf("help offers the history without a repository:\n%s", s)
	}
	m, _ = newTestApp(t, WithHistory(h.open))
	run(m, m.key(press("n")))
	run(m, m.key(press("B")))
	if len(h.opened) != 0 || m.topModal() != nil {
		t.Errorf("B opened the history %d times, want none", len(h.opened))
	}
	m, _ = newTestApp(t)
	run(m, m.key(press("B")))
	if m.topModal() != nil {
		t.Error("B opened a modal without a history")
	}
}

func TestBaseIsInTheHeader(t *testing.T) {
	const sha = "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"
	h := &historyOpener{}
	info := func(context.Context, core.RepoRef) (core.Repo, error) { return core.Repo{DefaultBranch: "main"}, nil }
	m, fakes := newTestApp(t, WithHistory(h.open), WithRepoInfo(info))
	run(m, m.Init())
	isBase := func(msg tea.Msg) bool { _, ok := msg.(ui.BaseMsg); return ok }

	// A base of another repository is left alone.
	run(m, func() tea.Msg { return ui.BaseMsg{Repo: core.RepoRef{Owner: "a", Name: "b"}, Ref: "x", Label: "x"} })
	if fakes[0].got(isBase) {
		t.Error("the base of another repository reached the sections")
	}

	base := ui.BaseMsg{Repo: core.RepoRef{Owner: "EggZec", Name: "gh-tui"}, Ref: sha, Label: "main @ a1b2c3d", Branch: "main"}
	run(m, func() tea.Msg { return base })
	if s := onScreen(m); !strings.Contains(s, "eggzec/gh-tui ─ main @ a1b2c3d") {
		t.Errorf("header lacks the base:\n%s", s)
	}
	if !fakes[0].got(isBase) {
		t.Error("the files missed the base")
	}
	run(m, m.key(press("B")))
	if got := h.opened[0].base; got != base {
		t.Errorf("the history opened on %+v, want the base %+v", got, base)
	}

	run(m, func() tea.Msg { return ui.BaseMsg{Repo: testRepo} })
	if s := onScreen(m); !strings.Contains(s, "eggzec/gh-tui ─ main") || strings.Contains(s, "a1b2c3d") {
		t.Errorf("header shows a base after a reset:\n%s", s)
	}

	// Selecting a repository resets the base.
	run(m, func() tea.Msg { return base })
	run(m, func() tea.Msg { return ui.RepoMsg{Repo: testRepo} })
	if s := onScreen(m); strings.Contains(s, "a1b2c3d") || m.base.Ref != "" {
		t.Errorf("header shows a base after the repository was selected again:\n%s", s)
	}
}

func TestOpenCommitMsgOpensTheHistoryOnTheCommit(t *testing.T) {
	type opened struct {
		repo        core.RepoRef
		sha, branch string
	}
	var got []opened
	mod := &fakeModal{title: "History · charmbracelet/glow"}
	open := func(_ context.Context, repo core.RepoRef, sha, branch string) (ui.Modal, tea.Cmd) {
		got = append(got, opened{repo, sha, branch})
		return mod, nil
	}
	glow := core.RepoRef{Owner: "charmbracelet", Name: "glow"}

	m, _ := newTestApp(t)
	m.Update(ui.OpenCommitMsg{Repo: glow, SHA: "7f75d0e"})
	if m.topModal() != nil {
		t.Error("a commit opened without a history")
	}

	m, _ = newTestApp(t, WithCommit(open))
	run(m, m.key(press("n")))
	m.Update(ui.OpenCommitMsg{Repo: glow, SHA: "7f75d0e"})
	if m.topModal() != mod || len(got) != 1 || got[0] != (opened{glow, "7f75d0e", ""}) {
		t.Errorf("opened %v, want the history of glow on the commit", got)
	}
	m.Update(ui.OpenCommitMsg{Repo: glow})
	if len(got) != 1 {
		t.Error("a message without a commit opened the history")
	}
}
