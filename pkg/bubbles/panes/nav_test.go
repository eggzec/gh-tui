package panes

import "testing"

func navLayout(width int, focus Pane, zoom bool) Layout {
	return Compute(Spec{Width: width, Height: 30, Panes: testPanes, Focus: focus, Zoom: zoom})
}

func TestNavStepWraps(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		start Pane
		delta int
		want  Pane
	}{
		{"forward", 1, 1, 2},
		{"forward wraps", 3, 1, 1},
		{"backward", 2, -1, 1},
		{"backward wraps", 1, -1, 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var n Nav
			n.SetFocus(tt.start)
			n.Step(navLayout(120, tt.start, false), tt.delta)
			if n.Focus() != tt.want {
				t.Errorf("focus = %d, want %d", n.Focus(), tt.want)
			}
		})
	}
}

func TestNavStepResetsSubs(t *testing.T) {
	t.Parallel()
	var n Nav
	n.SetFocus(2)
	zoomed := navLayout(120, 2, true)
	n.Step(zoomed, 1)
	if n.Sub(2) != Preview {
		t.Fatalf("sub = %v, want Preview", n.Sub(2))
	}
	n.Step(navLayout(120, 2, false), 1)
	if n.Focus() != 3 || n.Sub(2) != List {
		t.Errorf("focus = %d, sub(2) = %v; want 3 and List", n.Focus(), n.Sub(2))
	}
}

func TestNavZoomedToggle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		focus Pane
		width int
		want  Role
		can   bool
	}{
		{"with a preview", 2, 120, Preview, true},
		{"without a preview box", 2, 80, List, false},
		{"pane that never splits", 1, 120, List, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var n Nav
			n.SetFocus(tt.focus)
			l := navLayout(tt.width, tt.focus, true)
			if n.CanStep(l) != tt.can {
				t.Errorf("CanStep = %v, want %v", n.CanStep(l), tt.can)
			}
			n.Step(l, 1)
			if n.Focus() != tt.focus || n.Sub(tt.focus) != tt.want {
				t.Errorf("focus = %d, sub = %v; want %d and %v", n.Focus(), n.Sub(tt.focus), tt.focus, tt.want)
			}
			if got := n.InPreview(l); got != (tt.want == Preview) {
				t.Errorf("InPreview = %v", got)
			}
			if tt.can {
				n.Step(l, -1)
				if n.Sub(tt.focus) != List || n.InPreview(l) {
					t.Errorf("second step: sub = %v, want List", n.Sub(tt.focus))
				}
			}
		})
	}
}

func TestNavSetFocusResetsSubs(t *testing.T) {
	t.Parallel()
	var n Nav
	n.SetFocus(2)
	n.Step(navLayout(120, 2, true), 1)
	n.SetFocus(3)
	if n.Sub(2) != List || n.Focus() != 3 {
		t.Errorf("sub(2) = %v, focus = %d", n.Sub(2), n.Focus())
	}
}

func TestNavInPreviewNeedsPreviewBox(t *testing.T) {
	t.Parallel()
	var n Nav
	n.SetFocus(2)
	n.Step(navLayout(120, 2, true), 1)
	if !n.InPreview(navLayout(120, 2, true)) {
		t.Fatal("InPreview false with a preview box")
	}
	if n.InPreview(navLayout(80, 2, true)) {
		t.Error("InPreview true without a preview box")
	}
}
