package issues

import (
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

func TestSettingsIcons(t *testing.T) {
	h := started(t, newFakeService(sampleIssues(3)), 80, 20)
	c, err := config.Default().Set("ui.icons", config.IconsASCII)
	if err != nil {
		t.Fatal(err)
	}
	h.Update(ui.SettingsMsg{Config: c})
	if got := h.icons.Star; got != ui.NewIcons(config.IconsASCII).Star {
		t.Errorf("star = %q, want the ASCII one", got)
	}
}

// prefetchConfig returns the defaults with each of set, key=value, set.
func prefetchConfig(t *testing.T, set ...string) config.Config {
	t.Helper()
	c := config.Default()
	for _, kv := range set {
		k, v, _ := strings.Cut(kv, "=")
		var err error
		if c, err = c.Set(k, v); err != nil {
			t.Fatal(err)
		}
	}
	return c
}

func TestSettingsPrefetch(t *testing.T) {
	h := started(t, newFakeService(sampleIssues(3)), 80, 20, readingAhead(2, time.Second, true))
	h.Update(ui.SettingsMsg{Config: prefetchConfig(t, "prefetch.issues.window.after=6", "prefetch.issues.rest=2s")})
	if !h.ahead.On() || h.others == nil {
		t.Fatal("the reads ahead stopped")
	}
	h.Update(ui.SettingsMsg{Config: prefetchConfig(t, "prefetch.issues.other_tabs.enabled=false")})
	if !h.ahead.On() || h.others != nil {
		t.Errorf("other tabs off: ahead %v, others %v", h.ahead.On(), h.others != nil)
	}
	h.Update(ui.SettingsMsg{Config: prefetchConfig(t, "prefetch.issues.enabled=false")})
	if h.ahead.On() || h.others != nil {
		t.Error("the reads ahead didn't stop")
	}
	h.Update(ui.SettingsMsg{Config: config.Default()})
	if !h.ahead.On() || h.others == nil {
		t.Error("the reads ahead didn't start again")
	}
}

// TestSettingsPrefetchOnReadsTheListShown checks that turning the reads
// ahead on reads the first rows of the list already on screen, rather
// than waiting for the next list.
func TestSettingsPrefetchOnReadsTheListShown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newFakeService(sampleIssues(12))
		h := started(t, svc, 80, 30)
		off := prefetchConfig(t, "prefetch.issues.enabled=false")
		run(t, h, h.Update(ui.SettingsMsg{Config: off}))
		on, err := off.Set("prefetch.issues.enabled", "true")
		if err != nil {
			t.Fatal(err)
		}
		on, err = on.Set("prefetch.issues.window.after", "2")
		if err != nil {
			t.Fatal(err)
		}
		run(t, h, h.Update(ui.SettingsMsg{Config: on}))
		if got, want := sorted(svc.getCalls()), []int{998, 999, 1000}; !slices.Equal(got, want) {
			t.Errorf("read issues %v, want the first rows %v", got, want)
		}
	})
}

// Setting the date format tells the dates of the rows in it at once, in a
// column that keeps room for the widest, and those of the modal opened
// after.
func TestSettingsDateFormat(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 100, 30)
	h.Update(ui.SettingsMsg{Config: uitest.DateFormat(t)})
	h.SetTheme(testTheme())
	is := sampleIssues(12)
	uitest.Dated(t, h.View(), 100, is[0].UpdatedAt, is[1].UpdatedAt)
	press(t, h, "enter")
	m := h.modal()
	if m == nil {
		t.Fatal("enter didn't open the issue")
	}
	uitest.Dated(t, m.View(), 100, is[0].CreatedAt, is[0].UpdatedAt)
}
