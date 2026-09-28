package notifications

import "github.com/eggzec/gh-tui/internal/tui/ui"

// Selected implements ui.Selector: what the thread under the cursor is
// about.
func (s *Section) Selected() (ui.Selection, bool) {
	n, ok := s.feed.Selected()
	if !ok {
		return ui.Selection{}, false
	}
	return ui.NotificationSelection(n), true
}
