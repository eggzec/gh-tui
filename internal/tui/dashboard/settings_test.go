package dashboard

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
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
