package issues

import "github.com/eggzec/gh-tui/internal/tui/ui"

// Selected implements ui.Selector: the issue under the cursor.
func (s *Section) Selected() (ui.Selection, bool) {
	if !s.hasRepo {
		return ui.Selection{}, false
	}
	it, ok := s.list.Selected()
	if !ok {
		return ui.Selection{}, false
	}
	return ui.Selection{What: "issue", URL: it.URL, Repo: s.repo, Number: it.Number}, true
}
