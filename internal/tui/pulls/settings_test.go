package pulls

import (
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestSettingsIcons(t *testing.T) {
	h := started(t, newFakeService(), 80, 20)
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
	h := started(t, newFakeService(), 80, 20, WithPrefetch(3, time.Second), WithFilterPrefetch())
	h.Update(ui.SettingsMsg{Config: prefetchConfig(t, "details.prefetch.rows=7", "details.prefetch.hover_delay=2s")})
	if h.ahead == nil || h.others == nil {
		t.Fatal("the reads ahead stopped")
	}
	h.Update(ui.SettingsMsg{Config: prefetchConfig(t, "details.prefetch.filters=false")})
	if h.ahead == nil || h.others != nil {
		t.Errorf("filters off: ahead %v, others %v", h.ahead != nil, h.others != nil)
	}
	h.Update(ui.SettingsMsg{Config: prefetchConfig(t, "details.prefetch.enabled=false")})
	if h.ahead != nil || h.others != nil {
		t.Error("the reads ahead didn't stop")
	}
	h.Update(ui.SettingsMsg{Config: config.Default()})
	if h.ahead == nil || h.others == nil {
		t.Error("the reads ahead didn't start again")
	}
}
