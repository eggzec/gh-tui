package dashboard

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/details"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// configure keeps the settings of c that the section uses while it runs,
// which the set command changed: the icons, which the theme the app sets
// again after draws with, and the reads ahead of the work and of the
// inbox's threads.
func (s *Section) configure(c config.Config) {
	s.icons = ui.NewIcons(c.UI.Icons)
	s.opener.Configure(c)
	p := s.prefetch
	on, delay := c.DashboardPrefetch(), c.Details.Prefetch.HoverDelay
	switch {
	case !on:
		// Resetting cancels the reads in flight.
		s.ahead.Reset(s.ctx)
		s.ahead = nil
	case p == nil:
		// Nothing was given to read with.
	case s.ahead == nil:
		s.ahead = details.NewAhead("work", p.pulls, p.issues, aheadRows, delay)
		s.ahead.Reset(s.ctx)
	default:
		s.ahead.Set(aheadRows, delay)
	}
}
