package actions

import (
	"strconv"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// runSpan is how long the latest attempt of r ran, or has run so far.
func runSpan(r core.Run, now time.Time) (time.Duration, bool) {
	start := r.RunStartedAt
	if start.IsZero() {
		start = r.CreatedAt
	}
	var end time.Time
	if r.Done() {
		end = r.UpdatedAt
	}
	return ui.Span(start, end, now)
}

// runName names a run in prose, such as "CI #4812".
func runName(r core.Run) string {
	name := ui.OneLine(r.Name)
	if name == "" {
		name = "Run"
	}
	return name + " #" + strconv.Itoa(r.Number)
}

// failedJobs counts the jobs that a re-run of the failed jobs starts
// again: those that failed or were cancelled.
func failedJobs(jobs []core.Job) int {
	n := 0
	for i := range jobs {
		if c := jobs[i].Conclusion; c.Failed() || c == core.ConclusionCancelled {
			n++
		}
	}
	return n
}
