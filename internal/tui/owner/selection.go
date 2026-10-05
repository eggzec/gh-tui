package owner

import "github.com/eggzec/gh-tui/internal/tui/ui"

// Selected implements ui.Selector: what the cursor of the focused pane is
// on.
func (s *Section) Selected() (ui.Selection, bool) {
	p := s.page
	if p == nil {
		return ui.Selection{}, false
	}
	switch p.focus {
	case pinnedPane:
		if c, ok := p.pinned.Selected(); ok {
			return ui.RepoSelection(c.Repo, s.repoURL(c.Repo)), true
		}
	case listPane:
		if l := p.repos; l != nil {
			if r, ok := l.Feed.Selected(); ok {
				return ui.RepoSelection(r, s.repoURL(r)), true
			}
		}
	default:
	}
	return ui.Selection{}, false
}
