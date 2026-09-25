package jobview

import (
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// RunName names a run in prose, such as "CI #4812".
func RunName(r core.Run) string {
	name := ui.OneLine(r.Name)
	if name == "" {
		name = "Run"
	}
	return name + " #" + strconv.Itoa(r.Number)
}

// FailedJobs counts the jobs that a re-run of the failed jobs starts
// again: those that failed or were cancelled.
func FailedJobs(jobs []core.Job) int {
	n := 0
	for i := range jobs {
		if c := jobs[i].Conclusion; c.Failed() || c == core.ConclusionCancelled {
			n++
		}
	}
	return n
}
