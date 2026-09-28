package actions

import (
	"regexp"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The jobs pane groups the jobs of a run by the names GitHub gives them,
// since GitHub tells nothing of which jobs need which: the jobs of a
// reusable workflow are named "caller / job", and those of a matrix
// "base (values)". Jobs group on their first caller, whose group shows the
// rest of their names, and then on their matrix base, at the top and
// inside a caller. So the pane nests two levels at most, and a deeper
// "a / b / c (x)" shows as x under "b / c" under a, or under "a / b" when
// every job of a goes on through b. A job whose own name holds " / " is
// taken for a caller's, as nothing tells them apart.

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

const callerSep = " / "

// groupJobs returns the jobs of a run as the pane nests them. A group of
// one job is that job alone.
func groupJobs(items []core.Job) []jobNode {
	nodes := make([]jobNode, len(items))
	for i := range items {
		nodes[i] = jobNode{job: i, label: jobLabel(items[i].Name)}
	}
	nodes = clump(nodes, "", " /", callerOf)
	for i, n := range nodes {
		if g := n.group; g != nil {
			nodes[i].label += shared(g.kids)
			g.kids = clump(g.kids, g.key+" ", " (", matrixOf(g.kids))
		}
	}
	nodes = clump(nodes, "", " (", matrixOf(nodes))
	for _, n := range nodes {
		summarize(n.group, items)
	}
	return nodes
}

// split tells the group of a job labelled label, and its label in there.
type split func(label string) (group, rest string, ok bool)

// clump gathers the jobs of nodes that by puts in the same group, in a
// group at the place of the first. Groups already in nodes stay as they
// are. A group's key is prefix, its name and sep.
func clump(nodes []jobNode, prefix, sep string, by split) []jobNode {
	out := make([]jobNode, 0, len(nodes))
	at := map[string]int{}
	whole := map[string]string{}
	for _, n := range nodes {
		if n.group != nil {
			out = append(out, n)
			continue
		}
		name, rest, ok := by(n.label)
		if !ok {
			out = append(out, n)
			continue
		}
		key := prefix + name + sep
		i, seen := at[key]
		if !seen {
			i = len(out)
			at[key], whole[key] = i, n.label
			out = append(out, jobNode{job: -1, label: name, group: &jobGroup{key: key}})
		}
		g := out[i].group
		g.kids = append(g.kids, jobNode{job: n.job, label: rest})
		g.jobs = append(g.jobs, n.job)
	}
	for i, n := range out {
		if g := n.group; g != nil && len(g.jobs) == 1 {
			out[i] = jobNode{job: g.jobs[0], label: whole[g.key]}
		}
	}
	return out
}

// shared drops the callers every job of kids shares from their labels,
// as a reusable workflow that calls another, "Build / Linux (x)" and
// "Build / MacOS (y)", and returns them as the group's name goes on.
func shared(kids []jobNode) string {
	var drop string
	for {
		caller, _, ok := callerOf(kids[0].label)
		for _, k := range kids[1:] {
			c, _, isCaller := callerOf(k.label)
			ok = ok && isCaller && c == caller
		}
		if !ok {
			return drop
		}
		for i := range kids {
			kids[i].label = kids[i].label[len(caller)+len(callerSep):]
		}
		drop += callerSep + caller
	}
}

// callerOf splits the label of a job of a reusable workflow at its caller.
func callerOf(label string) (caller, rest string, ok bool) {
	caller, rest, ok = strings.Cut(label, callerSep)
	return caller, rest, ok && caller != "" && rest != ""
}

// matrixOf returns what splits the labels of jobs of nodes at their matrix
// base. A job named just the base of others, as GitHub names a matrix job
// it skipped before it expanded the matrix, joins them.
func matrixOf(nodes []jobNode) split {
	bases := map[string]bool{}
	for _, n := range nodes {
		if b, _, ok := matrixBase(n.label); ok && n.group == nil {
			bases[b] = true
		}
	}
	return func(label string) (string, string, bool) {
		if b, values, ok := matrixBase(label); ok {
			return b, values, true
		}
		return label, label, bases[label]
	}
}

// matrixBase splits the label "base (values)" of a matrix job. GitHub cuts
// long names with "...", which can take the closing parenthesis, and a
// base can hold parentheses of its own, as "Tests (2nd batch) (linux)".
func matrixBase(label string) (base, values string, ok bool) {
	var open int
	switch {
	case strings.HasSuffix(label, ")"):
		open = openParen(label)
		values = label[open+1 : len(label)-1]
	case strings.HasSuffix(label, "..."):
		open = strings.LastIndex(label, " (") + 1
		values = label[open+1:]
	default:
		return "", "", false
	}
	if open < 2 || label[open-1] != ' ' || values == "" {
		return "", "", false
	}
	return label[:open-1], values, true
}

// openParen returns where the parenthesis that closes label opens, or -1.
func openParen(label string) int {
	depth := 0
	for i := len(label) - 1; i >= 0; i-- {
		switch label[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
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

// matrixExpr is an expression GitHub left in the name of a matrix job it
// skipped before it expanded the matrix.
var matrixExpr = regexp.MustCompile(`\$\{\{\s*(?:matrix\.)?([^}]*?)\s*\}\}`)

// jobLabel is the name of a job as the pane shows it: on one line, with
// an expression GitHub left in it shown as the name of what it stands for.
func jobLabel(name string) string {
	name = ui.OneLine(name)
	if !strings.Contains(name, "${{") {
		return name
	}
	return matrixExpr.ReplaceAllString(name, "{$1}")
}
