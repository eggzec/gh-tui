package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// descs lists the descriptions of the enabled bindings that the help shows.
func descs(layers []keyhelp.Layer) map[string]bool {
	got := map[string]bool{}
	for _, l := range layers {
		for _, b := range l.Bindings {
			if b.Enabled() {
				got[b.Help().Desc] = true
			}
		}
	}
	return got
}

func TestPullModalZoomKeyReadsUnzoom(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, 160)
	if got := descs(m.KeyLayers()); !got["zoom"] || got["unzoom"] {
		t.Fatalf("unzoomed help = %v, want zoom only", got)
	}
	m.press(keyMsg("z"))
	if got := descs(m.KeyLayers()); !got["unzoom"] || got["zoom"] {
		t.Errorf("zoomed help = %v, want unzoom only", got)
	}
	m.press(keyMsg("z"))
	if got := descs(m.KeyLayers()); !got["zoom"] || got["unzoom"] {
		t.Errorf("help after unzooming = %v, want zoom only", got)
	}
}

// A narrow area forces the zoom with the user's zoom off, and Tab flips the
// split all the same.
func TestPullModalTabReadsSwitchSplitForcedZoom(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, wideMin-1)
	m.press(keyMsg("6"))
	if !m.ctl.Zoom.Zoomed() || m.ctl.Zoom.User() {
		t.Fatal("a narrow area didn't force the zoom")
	}
	if !m.ctl.Nav.CanStep(m.layout()) {
		t.Skip("the files pane has no split at this size")
	}
	got := descs(m.KeyLayers())
	if !got["switch split"] || got["next pane"] || got["unzoom"] {
		t.Errorf("forced zoom help = %v, want switch split, no next pane, no unzoom", got)
	}
}

func TestPullModalTabReadsSwitchSplitZoomed(t *testing.T) {
	t.Parallel()
	m := openedPanes(t, 160)
	m.press(keyMsg("6"))
	if got := descs(m.KeyLayers()); got["switch split"] {
		t.Errorf("unzoomed help = %v, want no switch split", got)
	}
	m.press(keyMsg("z"))
	if !m.ctl.Nav.CanStep(m.layout()) {
		t.Skip("the files pane has no split at this size")
	}
	got := descs(m.KeyLayers())
	if !got["switch split"] || got["next pane"] {
		t.Errorf("zoomed help = %v, want switch split, no next pane", got)
	}
	m.press(keyMsg("z"))
	got = descs(m.KeyLayers())
	if got["switch split"] || !got["next pane"] {
		t.Errorf("unzoomed help = %v, want next pane, no switch split", got)
	}
}
