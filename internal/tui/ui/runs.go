package ui

import (
	"time"

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

// RunStyles style what shows runs, jobs and steps: their text, and the
// glyphs of their states. Build them once per theme with [NewRunStyles].
type RunStyles struct {
	Text, Strong, Muted, Subtle lipgloss.Style
	Accent, Warning, Error      lipgloss.Style
	// States style the glyphs of the run states, and Glyphs holds them
	// rendered.
	States [NumRunStates]lipgloss.Style
	Glyphs [NumRunStates]string
}

// NewRunStyles returns the run styles of t, with the glyphs of ic.
func NewRunStyles(t Theme, ic Icons) RunStyles {
	s := RunStyles{
		Text: t.Text, Strong: t.Title, Muted: t.Muted, Subtle: t.Subtle,
		Accent: t.Accent, Warning: t.Warning, Error: t.Error,
	}
	for st := range NumRunStates {
		s.States[st] = t.Run(st)
		s.Glyphs[st] = s.States[st].Render(ic.Run(st))
	}
	return s
}

// Took renders how long a job or a step ran, or where it is while it
// hasn't started.
func (s *RunStyles) Took(status core.RunStatus, c core.Conclusion, start, end, now time.Time) string {
	switch {
	case c == core.ConclusionSkipped:
		return s.Subtle.Render("skipped")
	case status == core.RunCompleted:
		if d, ok := Span(start, end, now); ok {
			return s.Subtle.Render(Duration(d))
		}
		return ""
	case status == core.RunInProgress:
		if d, ok := Span(start, time.Time{}, now); ok {
			return s.States[RunInProgress].Render(Duration(d))
		}
		return s.States[RunInProgress].Render("running")
	case status == core.RunCancelling:
		return s.Warning.Render("cancelling")
	}
	return s.Muted.Render(string(status))
}
