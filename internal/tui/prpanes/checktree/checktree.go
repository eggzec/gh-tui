// Package checktree turns the checks of one commit into the rows of a
// foldtree: the workflow runs of GitHub Actions with their jobs, grouped as
// the jobs pane of the Actions modal groups them, then one group for each
// other GitHub App, then the plain commit statuses.
//
// The component holds no keys: folding and enter belong to the foldtree that
// draws the rows. What enter on a row asks for leaves as an intent message
// from [Targets], and the component never opens anything itself.
package checktree

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// Mode says which checks of a commit the rows show.
type Mode int

const (
	// All shows every check.
	All Mode = iota
	// Merge shows only the checks that don't pass, which is what stands in the
	// way of a merge, and counts the jobs of a workflow run that don't.
	Merge
)

// Options say where and when the rows of a commit are drawn.
type Options struct {
	// Mode picks the checks to show; the zero value shows all.
	Mode Mode
	// Depth is the depth of the top rows: under a commit row, its depth
	// plus one.
	Depth int
	// Prefix starts the ID of each row, so that the rows of two commits never
	// share one.
	Prefix string
	// Now is the time that the duration of what still runs is told at.
	Now time.Time
}

// Targets maps the ID of a row to the message that enter on it asks for. A
// row with no target does nothing on enter.
type Targets map[string]tea.Msg

// OpenJobMsg asks to open a job of GitHub Actions, of the workflow run RunID
// of the commit SHA.
type OpenJobMsg struct {
	SHA          string
	RunID, JobID int64
}

// OpenCheckMsg asks to open a check run of an App other than GitHub
// Actions, of the commit SHA.
type OpenCheckMsg struct {
	SHA              string
	CheckID, SuiteID int64
}
