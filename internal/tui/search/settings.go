package search

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// configure keeps the settings of c that the section uses while it runs,
// which the set command changed: the icons, which the theme the app sets
// again after draws with, and the reads ahead of the result under the
// cursor.
func (s *Section) configure(c config.Config) {
	s.icons = ui.NewIcons(c.UI.Icons)
	p := s.prefetch
	on, delay := c.Details.Prefetch.Enabled, c.Details.Prefetch.HoverDelay
	switch {
	case !on:
		// Resetting cancels the reads in flight.
		s.ahead.Reset(s.ctx)
		s.ahead = nil
	case p == nil:
		// Nothing was given to read with.
	case s.ahead == nil:
		s.ahead = details.NewAhead("search_hit", p.pulls, p.issues, 0, delay)
		s.ahead.Reset(s.textCtx)
	default:
		s.ahead.Set(0, delay)
	}
}
