package actions

import (
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The jobs pane nests the jobs of a run as [core.GroupJobs] groups them, and
// adds to each group the glyph and the job it opens on.

// jobNode is a job, or a group of jobs, of the jobs pane.
type jobNode struct {
	// job is the index of the job in the items, or -1 for a group.
	job int
	// label is what the row shows: the name, without its group's part.
	label string
	group *jobGroup
}

// jobGroup is the jobs of a caller, or of a matrix.
type jobGroup struct {
	// key names the group across polls and attempts, and tells a caller
	// from a matrix of the same name.
	key  string
	kids []jobNode
	// jobs are the indexes of every job in the group, nested ones too.
	jobs []int
	// state is the state its glyph shows, and first the job it opens on.
	state ui.RunState
	first int
}

// groupJobs returns the jobs of a run as the pane nests them.
func groupJobs(items []core.Job) []jobNode {
	nodes := paneNodes(core.GroupJobs(items))
	for _, n := range nodes {
		summarize(n.group, items)
	}
	return nodes
}

// paneNodes returns nodes as the pane keeps them.
func paneNodes(nodes []core.JobNode) []jobNode {
	out := make([]jobNode, len(nodes))
	for i, n := range nodes {
		out[i] = jobNode{job: n.Job, label: n.Label}
		if g := n.Group; g != nil {
			out[i].group = &jobGroup{key: g.Key, kids: paneNodes(g.Kids), jobs: g.Jobs}
		}
	}
	return out
}

// summarize works out the state and the first job of g and the groups in
// it.
func summarize(g *jobGroup, items []core.Job) {
	if g == nil {
		return
	}
	jobs := make([]core.Job, len(g.jobs))
	for i, j := range g.jobs {
		jobs[i] = items[j]
		if s := ui.RunStateOf(items[j].Status, items[j].Conclusion); i == 0 || stateRank[s] > stateRank[g.state] {
			g.state = s
		}
	}
	g.first = g.jobs[firstJob(jobs)]
	for _, k := range g.kids {
		summarize(k.group, items)
	}
}

// stateRank orders the states for the glyph of a group, which shows its
// worst: what failed, then what was cancelled, then what waits for a
// person, then what hasn't ended, the running before the waiting and the
// queued, and only then what passed or was skipped.
var stateRank = [ui.NumRunStates]int{
	ui.RunSkipped:        0,
	ui.RunNeutral:        1,
	ui.RunSuccess:        2,
	ui.RunQueued:         3,
	ui.RunWaiting:        4,
	ui.RunInProgress:     5,
	ui.RunActionRequired: 6,
	ui.RunCancelled:      7,
	ui.RunTimedOut:       8,
	ui.RunFailure:        9,
}
