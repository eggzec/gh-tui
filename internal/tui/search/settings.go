package search

import (
	"github.com/eggzec/gh-tui/internal/config"
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

// setPrefetch reads ahead as p says for prefetch.search: the pull requests
// and issues of the results around the cursor, and the first pages of the
// other kinds once the query rests.
func (s *Section) setPrefetch(p config.PrefetchLayers) {
	s.ahead.Configure(p)
	others := ui.Resolve(p, "search", "other_kinds")
	s.othersOn, s.othersWait = others.Enabled, others.Rest
}
