package files

import (
	"slices"
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
	c.Prefetch.Files.Preview.MaxSize, c.Prefetch.Files.Rest = 32*config.KiB, new(time.Second)
	c.Prefetch.Finder.Preview.Window.After = new(2)
	h.Update(ui.SettingsMsg{Config: c})
	p := h.prefetch
	if !p.preview.Enabled || p.preview.Rest != time.Second || p.preview.Window.After != 32 || p.previewMax != 32<<10 {
		t.Errorf("tree reads %+v up to %d, want after 32 up to 32KiB, after 1s", p.preview, p.previewMax)
	}
	if p.cursorMax != int64(c.Files.Preview.MaxSize) {
		t.Errorf("the file under the cursor is read up to %d, want the preview's size", p.cursorMax)
	}
	if p.finder.Window.After != 2 || p.finder.Rest != 100*time.Millisecond {
		t.Errorf("finder reads %+v, want after 2, after 100ms", p.finder)
	}
	c.Prefetch.Files.Preview.Enabled = new(false)
	h.Update(ui.SettingsMsg{Config: c})
	if h.prefetch.preview.Enabled {
		t.Error("the tree still reads ahead")
	}
	// Moving the cursor reads nothing.
	f := h.svc.(*fake)
	keys(h, slices.Repeat([]string{"down"}, rowAgents)...)
	if got := f.blobSHAs(); len(got) != 0 {
		t.Errorf("read %q, want nothing", got)
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

func TestSettingsEditor(t *testing.T) {
	h := loaded(t, sampleFake(), 60, 12)
	c, err := config.Default().Set("editor", "code --wait")
	if err != nil {
		t.Fatal(err)
	}
	h.Update(ui.SettingsMsg{Config: c})
	if h.editor != "code --wait" {
		t.Errorf("editor %q, want the one set", h.editor)
	}
}
