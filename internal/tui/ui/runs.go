package ui

import (
	"charm.land/lipgloss/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

// RunState is the state of a workflow run, a job or a step as a glyph
// shows it.
type RunState int

// Run states. A run waiting to start, for a runner or an approval, is
// queued, and one being cancelled shows as cancelled already.
const (
	RunQueued RunState = iota
	RunInProgress
	RunSuccess
	RunFailure
	RunCancelled
	RunSkipped
	RunTimedOut
	RunActionRequired
	RunNeutral
	// NumRunStates counts the run states, for arrays indexed by them.
	NumRunStates
)

// runGlyphs returns the glyphs of the run states of the icon set named set.
func runGlyphs(set string) [NumRunStates]string {
	switch set {
	case config.IconsUnicode:
		return [NumRunStates]string{
			RunQueued: "○", RunInProgress: "◐", RunSuccess: "✓", RunFailure: "✗", RunCancelled: "⊘",
			RunSkipped: "⊖", RunTimedOut: "⧗", RunActionRequired: "!", RunNeutral: "◇",
		}
	case config.IconsASCII:
		return [NumRunStates]string{
			RunQueued: ".", RunInProgress: "~", RunSuccess: "+", RunFailure: "x", RunCancelled: "/",
			RunSkipped: ">", RunTimedOut: "T", RunActionRequired: "!", RunNeutral: "=",
		}
	default:
		// The octicons GitHub marks checks with.
		return [NumRunStates]string{
			RunQueued: "", RunInProgress: "", RunSuccess: "", RunFailure: "", RunCancelled: "",
			RunSkipped: "", RunTimedOut: "", RunActionRequired: "", RunNeutral: "",
		}
	}
}

// Run returns the glyph of run state s.
func (ic Icons) Run(s RunState) string {
	if s < 0 || s >= NumRunStates {
		s = RunNeutral
	}
	return ic.runs[s]
}

// RunStateOf returns the state of a run, job or step with status and
// conclusion.
func RunStateOf(status core.RunStatus, c core.Conclusion) RunState {
	switch status {
	case core.RunCompleted:
	case core.RunInProgress:
		return RunInProgress
	case core.RunCancelling:
		return RunCancelled
	default:
		return RunQueued
	}
	switch c {
	case core.ConclusionSuccess:
		return RunSuccess
	case core.ConclusionFailure, core.ConclusionStartupFailure:
		return RunFailure
	case core.ConclusionCancelled:
		return RunCancelled
	case core.ConclusionSkipped:
		return RunSkipped
	case core.ConclusionTimedOut:
		return RunTimedOut
	case core.ConclusionActionRequired:
		return RunActionRequired
	default:
		return RunNeutral
	}
}

// The colors GitHub gives the run states, on light and on dark
// backgrounds: green for success, red for what failed, amber for what is
// on its way or waits for someone, and grey for the rest.
var runColors = [NumRunStates][2]string{
	RunQueued:         {"#9a6700", "#d29922"},
	RunInProgress:     {"#9a6700", "#d29922"},
	RunSuccess:        {"#1a7f37", "#3fb950"},
	RunFailure:        {"#cf222e", "#f85149"},
	RunCancelled:      {"#59636e", "#9198a1"},
	RunSkipped:        {"#59636e", "#9198a1"},
	RunTimedOut:       {"#cf222e", "#f85149"},
	RunActionRequired: {"#9a6700", "#d29922"},
	RunNeutral:        {"#59636e", "#9198a1"},
}

// Run returns the style of the glyph of run state s.
func (t Theme) Run(s RunState) lipgloss.Style {
	if s < 0 || s >= NumRunStates {
		s = RunNeutral
	}
	c := runColors[s][0]
	if t.Dark {
		c = runColors[s][1]
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c))
}
