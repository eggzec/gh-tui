package files

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// configure keeps the settings of c that the section uses while it runs,
// which the set command changed: the icons, which the theme the app sets
// again after draws with, whether the finder shows a preview, which it
// lays out with when it opens, and the reads ahead, as WithPrefetch and
// WithHoverPrefetch set them.
func (s *Section) configure(c config.Config) {
	s.icons = ui.NewIcons(c.UI.Icons)
	s.findPreview = c.Files.Finder.Preview
	p := c.Files.Prefetch
	if !p.Enabled {
		s.prefetchMax, s.hover.max = 0, 0
		// The read in flight stops, and the wait of the cursor is moot.
		s.hover.stop()
		s.hover.seq++
		return
	}
	// The file under the cursor is likely opened next, so it is read up
	// to the size the preview reads.
	s.prefetchMax = int64(p.MaxSize)
	s.hover.delay, s.hover.max = max(p.HoverDelay, 0), int64(c.Files.Preview.MaxSize)
}
