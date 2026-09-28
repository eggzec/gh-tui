package files

import (
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestSettingsIcons(t *testing.T) {
	h := loaded(t, sampleFake(), 60, 12)
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
	h := loaded(t, sampleFake(), 60, 12)
	c := config.Default()
	c.Files.Prefetch.MaxSize, c.Files.Prefetch.HoverDelay = 32*config.KiB, time.Second
	h.Update(ui.SettingsMsg{Config: c})
	if h.prefetchMax != 32<<10 || h.hover.max != int64(c.Files.Preview.MaxSize) || h.hover.delay != time.Second {
		t.Errorf("top %d, hover %d after %v; want 32KiB, the preview's size after 1s", h.prefetchMax, h.hover.max, h.hover.delay)
	}
	seq := h.hover.seq
	c.Files.Prefetch.Enabled = false
	h.Update(ui.SettingsMsg{Config: c})
	if h.prefetchMax != 0 || h.hover.max != 0 || h.hover.seq == seq {
		t.Errorf("top %d, hover %d; want nothing read ahead, and the wait of the cursor moot", h.prefetchMax, h.hover.max)
	}
}

func TestSettingsFinderPreview(t *testing.T) {
	h := loaded(t, sampleFake(), 60, 12)
	c := config.Default()
	c.Files.Finder.Preview = false
	h.Update(ui.SettingsMsg{Config: c})
	if h.findPreview {
		t.Error("the finder still shows a preview")
	}
}
