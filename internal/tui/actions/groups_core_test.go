package actions

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// The pane nests the jobs as core does, so the check tree of a pull request
// shows the same groups.
func TestGroupJobsAreCores(t *testing.T) {
	t.Parallel()
	items := duckJobs(1)
	var same func(pane []jobNode, shared []core.JobNode)
	same = func(pane []jobNode, shared []core.JobNode) {
		t.Helper()
		if len(pane) != len(shared) {
			t.Fatalf("pane has %d nodes, core %d", len(pane), len(shared))
		}
		for i, n := range pane {
			c := shared[i]
			if n.job != c.Job || n.label != c.Label || (n.group == nil) != (c.Group == nil) {
				t.Fatalf("node %d is %d %q, core's %d %q", i, n.job, n.label, c.Job, c.Label)
			}
			if n.group == nil {
				continue
			}
			if n.group.key != c.Group.Key || !slices.Equal(n.group.jobs, c.Group.Jobs) {
				t.Errorf("group %q has key %q and jobs %v, core's %q and %v", n.label, n.group.key, n.group.jobs, c.Group.Key, c.Group.Jobs)
			}
			same(n.group.kids, c.Group.Kids)
		}
	}
	same(groupJobs(items), core.GroupJobs(items))
}
