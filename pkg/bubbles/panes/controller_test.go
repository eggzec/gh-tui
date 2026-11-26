package panes

import "testing"

func TestControllerToggle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		narrow     bool
		wantOK     bool
		wantZoom   bool
		wantSub    Role
		startInSub Role
	}{
		{"gated while narrow keeps the split focus", true, false, false, Preview, Preview},
		{"resets the split focus", false, true, true, List, Preview},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var c Controller
			c.Nav.SetFocus(2)
			c.Nav.sub[1] = tt.startInSub
			c.Zoom.SetNarrow(tt.narrow)
			if got := c.Toggle(); got != tt.wantOK {
				t.Errorf("Toggle = %v, want %v", got, tt.wantOK)
			}
			if c.Zoom.User() != tt.wantZoom {
				t.Errorf("user zoom = %v, want %v", c.Zoom.User(), tt.wantZoom)
			}
			if c.Nav.Sub(2) != tt.wantSub {
				t.Errorf("sub = %v, want %v", c.Nav.Sub(2), tt.wantSub)
			}
		})
	}
}

func TestControllerUndoZoom(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		zoomed bool
		narrow bool
		want   bool
	}{
		{"user zoom is undone", true, false, true},
		{"nothing to undo unzoomed", false, false, false},
		{"never while narrow", true, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var c Controller
			c.Nav.SetFocus(2)
			c.Zoom.SetZoom(tt.zoomed)
			c.Zoom.SetNarrow(tt.narrow)
			c.Nav.sub[1] = Preview
			if got := c.UndoZoom(); got != tt.want {
				t.Errorf("UndoZoom = %v, want %v", got, tt.want)
			}
			if tt.want && (c.Zoom.User() || c.Nav.Sub(2) != List) {
				t.Errorf("zoom user = %v, sub = %v, want unzoomed and list", c.Zoom.User(), c.Nav.Sub(2))
			}
			if !tt.want && c.Nav.Sub(2) != Preview {
				t.Errorf("a refused undo reset the split focus")
			}
		})
	}
}

func TestControllerSetNarrow(t *testing.T) {
	t.Parallel()
	var c Controller
	c.SetNarrow(true)
	if c.Zoom.CanZoom() {
		t.Error("narrow: can still zoom")
	}
	c.SetNarrow(false)
	if !c.Zoom.CanZoom() {
		t.Error("wide: cannot zoom")
	}
}

func TestControllerSettle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		sub       Role
		width     int
		wantReset bool
	}{
		{"a preview whose box is gone is reset", Preview, 60, true},
		{"a preview still shown keeps the focus", Preview, 120, false},
		{"the list focus is reset", List, 120, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var c Controller
			c.Nav.SetFocus(2)
			c.Nav.sub[1] = tt.sub
			c.SetNarrow(true)
			l := navLayout(tt.width, 2, c.Zoom.Zoomed())
			if got := c.Settle(l); got != tt.wantReset {
				t.Errorf("Settle = %v, want %v", got, tt.wantReset)
			}
			if tt.wantReset && c.Nav.Sub(2) != List {
				t.Errorf("sub = %v, want list after a reset", c.Nav.Sub(2))
			}
			if !tt.wantReset && c.Nav.Sub(2) != Preview {
				t.Errorf("sub = %v, want preview kept", c.Nav.Sub(2))
			}
		})
	}
}

func TestControllerTabUnzoomed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		start Pane
		delta int
		want  Pane
		moved bool
	}{
		{"forward", 1, 1, 2, true},
		{"forward wraps", 3, 1, 1, true},
		{"backward", 2, -1, 1, true},
		{"backward wraps", 1, -1, 3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var c Controller
			c.Nav.SetFocus(tt.start)
			got, moved := c.Tab(navLayout(120, tt.start, false), tt.delta)
			if got != tt.want || moved != tt.moved || c.Nav.Focus() != tt.want {
				t.Errorf("Tab = %d, %v, want %d, %v", got, moved, tt.want, tt.moved)
			}
		})
	}
	t.Run("one pane reports no change", func(t *testing.T) {
		t.Parallel()
		var c Controller
		c.Nav.SetFocus(1)
		l := Layout{Panes: testPanes[:1]}
		if got, moved := c.Tab(l, 1); got != 1 || moved {
			t.Errorf("Tab = %d, %v, want 1, false", got, moved)
		}
	})
}

func TestControllerTabZoomed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		focus   Pane
		width   int
		delta   int
		changed bool
		want    Role
	}{
		{"flips to the preview", 2, 120, 1, true, Preview},
		{"shift also flips to the preview", 2, 120, -1, true, Preview},
		{"no split does nothing", 1, 120, 1, false, List},
		{"narrow split box does nothing", 2, 60, 1, false, List},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var c Controller
			c.Nav.SetFocus(tt.focus)
			got, ok := c.Tab(navLayout(tt.width, tt.focus, true), tt.delta)
			if got != tt.focus || c.Nav.Focus() != tt.focus {
				t.Errorf("focus = %d, want %d", got, tt.focus)
			}
			if ok != tt.changed {
				t.Errorf("changed = %v, want %v", ok, tt.changed)
			}
			if c.Nav.Sub(tt.focus) != tt.want {
				t.Errorf("sub = %v, want %v", c.Nav.Sub(tt.focus), tt.want)
			}
		})
	}
	t.Run("twice returns to the list", func(t *testing.T) {
		t.Parallel()
		var c Controller
		c.Nav.SetFocus(2)
		l := navLayout(120, 2, true)
		c.Tab(l, 1)
		c.Tab(l, 1)
		if c.Nav.Sub(2) != List || c.Nav.Focus() != 2 {
			t.Errorf("sub = %v, focus = %d", c.Nav.Sub(2), c.Nav.Focus())
		}
	})
}

func TestControllerDigit(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		pane   Pane
		zoomed bool
		want   bool
		focus  Pane
	}{
		{"unzoomed", 3, false, true, 3},
		{"zoomed", 3, true, true, 3},
		{"zero is ignored", 0, false, false, 2},
		{"past the last is ignored", 4, true, false, 2},
		{"negative is ignored", -1, false, false, 2},
		{"focused pane", 2, true, true, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var c Controller
			c.Nav.SetFocus(2)
			c.Nav.sub[1] = Preview
			c.Zoom.SetZoom(tt.zoomed)
			l := navLayout(120, 2, tt.zoomed)
			if got := c.Digit(l, tt.pane); got != tt.want {
				t.Errorf("Digit = %v, want %v", got, tt.want)
			}
			if c.Nav.Focus() != tt.focus {
				t.Errorf("focus = %d, want %d", c.Nav.Focus(), tt.focus)
			}
			if c.Zoom.User() != tt.zoomed {
				t.Errorf("user zoom = %v, want %v", c.Zoom.User(), tt.zoomed)
			}
			if tt.want && c.Nav.Sub(2) != List {
				t.Error("a digit left the preview focused")
			}
			if !tt.want && c.Nav.Sub(2) != Preview {
				t.Error("an ignored digit reset the preview")
			}
		})
	}
}
