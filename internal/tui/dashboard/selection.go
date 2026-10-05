package dashboard

import "github.com/eggzec/gh-tui/internal/tui/ui"

// Selected implements ui.Selector: what the cursor of the focused pane is
// on. The calendar has nothing to select.
func (s *Section) Selected() (ui.Selection, bool) {
	switch s.focus {
	case pinnedPane:
		if c, ok := s.pinned.Selected(); ok {
			return ui.RepoSelection(c.Repo, s.repoURL(c.Repo)), true
		}
	case reposPane:
		if r, ok := s.repos.selected(); ok {
			return ui.RepoSelection(r, s.repoURL(r)), true
		}
	case workPane:
		if hit, ok := s.tasks.selected(); ok {
			return ui.HitSelection(hit, s.repoURL(hit.Repo)), true
		}
	case inboxPane:
		if n, ok := s.threads.selected(); ok {
			return ui.NotificationSelection(n), true
		}
	default:
	}
	return ui.Selection{}, false
}
