package issues

import (
	"github.com/eggzec/gh-tui/internal/config"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// configure keeps the settings of c that the section uses while it runs,
// which the set command changed: the icons, which the theme the app sets
// again after draws with, the dates, and the reads ahead, as WithPrefetch
// sets them.
func (s *Section) configure(c config.Config) {
	s.icons = ui.NewIcons(c.UI.Icons)
	s.dates = ui.NewDates(c.UI.DateFormat)
	s.setPrefetch(c.Prefetch)
}

// setPrefetch reads ahead as p says for prefetch.issues.
func (s *Section) setPrefetch(p config.PrefetchLayers) {
	s.ahead.Configure(p)
	s.setOthers(ui.Resolve(p, "issues", "other_tabs").Enabled)
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

// newOthers returns what reads the first pages of the states not shown
// ahead.
func (s *Section) newOthers() *ui.Filters[issuesvc.ListQuery] {
	return ui.NewFilters("issue_filter", readList(s.svc), s.svc.FreshList,
		func(q issuesvc.ListQuery) string { return string(q.State) })
}
