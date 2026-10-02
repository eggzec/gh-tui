package files

import (
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// configure keeps the settings of c that the section uses while it runs,
// which the set command changed: the icons, which the theme the app sets
// again after draws with, whether the finder shows a preview, which it
// lays out with when it opens, the editor, which the next preview opens
// files in, and the reads ahead, as WithPrefetch sets them, from the next
// move of a cursor.
func (s *Section) configure(c config.Config) {
	s.icons = ui.NewIcons(c.UI.Icons)
	s.voice.Icons = &s.icons
	s.findPreview = c.Files.Finder.Preview
	s.editor = c.Editor
	s.prefetch = newPrefetch(c.Prefetch, c.Files.Preview.MaxSize)
	s.ahead.Configure(s.prefetch.preview)
	s.dirs.Configure(s.prefetch.tree)
}
