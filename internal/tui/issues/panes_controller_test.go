package issues

import "testing"

func TestZoomedTabFlipsAndKeepsPane(t *testing.T) {
	t.Parallel()
	m := openDev(t, 120, true)
	pane := m.ctl.Nav.Focus()
	if _, ok := m.ctl.Tab(m.paneLayout(), 1); !ok || m.ctl.Nav.Focus() != pane || !m.ctl.Nav.InPreview(m.paneLayout()) {
		t.Fatalf("tab zoomed: focus %v, preview %v; want pane %v in its preview", m.ctl.Nav.Focus(), m.ctl.Nav.InPreview(m.paneLayout()), pane)
	}
	if _, ok := m.ctl.Tab(m.paneLayout(), 1); !ok || m.ctl.Nav.Focus() != pane || m.ctl.Nav.InPreview(m.paneLayout()) {
		t.Errorf("second tab: focus %v; want the list of pane %v", m.ctl.Nav.Focus(), pane)
	}
}

func TestZoomedTabWithoutPreviewDoesNothing(t *testing.T) {
	t.Parallel()
	m := openDev(t, 120, true)
	l := m.paneLayout()
	m.ctl.Digit(l, paneConversation)
	if m.ctl.Nav.CanStep(l) {
		t.Skip("the conversation has a preview here")
	}
	if p, changed := m.ctl.Tab(l, 1); changed || p != paneConversation {
		t.Errorf("tab on a pane with no preview = %v, %v; want nothing", p, changed)
	}
}

func TestUnzoomedTabCyclesAndDigitWorksZoomed(t *testing.T) {
	t.Parallel()
	m := openDev(t, 120, false)
	before := m.ctl.Nav.Focus()
	if p, changed := m.ctl.Tab(m.paneLayout(), 1); !changed || p == before {
		t.Errorf("unzoomed tab = %v, %v; want another pane", p, changed)
	}
	m = openDev(t, 120, true)
	if !m.ctl.Digit(m.paneLayout(), paneConversation) || m.ctl.Nav.Focus() != paneConversation || !m.ctl.Zoom.Zoomed() {
		t.Errorf("digit zoomed: focus %v zoomed %v; want the conversation, still zoomed", m.ctl.Nav.Focus(), m.ctl.Zoom.Zoomed())
	}
}

func TestZoomGatedWhileNarrow(t *testing.T) {
	t.Parallel()
	m := openDev(t, 90, false)
	if m.ctl.Toggle() {
		t.Error("the zoom key worked while narrow")
	}
}

func TestBackspaceDropsUserZoomFirst(t *testing.T) {
	t.Parallel()
	h := started(t, blockedSvc(), 120, 30)
	press(t, h, "down", "enter", "4", "z")
	m := h.modal()
	if m == nil || !m.ctl.Zoom.User() {
		t.Fatal("z didn't zoom")
	}
	if _, ok := m.Act("back"); !ok || m.ctl.Zoom.User() {
		t.Errorf("back: handled %v, user zoom %v; want it handled and unzoomed", ok, m.ctl.Zoom.User())
	}
	if _, ok := m.Act("back"); ok {
		t.Error("back with no zoom left was handled; the app should step back")
	}
}
