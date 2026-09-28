package dashboard

import (
	"fmt"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestSettingsIcons(t *testing.T) {
	h := newSection(t, newFake(), nil, 140, 38)
	c, err := config.Default().Set("ui.icons", config.IconsASCII)
	if err != nil {
		t.Fatal(err)
	}
	h.Update(ui.SettingsMsg{Config: c})
	if got := h.icons.Star; got != ui.NewIcons(config.IconsASCII).Star {
		t.Errorf("star = %q, want the ASCII one", got)
	}
}

// Switching the icons marks every error with the new set's glyph at once,
// the error lines of the section and the lists' alike, as the app applies
// the settings and then the theme.
func TestSettingsErrorMark(t *testing.T) {
	offline := fmt.Errorf("github: GET /graphql: %w", core.ErrOffline)
	svc := newFake()
	for _, what := range []string{"header", "work", "repos @me@"} {
		svc.fail[what] = offline
	}
	s := newSection(t, svc, &fakeInbox{threads: inboxThreads(), err: offline}, 140, 38)
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	// From the default Nerd Font set to each of the others, and back.
	for _, set := range []string{config.IconsUnicode, config.IconsASCII, config.IconsNerd} {
		c, err := config.Default().Set("ui.icons", set)
		if err != nil {
			t.Fatal(err)
		}
		s.Update(ui.SettingsMsg{Config: c})
		s.SetTheme(ui.NewTheme(p, true))
		view := screen(s)
		// The profile, and the pinned, work, repositories and
		// notifications panes.
		mark := ui.NewIcons(set).Error
		if n := strings.Count(view, mark+" Can't reach GitHub"); n != 5 {
			t.Errorf("%s: the dashboard marks %d errors with %q, want 5:\n%s", set, n, mark, view)
		}
		if n := strings.Count(view, "Can't reach GitHub"); n != 5 {
			t.Errorf("%s: the dashboard shows %d errors, want 5:\n%s", set, n, view)
		}
	}
}

func TestSettingsPrefetch(t *testing.T) {
	f := &detailFake{}
	s := newSection(t, newFake(), nil, 140, 38, WithDetails(fakePulls{f}, fakeIssues{f}))
	if s.ahead != nil {
		t.Fatal("reads ahead without WithPrefetch")
	}
	s.Update(ui.SettingsMsg{Config: config.Default()})
	if s.ahead == nil {
		t.Fatal("the settings didn't turn the reads ahead on")
	}
	off := config.Default()
	off.Details.Prefetch.Enabled = false
	s.Update(ui.SettingsMsg{Config: off})
	if s.ahead != nil {
		t.Error("the reads ahead didn't stop")
	}
	bare := newSection(t, newFake(), nil, 140, 38)
	bare.Update(ui.SettingsMsg{Config: config.Default()})
	if bare.ahead != nil {
		t.Error("a section with nothing to read with reads ahead")
	}
}

func TestSettingsCalendar(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	c := config.Default()
	c.Dashboard.CalendarGlyph, c.Dashboard.Contributions = "#", config.Contributions30d
	s.Update(ui.SettingsMsg{Config: c})
	if s.cal.Glyph() != "#" || s.cal.Range() != 30 || s.calDays != 30 {
		t.Errorf("glyph %q, range %d; want # over 30 days", s.cal.Glyph(), s.cal.Range())
	}
}
