package notifications

import (
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
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

// A switch to the ASCII icons draws the list in ASCII, its dots, header,
// separators and the cuts of narrow columns too, apart from what GitHub
// wrote.
func TestSettingsIconsASCII(t *testing.T) {
	threads := inbox()
	s := newSection(t, newFake(threads...), 60, 12)
	c, err := config.Default().Set("ui.icons", config.IconsASCII)
	if err != nil {
		t.Fatal(err)
	}
	s.Update(ui.SettingsMsg{Config: c})
	s.SetTheme(s.theme)
	v := ansi.Strip(s.View())
	for _, n := range threads {
		v = strings.ReplaceAll(v, n.Subject.Title, "")
	}
	if strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
		t.Errorf("view isn't ASCII:\n%s", v)
	}
	if !strings.Contains(v, "* ") {
		t.Errorf("view lacks the unread dot:\n%s", v)
	}
}
