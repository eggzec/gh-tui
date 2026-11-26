package issues

import "testing"

// descs lists the descriptions of the enabled bindings that the help shows.
func descs(m *detailModal) map[string]bool {
	got := map[string]bool{}
	for _, l := range m.KeyLayers() {
		for _, b := range l.Bindings {
			if b.Enabled() {
				got[b.Help().Desc] = true
			}
		}
	}
	return got
}

func TestZoomKeyReadsUnzoomWhileZoomed(t *testing.T) {
	t.Parallel()
	if got := descs(openDev(t, 120, false)); !got["zoom"] || got["unzoom"] {
		t.Errorf("unzoomed help = %v, want zoom only", got)
	}
	if got := descs(openDev(t, 120, true)); !got["unzoom"] || got["zoom"] {
		t.Errorf("zoomed help = %v, want unzoom only", got)
	}
}

// A short terminal forces the zoom with the user's zoom off, and Tab flips
// the split all the same.
func TestTabReadsSwitchSplitUnderForcedZoom(t *testing.T) {
	t.Parallel()
	m := openDev(t, 120, false)
	m.narrow = true
	m.SetSize(m.width, m.height)
	if !m.ctl.Zoom.Zoomed() || m.ctl.Zoom.User() {
		t.Fatal("a narrow area didn't force the zoom")
	}
	if !m.ctl.Nav.CanStep(m.paneLayout()) {
		t.Skip("the pane has no split here")
	}
	got := descs(m)
	if !got["switch split"] || got["next pane"] || got["unzoom"] {
		t.Errorf("forced zoom help = %v, want switch split, no next pane, no unzoom", got)
	}
}

func TestTabReadsSwitchSplitWhileZoomed(t *testing.T) {
	t.Parallel()
	m := openDev(t, 120, true)
	if !m.ctl.Nav.CanStep(m.paneLayout()) {
		t.Skip("the pane has no split here")
	}
	if got := descs(m); !got["switch split"] || got["next pane"] {
		t.Errorf("zoomed help = %v, want switch split, no next pane", got)
	}
	m = openDev(t, 120, false)
	if got := descs(m); got["switch split"] || !got["next pane"] {
		t.Errorf("unzoomed help = %v, want next pane, no switch split", got)
	}
}
