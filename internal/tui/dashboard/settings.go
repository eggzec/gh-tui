package dashboard

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// configure keeps the settings of c that the section uses while it runs,
// which the set command changed: the icons, which the theme the app sets
// again after draws with, the dates, the glyph and range of the
// calendar, and the reads ahead of the work, of the repositories and of
// the inbox's threads.
func (s *Section) configure(c config.Config) {
	s.icons = ui.NewIcons(c.UI.Icons)
	if d := ui.NewDates(c.UI.DateFormat); d != s.dates {
		// The work wraps its titles around the dates, and the
		// repositories keep a column as wide as the widest.
		s.dates, s.tasks.dates = d, d
		s.tasks.wrap()
		s.repos.resize(s.repos.width, s.repos.height)
	}
	s.glyph = c.Dashboard.CalendarGlyph
	s.cal.SetGlyph(cellGlyph(s.glyph, s.icons))
	if days := c.Dashboard.ContributionDays(); days != s.calDays {
		s.calDays = days
		s.cal.SetRange(days)
		if s.contribs.ok {
			// What the calendar says of a range without contributions
			// names it.
			s.setContributions()
		}
	}
	s.opener.Configure(c)
	s.setPrefetch(c.Prefetch)
}
