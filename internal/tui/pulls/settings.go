package pulls

import (
	"cmp"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// configure keeps the settings of c that the section uses while it runs,
// which the set command changed: the icons, which the theme the app sets
// again after draws with, the dates, and the reads ahead, as WithPrefetch and
// WithFilterPrefetch set them.
func (s *Section) configure(c config.Config) {
	s.icons = ui.NewIcons(c.UI.Icons)
	s.dates = ui.NewDates(c.UI.DateFormat)
	p := c.Details.Prefetch
	s.setAhead(p.Enabled, p.Rows, p.HoverDelay)
	s.setOthers(p.Enabled && p.Filters)
}

// setAhead reads the details of the first rows of each list ahead, and of
// the row the cursor rests on for delay, or reads none ahead unless on.
func (s *Section) setAhead(on bool, rows int, delay time.Duration) {
	switch {
	case !on:
		// Resetting cancels the reads in flight.
		s.ahead.Reset(s.ctx)
		s.ahead = nil
	case s.ahead == nil:
		s.ahead = s.newAhead(rows, delay)
		s.ahead.Reset(s.ctx)
	default:
		s.ahead.Set(rows, delay)
	}
}

// setOthers reads the first pages of the states not shown ahead, or none
// unless on.
func (s *Section) setOthers(on bool) {
	switch {
	case !on:
		s.others.Reset(s.ctx, "")
		s.others = nil
	case s.others == nil:
		s.others = s.newOthers()
		s.others.Reset(s.ctx, s.repo.String())
	}
}

// newAhead returns what reads the details of the rows ahead.
func (s *Section) newAhead(rows int, delay time.Duration) *ui.Ahead[pulls.CommentsQuery] {
	return ui.NewAhead("pull", readDetail(s.svc), s.svc.Current, rows, delay)
}

// newOthers returns what reads the first pages of the states not shown
// ahead.
func (s *Section) newOthers() *ui.Filters[pulls.ListQuery] {
	return ui.NewFilters("pull_filter", readList(s.svc), s.svc.FreshList,
		func(q pulls.ListQuery) string { return cmp.Or(string(q.State), "all") })
}
