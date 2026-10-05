package owner

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// configure keeps the settings of c that the section uses while it runs,
// which the set command changed: the icons, which the theme the app sets
// again after draws with, and the dates, whose width the lists of
// repositories keep a column for.
func (s *Section) configure(c config.Config) {
	s.icons = ui.NewIcons(c.UI.Icons)
	s.voice.Icons = &s.icons
	if d := ui.NewDates(c.UI.DateFormat); d != s.dates {
		s.dates = d
		s.layout()
	}
}
