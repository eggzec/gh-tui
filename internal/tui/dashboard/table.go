package dashboard

import (
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ownerui"
)

// renderRepo renders a repository of the list in the columns of the tab:
// its name, flags, description, language, stars and age.
func (s *Section) renderRepo(c ownerui.Cols, r core.Repo, selected bool) string {
	return s.drawer().Row(c, r, selected, s.dates, s.now)
}

// langPaint returns the paint of the language glyph of r, built once per
// language and theme.
func (s *Section) langPaint(r core.Repo) ownerui.Paint {
	k := r.Language + "\x00" + r.LanguageColor
	if p, ok := s.st.langs[k]; ok {
		return p
	}
	p := ownerui.NewPaint(s.theme.Language(r.Language, r.LanguageColor))
	if s.st.langs == nil {
		s.st.langs = map[string]ownerui.Paint{}
	}
	s.st.langs[k] = p
	return p
}
