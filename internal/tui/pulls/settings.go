package pulls

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// configure keeps the settings of c that the section uses while it runs,
// which the set command changed. The app sets the theme again after, which
// draws with them.
func (s *Section) configure(c config.Config) {
	s.icons = ui.NewIcons(c.UI.Icons)
}
