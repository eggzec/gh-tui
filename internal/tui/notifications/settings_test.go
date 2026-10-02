package notifications

import (
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
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

// A failed read words its hint in the icon set, from the start and after
// a switch of ui.icons, with refresh bound to enter, which the ASCII set
// names in words.
func TestSettingsIconsErrorHint(t *testing.T) {
	keys := maps.Clone(config.Default().Keys)
	keys[config.ActionRefresh] = []string{"enter"}
	for _, switched := range []bool{false, true} {
		f := newFake()
		f.listErr = fmt.Errorf("list notifications: %w", core.ErrOffline)
		start := config.IconsASCII
		if switched {
			start = config.IconsUnicode
		}
		s := New(t.Context(), f, keys, WithNow(func() time.Time { return now }), WithIcons(ui.NewIcons(start)))
		p, _ := config.Default().Palette(true)
		s.SetTheme(ui.NewTheme(p, true))
		s.SetSize(80, 10)
		run(t, s, s.Init())
		if switched {
			c, err := config.Default().Set("ui.icons", config.IconsASCII)
			if err != nil {
				t.Fatal(err)
			}
			s.Update(ui.SettingsMsg{Config: c})
			s.SetTheme(s.theme)
		}
		v := ansi.Strip(s.View())
		if !strings.Contains(v, "enter to retry") || strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("switched %v: view isn't ASCII or lacks the hint:\n%s", switched, v)
		}
	}
}
