package ui

import (
	tea "charm.land/bubbletea/v2"

	"charm.land/bubbles/v2/spinner"
)

// Spinner is a spinner that ticks only while something loads, and whose
// tick is started once however many reads ask for it.
type Spinner struct {
	spinner.Model
	running bool
}

// NewSpinner returns a Spinner that draws sp.
func NewSpinner(sp spinner.Spinner) Spinner {
	return Spinner{Model: spinner.New(spinner.WithSpinner(sp))}
}

// Start returns the first tick, unless the spinner already ticks.
func (s *Spinner) Start() tea.Cmd {
	if s.running {
		return nil
	}
	s.running = true
	return s.Tick
}

// Stop forgets that the spinner ticks, so that Start begins it again.
func (s *Spinner) Stop() { s.running = false }

// Update moves the spinner on and returns the next tick while loading is
// true. Otherwise it stops and returns nil.
func (s *Spinner) Update(msg spinner.TickMsg, loading bool) tea.Cmd {
	if !loading {
		s.running = false
		return nil
	}
	var cmd tea.Cmd
	s.Model, cmd = s.Model.Update(msg)
	return cmd
}
