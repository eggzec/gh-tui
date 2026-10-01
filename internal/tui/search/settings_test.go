package search

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

func TestSettingsIcons(t *testing.T) {
	h := newSection(t, newFake(), 120, 30)
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
	s := newSection(t, newFake(), 120, 30, WithDetails(fakePulls{f}, fakeIssues{f}))
	if s.ahead.On() || !s.othersOn {
		t.Fatal("without WithPrefetch, want only the other kinds read ahead")
	}
	s.Update(ui.SettingsMsg{Config: config.Default()})
	if !s.ahead.On() || !s.othersOn || s.othersWait != defaultOthersWait {
		t.Fatal("the settings didn't turn the reads ahead on")
	}
	kinds := config.Default()
	kinds.Prefetch.Search.OtherKinds.Enabled = new(false)
	s.Update(ui.SettingsMsg{Config: kinds})
	if !s.ahead.On() || s.othersOn {
		t.Errorf("other kinds off: ahead %v, other kinds %v", s.ahead.On(), s.othersOn)
	}
	off := config.Default()
	off.Prefetch.Search.Enabled = new(false)
	s.Update(ui.SettingsMsg{Config: off})
	if s.ahead.On() || s.othersOn {
		t.Error("the reads ahead didn't stop")
	}
}

// Setting the date format tells the dates of the results in it at once,
// the repositories' in a column that keeps room for the widest.
func TestSettingsDateFormat(t *testing.T) {
	f := flagged()
	s := newSection(t, f, 140, 22)
	typeText(t, s, "tea")
	s.Update(ui.SettingsMsg{Config: uitest.DateFormat(t)})
	s.SetTheme(s.theme)
	uitest.Dated(t, s.View(), 140, f.repos[0].Repo.UpdatedAt, f.repos[1].Repo.UpdatedAt)
}
