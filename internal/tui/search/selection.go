package search

import "github.com/eggzec/gh-tui/internal/tui/ui"

// Selected implements ui.Selector: the result under the cursor, or the
// repository offered before the user types.
func (s *Section) Selected() (ui.Selection, bool) {
	if s.text == "" {
		it, ok := s.starts.selected()
		if !ok || it.repo == nil {
			return ui.Selection{}, false
		}
		return ui.RepoSelection(*it.repo, s.repoURL(*it.repo)), true
	}
	if l, ok := s.visibleHits(); ok {
		hit, ok := l.feed.Selected()
		if !ok {
			return ui.Selection{}, false
		}
		return ui.HitSelection(hit, s.repoURL(hit.Repo)), true
	}
	if l, ok := s.visibleCode(); ok {
		hit, ok := l.feed.Selected()
		if !ok {
			return ui.Selection{}, false
		}
		return ui.Selection{What: "file", URL: hit.URL, Repo: hit.Repo, Path: hit.Path}, true
	}
	return ui.Selection{}, false
}
