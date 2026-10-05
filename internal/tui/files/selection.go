package files

import "github.com/eggzec/gh-tui/internal/tui/ui"

// Selected implements ui.Selector: the file or directory under the
// cursor, at the commit the files are shown at if they are shown at one.
func (s *Section) Selected() (ui.Selection, bool) {
	if s.tree == nil {
		return ui.Selection{}, false
	}
	e := s.selected()
	sel := ui.Selection{What: "file", URL: webURL(s.host, s.repo, s.ref, e), Repo: s.repo, Path: e.Path, Owner: s.repo.Owner}
	switch {
	case e.Path == "":
		sel.What = "repository"
	case e.Dir():
		sel.What = "directory"
	}
	if shortRef(s.ref) != s.ref {
		sel.SHA = s.ref
	}
	return sel, true
}
