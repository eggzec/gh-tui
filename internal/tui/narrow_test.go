package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
)

// narrowFake records the last SetNarrow it was told.
type narrowFake struct {
	*fakeSection
	narrow bool
}

func (f *narrowFake) SetNarrow(narrow bool) { f.narrow = narrow }

// :set ui.wide.width lays the dashboard out again at once, with no resize.
func TestSetWideWidthRelaysOut(t *testing.T) {
	t.Parallel()
	dash := &narrowFake{fakeSection: &fakeSection{title: "Dashboard"}}
	layout := Layout{Dashboard: dash, Notifications: &fakeSection{title: "Notifications"}}
	m := New(t.Context(), config.Default(), layout)
	m.toast.SetDuration(0)
	m.Update(tea.WindowSizeMsg{Width: 90, Height: 30})
	if !dash.narrow {
		t.Fatal("dashboard at 90x30 with the default size is not narrow")
	}
	m.setCommand("ui.wide.width=80")
	if dash.narrow {
		t.Error("dashboard is still narrow after ui.wide.width=80 with no resize")
	}
}

// ui.wide moves the boundary the root decides narrow at, from the size of the
// whole terminal.
func TestWideConfigOverrideMovesBoundary(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name          string
		wide          config.Wide
		width, height int
		narrow        bool
	}{
		{"default, 99 wide", config.Wide{Width: 100, Height: 30}, 99, 30, true},
		{"default, 100 low", config.Wide{Width: 100, Height: 30}, 100, 29, true},
		{"default, 100x30", config.Wide{Width: 100, Height: 30}, 100, 30, false},
		{"narrower setting, 80x30", config.Wide{Width: 80, Height: 30}, 80, 30, false},
		{"narrower setting, 79x30", config.Wide{Width: 80, Height: 30}, 79, 30, true},
		{"lower setting, 100x20", config.Wide{Width: 100, Height: 20}, 100, 20, false},
	} {
		m := &Model{width: tt.width, height: tt.height, wide: tt.wide}
		if got := m.narrow(); got != tt.narrow {
			t.Errorf("%s: narrow() = %v, want %v", tt.name, got, tt.narrow)
		}
	}
}
