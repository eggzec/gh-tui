package core

import (
	"regexp"
	"strings"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// CheckState is how a check run, a job, a commit status or a set of them
// stands, as the summary of the checks of a pull request counts it. The
// states are ordered by how much they matter: [CheckFailing] is the worst.
type CheckState int

// Check states. CheckNone is the state of no checks at all, which shows no
// icon.
const (
	CheckNone CheckState = iota
	CheckPassing
	CheckPending
	CheckFailing
)

// CheckStateOf returns the state of a GitHub conclusion, run status or
// commit status, in lower case as GitHub writes it. A failure, a timeout, a
// failure to start, a wait for approval and a cancel fail; what is queued,
// running or waiting is pending; success, neutral and skipped pass. A commit
// status of error or failure fails, one of pending is pending and one of
// success passes. A value not known, or an empty one, is pending: a state
// we can't read never counts as a pass.
func CheckStateOf(s string) CheckState {
	switch s {
	case "failure", "timed_out", "startup_failure", "action_required", "cancelled", "stale", "error":
		return CheckFailing
	case "success", "neutral", "skipped":
		return CheckPassing
	}
	return CheckPending
}

// Worst returns the worst of states: failing beats pending, which beats
// passing. No states, or only none, give [CheckNone].
func Worst(states ...CheckState) CheckState {
	w := CheckNone
	for _, s := range states {
		w = max(w, s)
	}
	return w
}

// Outcome is how j stands: pending until it completed, and then as its
// conclusion says, see [CheckStateOf].
func (j Job) Outcome() CheckState {
	if j.Status != RunCompleted {
		return CheckPending
	}
	return CheckStateOf(string(j.Conclusion))
}

// The jobs of a run are grouped by the names GitHub gives them, since GitHub
// tells nothing of which jobs need which: the jobs of a reusable workflow
// are named "caller / job", and those of a matrix "base (values)". Jobs
// group on their first caller, whose group shows the rest of their names,
// and then on their matrix base, at the top and inside a caller. So the
// nodes nest two levels at most, and a deeper "a / b / c (x)" shows as x
// under "b / c" under a, or under "a / b" when every job of a goes on
// through b. A job whose own name holds " / " is taken for a caller's, as
// nothing tells them apart.

// JobNode is a job, or a group of jobs, of a run.
type JobNode struct {
	// Job is the index of the job in the jobs grouped, or -1 for a group.
	Job int
	// Label is what a row shows: the name, without its group's part.
	Label string
	// Group is set for a group.
	Group *JobGroup
}

// JobGroup is the jobs of a caller, or of a matrix.
type JobGroup struct {
	// Key names the group across polls and attempts, and tells a caller
	// from a matrix of the same name.
	Key  string
	Kids []JobNode
	// Jobs are the indexes of every job in the group, nested ones too.
	Jobs []int
	// State is the worst state of the jobs of the group.
	State CheckState
}

const callerSep = " / "

// GroupJobs returns the jobs of a run nested in their groups, as their names
// say. It groups the jobs it is given and reads nothing more. A group of
// one job is that job alone.
func GroupJobs(jobs []Job) []JobNode {
	nodes := make([]JobNode, len(jobs))
	for i := range jobs {
		nodes[i] = JobNode{Job: i, Label: jobLabel(jobs[i].Name)}
	}
	nodes = clump(nodes, "", " /", callerOf)
	for i, n := range nodes {
		if g := n.Group; g != nil {
			nodes[i].Label += shared(g.Kids)
			g.Kids = clump(g.Kids, g.Key+" ", " (", matrixOf(g.Kids))
		}
	}
	nodes = clump(nodes, "", " (", matrixOf(nodes))
	for _, n := range nodes {
		rollup(n.Group, jobs)
	}
	return nodes
}

// split tells the group of a job labelled label, and its label in there.
type split func(label string) (group, rest string, ok bool)

// clump gathers the jobs of nodes that by puts in the same group, in a
// group at the place of the first. Groups already in nodes stay as they
// are. A group's key is prefix, its name and sep.
func clump(nodes []JobNode, prefix, sep string, by split) []JobNode {
	out := make([]JobNode, 0, len(nodes))
	at := map[string]int{}
	whole := map[string]string{}
	for _, n := range nodes {
		if n.Group != nil {
			out = append(out, n)
			continue
		}
		name, rest, ok := by(n.Label)
		if !ok {
			out = append(out, n)
			continue
		}
		key := prefix + name + sep
		i, seen := at[key]
		if !seen {
			i = len(out)
			at[key], whole[key] = i, n.Label
			out = append(out, JobNode{Job: -1, Label: name, Group: &JobGroup{Key: key}})
		}
		g := out[i].Group
		g.Kids = append(g.Kids, JobNode{Job: n.Job, Label: rest})
		g.Jobs = append(g.Jobs, n.Job)
	}
	for i, n := range out {
		if g := n.Group; g != nil && len(g.Jobs) == 1 {
			out[i] = JobNode{Job: g.Jobs[0], Label: whole[g.Key]}
		}
	}
	return out
}

// shared drops the callers every job of kids shares from their labels,
// as a reusable workflow that calls another, "Build / Linux (x)" and
// "Build / MacOS (y)", and returns them as the group's name goes on.
func shared(kids []JobNode) string {
	var drop string
	for {
		caller, _, ok := callerOf(kids[0].Label)
		for _, k := range kids[1:] {
			c, _, isCaller := callerOf(k.Label)
			ok = ok && isCaller && c == caller
		}
		if !ok {
			return drop
		}
		for i := range kids {
			kids[i].Label = kids[i].Label[len(caller)+len(callerSep):]
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
func matrixOf(nodes []JobNode) split {
	bases := map[string]bool{}
	for _, n := range nodes {
		if b, _, ok := matrixBase(n.Label); ok && n.Group == nil {
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

// rollup works out the state of g and of the groups in it.
func rollup(g *JobGroup, jobs []Job) {
	if g == nil {
		return
	}
	states := make([]CheckState, len(g.Jobs))
	for i, j := range g.Jobs {
		states[i] = jobs[j].Outcome()
	}
	g.State = Worst(states...)
	for _, k := range g.Kids {
		rollup(k.Group, jobs)
	}
}

// matrixExpr is an expression GitHub left in the name of a matrix job it
// skipped before it expanded the matrix.
var matrixExpr = regexp.MustCompile(`\$\{\{\s*(?:matrix\.)?([^}]*?)\s*\}\}`)

// jobLabel is the name of a job as a row shows it: on one line, with an
// expression GitHub left in it shown as the name of what it stands for.
func jobLabel(name string) string {
	name = termtext.OneLine(name)
	if !strings.Contains(name, "${{") {
		return name
	}
	return matrixExpr.ReplaceAllString(name, "{$1}")
}
