package owner

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/markdown"
)

// sideConf is what the README beside the list shares across the pages.
type sideConf struct {
	// md renders the READMEs, once the first is.
	md *markdown.Renderer
	// keys are those of the README's pager.
	keys readmeKeys
	// focus is the pane each page opens with the focus on.
	focus paneID
}

func newSideConf(keys config.Keymap) sideConf {
	def := baseConfig()
	return sideConf{keys: newReadmeKeys(keys), focus: focusFor(def.Owner.DefaultTab)}
}

// focusFor returns the pane a page opens with the focus on, for
// owner.default_tab name: the README's for the README, and the list's for
// the tabs.
func focusFor(name string) paneID {
	if name == config.OwnerTabReadme {
		return readmePane
	}
	return listPane
}

// themeSide restyles the pagers and the markdown of the pages.
func (s *Section) themeSide() {
	if s.sc.md != nil {
		s.sc.md.SetStyle(s.theme.Thread(s.icons).Markdown)
		s.sc.md.SetGlyphs(s.icons.Markdown)
	}
	for _, p := range s.pages() {
		if pg := p.side.pager; pg != nil {
			pg.SetStyles(s.theme.Pager(s.icons))
			pg.Rerender()
		}
	}
}

// redrawSide draws the README of the page on view again, with the images
// that arrived or failed since, if any may change it.
func (s *Section) redrawSide() {
	if p := s.page; p != nil && p.side.pager != nil && s.readmeStale(p) {
		p.side.pager.Rerender()
	}
}
