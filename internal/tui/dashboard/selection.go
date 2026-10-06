package dashboard

import (
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Selected implements ui.Selector: what the cursor of the focused pane is
// on. The calendar has nothing to select. The owner of a task is its
// author, unless an app, or its repository's owner when it has none; that
// of a thread of the inbox is its repository's.
func (s *Section) Selected() (ui.Selection, bool) {
	switch s.focus {
	case pinnedPane:
		if c, ok := s.pinned.Selected(); ok {
			return ui.RepoSelection(c.Repo, s.repoURL(c.Repo)), true
		}
	case reposPane:
		return s.repos.selection()
	case workPane:
		if hit, ok := s.tasks.selected(); ok {
			sel := ui.HitSelection(hit, s.repoURL(hit.Repo))
			if hit.Kind != core.SearchRepos && hit.Issue.Author.Login == "" {
				// A deleted account leaves the repository's owner.
				sel.Owner = hit.Issue.Repo.Owner
			}
			return sel, true
		}
	case inboxPane:
		if n, ok := s.threads.selected(); ok {
			return ui.NotificationSelection(n), true
		}
	default:
	}
	return ui.Selection{}, false
}
