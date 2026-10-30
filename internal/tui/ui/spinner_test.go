package ui

import (
	"testing"

	"charm.land/bubbles/v2/spinner"
)

func TestSpinnerLifecycle(t *testing.T) {
	t.Parallel()
	s := NewSpinner(spinner.Dot)
	if s.Start() == nil {
		t.Fatal("first Start returned no tick")
	}
	if s.Start() != nil {
		t.Fatal("Start while running returned a tick")
	}
	tick := s.Tick().(spinner.TickMsg)
	if s.Update(tick, true) == nil {
		t.Fatal("no next tick while loading")
	}
	if s.Update(tick, false) != nil {
		t.Fatal("tick continued when not loading")
	}
	if s.Start() == nil {
		t.Fatal("Start after the spinner stopped returned no tick")
	}
	s.Stop()
	if s.Start() == nil {
		t.Fatal("Start after Stop returned no tick")
	}
}
