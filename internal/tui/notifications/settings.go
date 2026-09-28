package notifications

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// configure takes the settings of c that the set command changed: the
// icons, which the theme the app sets next draws with, and what the
// opener, which reads the threads ahead, reads.
func (s *Section) configure(c config.Config) {
	s.icons = ui.NewIcons(c.UI.Icons)
	s.opener.Configure(c)
}
