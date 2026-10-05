package pulls

import "github.com/eggzec/gh-tui/internal/tui/ui"

// Selected implements ui.Selector: the pull request under the cursor,
// whose owner is its author.
func (s *Section) Selected() (ui.Selection, bool) {
	if !s.hasRepo || s.feed == nil {
		return ui.Selection{}, false
	}
	pr, ok := s.feed.Selected()
	if !ok {
		return ui.Selection{}, false
	}
	return ui.Selection{What: "pull request", URL: pr.URL, Repo: s.repo, Number: pr.Number, Owner: pr.Author.Login}, true
}
