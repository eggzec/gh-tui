package notifications

import (
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// Setting the date format tells the dates of the threads in it at once,
// in a column that keeps room for the widest.
func TestSettingsDateFormat(t *testing.T) {
	threads := inbox()
	s := newSection(t, newFake(threads...), 100, 12)
	s.Update(ui.SettingsMsg{Config: uitest.DateFormat(t)})
	s.SetTheme(s.theme)
	var unread []time.Time
	for _, n := range threads {
		if n.Unread {
			unread = append(unread, n.UpdatedAt)
		}
	}
	uitest.Dated(t, s.View(), 100, unread...)
}
