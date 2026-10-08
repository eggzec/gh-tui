package search

import "github.com/eggzec/gh-tui/internal/tui/ui"

// OpenedRepo implements ui.RepoOpener: opening the repository of a result
// keeps the query that found it among the recent searches.
func (s *Section) OpenedRepo(ui.Selection) {
	if s.text != "" {
		s.remember(s.text)
	}
}

// Selected implements ui.Selector: the result under the cursor, or the
// repository offered before the user types. With the query focused no
// result is.
func (s *Section) Selected() (ui.Selection, bool) {
	if s.area == inputArea {
		return ui.Selection{}, false
	}
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
		return ui.Selection{What: "file", URL: hit.URL, Repo: hit.Repo, Path: hit.Path, Owner: hit.Repo.Owner}, true
	}
	return ui.Selection{}, false
}
