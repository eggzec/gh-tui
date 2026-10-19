package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// canForkSwitch reports whether the fork key does something: only the
// repository screen has a repository whose fork or parent to go to.
func (m *Model) canForkSwitch() bool {
	return m.repos != nil && m.screen == repoScreen && m.repo != (core.RepoRef{})
}

// forkSwitch goes from the repository of the screen to the one it is a fork
// of, else to the fork of it that the viewer has, else says there is none.
// A fork that the viewer forked again goes to its parent.
func (m *Model) forkSwitch() tea.Cmd {
	repo := m.repo
	r, ok := m.repos.CachedGet(repo)
	if !ok || !r.Caps.Known {
		// A list of repositories doesn't say which are forked, so the answer
		// waits for the read of the repository on its own.
		return ui.Notify(toast.Info, "Still reading "+repo.String()+", so press the key again in a moment.")
	}
	switch {
	case r.Parent != (core.RepoRef{}):
		return m.gotoRepo(r.Parent)
	case r.ViewerFork != (core.RepoRef{}):
		return m.gotoRepo(r.ViewerFork)
	}
	return ui.Notify(toast.Info, repo.String()+" has no fork or parent to go to.")
}
