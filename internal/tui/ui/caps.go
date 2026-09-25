package ui

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// Repos reads repositories with what the viewer may do in them, as the
// repositories service does.
type Repos interface {
	CachedGet(ref core.RepoRef) (core.Repo, bool)
	Get(ctx context.Context, ref core.RepoRef) (core.Repo, error)
}

// CapsMsg reports what the viewer may do in Repo. The app sends one once
// it has read the selected repository; a modal on another reads its own
// with LoadCaps.
type CapsMsg struct {
	Repo core.RepoRef
	Caps core.RepoCaps
}

// CachedCaps returns what src holds of the caps of repo, without I/O, or
// unknown caps. src may be nil.
func CachedCaps(src Repos, repo core.RepoRef) core.RepoCaps {
	if src == nil {
		return core.RepoCaps{}
	}
	r, ok := src.CachedGet(repo)
	if !ok {
		return core.RepoCaps{}
	}
	return r.Caps
}

// LoadCaps returns the command that reads the caps of repo from src and
// reports them in a CapsMsg, or nil when src is nil. A fresh cached
// repository costs no request. A failed read reports nothing, which leaves
// the caps unknown.
func LoadCaps(ctx context.Context, src Repos, repo core.RepoRef) tea.Cmd {
	if src == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "repo.caps")
		r, err := src.Get(ctx, repo)
		end(err, "span", "tui", "repo", repo.String())
		if err != nil {
			return nil
		}
		return CapsMsg{Repo: repo, Caps: r.Caps}
	}
}
