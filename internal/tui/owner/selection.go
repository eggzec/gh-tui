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
		if l := p.list(); l != nil {
			return l.selection(s)
		}
	default:
		return s.sideSelected()
	}
	return ui.Selection{}, false
}
