package panes

import "testing"

func TestZoomValue(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name                   string
		user, narrow           bool
		zoomed, userZ, canZoom bool
	}{
		{"neither", false, false, false, false, true},
		{"user", true, false, true, true, true},
		{"narrow", false, true, true, false, false},
		{"both", true, true, true, true, false},
	} {
		z := Zoom{user: tt.user, narrow: tt.narrow}
		if z.Zoomed() != tt.zoomed || z.User() != tt.userZ || z.CanZoom() != tt.canZoom {
			t.Errorf("%s: Zoomed %v User %v CanZoom %v", tt.name, z.Zoomed(), z.User(), z.CanZoom())
		}
	}
}

func TestZoomNarrowForcesAndKeepsUser(t *testing.T) {
	t.Parallel()
	var z Zoom
	z.Toggle()
	z.SetNarrow(true)
	if !z.Zoomed() || !z.User() {
		t.Errorf("narrow: Zoomed %v User %v, want both", z.Zoomed(), z.User())
	}
	z.SetNarrow(false)
	if !z.Zoomed() || !z.User() {
		t.Errorf("grown again: Zoomed %v User %v, want the user's zoom back", z.Zoomed(), z.User())
	}
	z.SetZoom(false)
	z.SetNarrow(true)
	z.SetNarrow(false)
	if z.Zoomed() {
		t.Error("zoomed after the user left it and the terminal grew")
	}
}

func TestZoomToggleNoopWhileNarrow(t *testing.T) {
	t.Parallel()
	var z Zoom
	z.SetNarrow(true)
	z.Toggle()
	z.SetZoom(true)
	if z.User() {
		t.Error("user zoom set while narrow")
	}
	z.SetNarrow(false)
	z.Toggle()
	z.SetNarrow(true)
	z.Toggle()
	if !z.User() {
		t.Error("toggle while narrow cleared the user's zoom")
	}
}

func TestZeroZoomIsOff(t *testing.T) {
	t.Parallel()
	var z Zoom
	if z.Zoomed() || z.User() || !z.CanZoom() {
		t.Errorf("zero value: Zoomed %v User %v CanZoom %v", z.Zoomed(), z.User(), z.CanZoom())
	}
}
