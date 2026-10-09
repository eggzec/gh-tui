package pulls

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// mergeKind is what the merge key does to a pull request: the one thing
// that suits what its detail says, since the key doesn't ask which.
type mergeKind int

const (
	// mergeNow merges it at once.
	mergeNow mergeKind = iota + 1
	// mergeAuto turns on auto-merge: GitHub merges it once its checks pass.
	mergeAuto
	// mergeQueue adds it to the merge queue of its base branch.
	mergeQueue
	// mergeStop turns auto-merge off, which is already on.
	mergeStop
)

// mergePlan is what the merge key does to a pull request. Kind is unset
// when it can't merge, and Cant says why, as a sentence.
type mergePlan struct {
	Kind mergeKind
	// Review is set when auto-merge waits for a review as well as for the
	// checks.
	Review bool
	Cant   string
}

// planMerge decides what the merge key does to d. What the detail hasn't
// told yet is left to GitHub, as it would anyway: the pull request merges.
func planMerge(d core.PullRequestDetail) mergePlan {
	m, n := d.Merge, "#"+strconv.Itoa(d.Number)
	switch {
	case m.AutoMerge != nil && !m.CanDisableAutoMerge:
		return mergePlan{Cant: "You can't turn off auto-merge for " + n + "."}
	case m.AutoMerge != nil:
		return mergePlan{Kind: mergeStop}
	case m.Queue.Queued:
		return mergePlan{Cant: n + " is already in the merge queue."}
	case !decided(m):
		// A plain merge goes past the rules for an administrator, so it is
		// only asked for a pull request that GitHub says may merge.
		return mergePlan{Cant: "GitHub is still checking whether " + n + " can merge; try again in a moment."}
	}
	hard, review, pending := unmet(d)
	switch {
	case len(hard) == 0 && !review && !pending:
		if m.Queue.Enabled {
			return mergePlan{Kind: mergeQueue}
		}
		return mergePlan{Kind: mergeNow}
	case len(hard) == 0 && pending && m.CanAutoMerge && m.RepoAutoMerge:
		return mergePlan{Kind: mergeAuto, Review: review}
	}
	if review {
		hard = append(hard, "a review is required")
	}
	if pending {
		hard = append(hard, "checks are still running")
	}
	return mergePlan{Cant: "Can't merge: " + strings.Join(hard, ", ") + "."}
}

// decided reports whether m says whether the pull request may merge: its
// status is one that the merge key knows what to do with, or GitHub says
// it conflicts. A read that didn't say, or that GitHub is still working
// out, or a status this doesn't know, doesn't.
func decided(m core.MergeInfo) bool {
	switch m.Status {
	case core.MergeClean, core.MergeUnstable, core.MergeHasHooks,
		core.MergeBlocked, core.MergeBehind, core.MergeDirty, core.MergeDraft:
		return true
	case core.MergeUnknown:
		// GitHub is still working it out.
	}
	return m.Mergeable == core.MergeableConflicting
}

// unmet returns what keeps d from merging now, from what GitHub says of
// its state: the reasons that waiting won't lift, whether a review is
// still to come, and whether checks are still running. A pull request that
// GitHub calls blocked is held by a rule the detail names, or else by a
// rule it can't name.
func unmet(d core.PullRequestDetail) (hard []string, review, pending bool) {
	m := d.Merge
	if m.Mergeable == core.MergeableConflicting || m.Status == core.MergeDirty {
		hard = append(hard, "it has conflicts with "+baseName(d.PullRequest))
	}
	switch m.Status {
	case core.MergeBehind:
		hard = append(hard, "its branch is behind "+baseName(d.PullRequest))
	case core.MergeDraft:
		hard = append(hard, "it is a draft")
	case core.MergeBlocked:
		before := len(hard)
		if d.ReviewDecision == core.ReviewChangesRequested {
			hard = append(hard, "changes were requested")
		}
		review = d.ReviewDecision == core.ReviewRequired
		failing, waiting := requiredChecks(d)
		pending = waiting
		if failing != "" {
			hard = append(hard, failing)
		}
		if len(hard) == before && !review && !pending {
			hard = append(hard, "branch protection rules aren't met")
		}
	case core.MergeClean, core.MergeDirty, core.MergeHasHooks, core.MergeUnstable, core.MergeUnknown:
		// Conflicts are said above; the rest may merge.
	}
	return hard, review, pending
}

// requiredChecks says how the checks that the rules require fare: what
// fails, as a reason to give, and whether any is still running. Without
// the read of the required checks, whether complete or at all, it goes by
// the checks as a whole.
func requiredChecks(d core.PullRequestDetail) (failing string, pending bool) {
	m := d.Merge
	if c := m.RequiredChecks; c.Total() > 0 && !d.ChecksTruncated {
		switch c.Failed {
		case 0:
		case 1:
			failing = "1 required check is failing"
		default:
			failing = strconv.Itoa(c.Failed) + " required checks are failing"
		}
		return failing, c.Pending > 0
	}
	if m.Rules.Known && len(m.Rules.Checks) == 0 {
		// No check is required, so none of them blocks.
		return "", false
	}
	if d.Checks == core.ChecksFailure {
		failing = "checks are failing"
	}
	return failing, d.Checks == core.ChecksPending
}

// baseName names the branch pr merges into, or says "its base".
func baseName(pr core.PullRequest) string {
	if pr.BaseRef == "" {
		return "its base"
	}
	return pr.BaseRef
}

// mergeChange returns the change that the merge key makes to d: what
// planMerge decides, with method, or else one the repository allows. of
// says what the change is so far.
func (k keyMap) mergeChange(svc Service, g ui.Gate, method core.MergeMethod, d core.PullRequestDetail, of changeOf) (c change, ok bool, warn tea.Cmd) {
	pr := d.PullRequest
	n := "#" + strconv.Itoa(pr.Number)
	if pr.Draft {
		return change{}, false, ui.Notify(toast.Warning, "Mark "+n+" ready for review before merging it.")
	}
	plan := planMerge(d)
	if plan.Cant != "" {
		return change{}, false, ui.Notify(toast.Warning, plan.Cant)
	}
	repo, number, head := g.Repo, pr.Number, pr.HeadSHA
	start := of
	caps := capsFor(g.Caps, d.Merge)
	m, _ := caps.MergeMethod(method)
	// A merge queue merges as it is set up, whatever the method.
	chooses := plan.Kind == mergeNow || plan.Kind == mergeAuto && !d.Merge.Queue.Enabled
	of.kind, of.base, of.head = plan.Kind, pr.BaseRef, head
	if chooses {
		of.method = m
	}
	c = change{of: of}
	switch plan.Kind {
	case mergeNow:
		c.question = mergeQuestion(pr, m)
		c.what = "merge " + n
		c.start = func() *optimistic.Op { return svc.Merge(repo, number, m, head) }
	case mergeAuto:
		c.question = autoQuestion(pr, m, d.Merge.Queue.Enabled, plan.Review)
		c.what = "merge " + n + " when its checks pass"
		c.start = func() *optimistic.Op { return svc.AutoMerge(repo, number, m, head) }
	case mergeQueue:
		c.question = "Add " + n + " to the merge queue" + forBase(pr) + "?"
		c.what = "add " + n + " to the merge queue"
		c.start = func() *optimistic.Op { return svc.Enqueue(repo, number, head) }
	case mergeStop:
		c.question = "Turn off auto-merge for " + n + "?"
		c.what = "turn off auto-merge for " + n
		c.start = func() *optimistic.Op { return svc.StopAutoMerge(repo, number) }
	}
	if methods := caps.MergeMethods(); chooses && len(methods) > 1 {
		following := methods[(slices.Index(methods, m)+1)%len(methods)]
		c.next = func() change {
			next, _, _ := k.mergeChange(svc, g, following, d, start)
			return next
		}
	}
	return c, true, nil
}

// capsFor returns caps with the merge methods that the detail says the
// repository allows, which are as current as the detail, when it says.
func capsFor(caps core.RepoCaps, m core.MergeInfo) core.RepoCaps {
	if len(m.Methods) == 0 {
		return caps
	}
	caps.Known = true
	caps.MergeCommit, caps.Squash, caps.Rebase = m.Allows(core.MergeCommit), m.Allows(core.MergeSquash), m.Allows(core.MergeRebase)
	return caps
}

// forBase says " for main", or nothing without a base.
func forBase(pr core.PullRequest) string {
	if pr.BaseRef == "" {
		return ""
	}
	return " for " + pr.BaseRef
}

// mergeQuestion asks to merge pr with method, such as "Squash-merge #79
// into main?". The screen shows the repository, so the question leaves
// it out, and it leads with the method, which a cut would lose.
func mergeQuestion(pr core.PullRequest, method core.MergeMethod) string {
	n := "#" + strconv.Itoa(pr.Number)
	var into string
	if pr.BaseRef != "" {
		into = " into " + pr.BaseRef
	}
	var q string
	switch method {
	case core.MergeSquash:
		q = "Squash-merge " + n + into
	case core.MergeRebase:
		q = "Rebase-merge " + n + into
	case core.MergeCommit:
		q = "Merge " + n + into + " with a merge commit"
	default:
		q = "Merge " + n + into
	}
	return q + "?"
}

// autoQuestion asks to merge pr with method once its checks pass, such as
// "Merge #79 into main when checks pass (squash)?", or "when it can be
// merged" if a review is awaited too. A merge queue merges as it is set
// up, so the question leaves the method out.
func autoQuestion(pr core.PullRequest, method core.MergeMethod, queued, review bool) string {
	q := "Merge #" + strconv.Itoa(pr.Number)
	if pr.BaseRef != "" {
		q += " into " + pr.BaseRef
	}
	q += " when checks pass"
	if review {
		q = strings.TrimSuffix(q, "checks pass") + "it can be merged"
	}
	if queued {
		return q + "?"
	}
	name := string(method)
	if method == core.MergeCommit {
		name = "merge commit"
	}
	return q + " (" + name + ")?"
}
