// Package merge turns what stands in the way of merging a pull request into
// the rows of the merge pane of the pull request modal, and into the lines
// above and below them: the branch line and the actions line.
//
// The package does no I/O and holds no keys. The checks of the head commit,
// the count of commits behind and the method a merge would use arrive in
// [Data], and the key labels of the hints in [Options]. What enter on a row
// asks for leaves as an intent message from [Model], and the host decides
// what to do with it.
package merge

import (
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// shortBelow is the width, in columns, below which the actions line drops the
// method and the long names of its hints.
const shortBelow = 54

// ruleWidth is the width of the rule above the actions line.
const ruleWidth = 5

// IDs of the rows, which stay the same across refreshes so that the cursor
// and the folds follow them. The checks node is "checks", as
// [checktree.ChecksNode] names it, and each reviewer that asked for changes
// has "changes:" and their login, or their place among them without one.
const (
	idReview   = "review-required"
	idChanges  = "changes:"
	idConflict = "conflicts"
	idBehind   = "behind"
	idDraft    = "draft"
)

// Data is what the pane shows.
type Data struct {
	// Detail is the pull request with its merge state and reviewers.
	Detail core.PullRequestDetail
	// Checks are the checks of the head commit, and ChecksLoaded says they
	// were read: until then, and where no reader is set, the checks show no
	// row.
	Checks       core.CommitChecks
	ChecksLoaded bool
	// Behind is how many commits the head is behind the base, or -1 while it
	// is not known.
	Behind int
	// BaseCommits are the commits of the base that the head lacks, which the
	// preview of the behind row lists. They are empty until they are read.
	BaseCommits core.BaseCommits
	// Method is the merge method a merge starts on, such as "squash", or empty
	// when a merge has no method to name.
	Method string
}

// Options say how the rows and lines are drawn.
type Options struct {
	// Width is the width of a line in cells. A line that is longer is cut with
	// an ellipsis, never wrapped. Zero means no limit.
	Width  int
	Styles Styles
	// Now is the time that the duration of the checks that still run is told
	// at.
	Now time.Time
	// Keys label the hints of the actions line.
	Keys Hints
}

// Hints are the labels of the keys of the actions line. A hint whose key is
// empty, since the user unbound it, is left out.
type Hints struct {
	Merge, Update string
}

// FocusReviewMsg asks to focus the conversation on the latest review of
// Login.
type FocusReviewMsg struct{ Login string }

// UpdateBranchMsg asks to update the branch with its base.
type UpdateBranchMsg struct{}

// MarkReadyMsg asks to mark the draft as ready for review.
type MarkReadyMsg struct{}

// RequestReviewMsg asks to pick the reviewers to request.
type RequestReviewMsg struct{}

// Count returns the number of blockers: the checks count as one, and each
// other blocker, each reviewer that asked for changes included, as one.
func Count(d Data) int {
	n := len(changers(d.Detail.Reviewers))
	if checksBlock(d) {
		n++
	}
	if d.Detail.ReviewDecision == core.ReviewRequired {
		n++
	}
	if d.Detail.Merge.Mergeable == core.MergeableConflicting {
		n++
	}
	if d.Detail.Merge.Status == core.MergeBehind {
		n++
	}
	if d.Detail.Draft {
		n++
	}
	return n
}

// Rows returns the rows of the blockers in their fixed order, which are open
// folds where they fold, and what enter on each of them asks for. There are no
// rows when nothing blocks. The rows share nothing with d.
func Rows(d Data, o Options) ([]foldtree.Row, checktree.Targets) {
	st := &o.Styles
	targets := checktree.Targets{}
	var rows []foldtree.Row

	if checksBlock(d) {
		co := checktree.Options{Mode: checktree.Merge, Now: o.Now, Width: o.Width}
		rows = append(rows, checktree.ChecksNode(d.Checks, co, st.Checks))
		co.Depth = 1
		kids, kt := checktree.Rows(d.Checks, co, st.Checks)
		rows = append(rows, kids...)
		maps.Copy(targets, kt)
	}

	base := text(d.Detail.BaseRef)
	if d.Detail.ReviewDecision == core.ReviewRequired {
		line := "Review required"
		if names := who(d.Detail.Reviewers.Requested); names != "" {
			line += ": " + names
		}
		rows = append(rows, st.row(idReview, ui.GlyphRequested, line))
		targets[idReview] = RequestReviewMsg{}
	}
	for i, v := range changers(d.Detail.Reviewers) {
		// A deleted account has no login, so its row is told by its place.
		id := idChanges + v.Author.Login
		if v.Author.Login == "" {
			id = idChanges + strconv.Itoa(i)
		}
		rows = append(rows, st.row(id, ui.GlyphChanges, "Changes requested: "+conv.Name(v.Author)))
		targets[id] = FocusReviewMsg{Login: v.Author.Login}
	}
	if d.Detail.Merge.Mergeable == core.MergeableConflicting {
		rows = append(rows, st.row(idConflict, ui.GlyphBlocked, "Conflicts with "+base))
	}
	if d.Detail.Merge.Status == core.MergeBehind {
		rows = append(rows, st.row(idBehind, ui.GlyphBlocked, behind(d.Behind, base)))
		targets[idBehind] = UpdateBranchMsg{}
	}
	if d.Detail.Draft {
		rows = append(rows, st.row(idDraft, ui.GlyphBlocked, "Draft"))
		targets[idDraft] = MarkReadyMsg{}
	}
	return rows, targets
}

// Branch returns the first line of the pane: the head, an arrow and the base.
func Branch(d Data, o Options) string {
	st := &o.Styles
	s := st.bold.Render(text(d.Detail.HeadRef)) + st.arrow + st.bold.Render(text(d.Detail.BaseRef))
	return cut(s, o)
}

// Rule returns the dim rule between the rows and the actions line.
func Rule(o Options) string {
	n := ruleWidth
	if o.Width > 0 {
		n = min(n, o.Width)
	}
	return o.Styles.dim.Render(strings.Repeat(o.Styles.top, n))
}

// Actions returns the actions line, such as "Merge (squash) M · Update branch
// U". Below 54 columns it is "Merge M · Update U". A hint whose key is unbound
// is left out, and the line is empty when both are.
func Actions(d Data, o Options) string {
	st := &o.Styles
	short := o.Width > 0 && o.Width < shortBelow
	var hints []string
	if k := o.Keys.Merge; k != "" {
		s := "Merge"
		if d.Method != "" && !short {
			s += " (" + text(d.Method) + ")"
		}
		hints = append(hints, s+" "+st.dim.Render(text(k)))
	}
	if k := o.Keys.Update; k != "" {
		s := "Update branch "
		if short {
			s = "Update "
		}
		hints = append(hints, s+st.dim.Render(text(k)))
	}
	return cut(strings.Join(hints, st.sep), o)
}

// checksBlock reports whether the checks stand in the way: they were read, and
// one fails or still runs.
func checksBlock(d Data) bool {
	if !d.ChecksLoaded {
		return false
	}
	s := checktree.StateOf(d.Checks)
	return s == core.CheckFailing || s == core.CheckPending
}

// changers returns the reviews, one for each reviewer, whose verdict asks for
// changes, in the order of the reviews.
func changers(r core.Reviewers) []core.Verdict {
	var out []core.Verdict
	for _, v := range r.Verdicts {
		if v.State == core.ReviewStateChangesRequested {
			out = append(out, v)
		}
	}
	return out
}

// who names the reviewers that a review is still required of: the code owners
// among those asked, else all of those asked. It is empty when nobody is left.
func who(asked []core.ReviewRequest) string {
	pick := func(owners bool) []string {
		var names []string
		for _, r := range asked {
			if r.Unknown || (owners && !r.CodeOwner) {
				continue
			}
			if n := reviewer(r); n != "" {
				names = append(names, n)
			}
		}
		return names
	}
	names := pick(true)
	if len(names) == 0 {
		names = pick(false)
	}
	return strings.Join(names, ", ")
}

// reviewer names a person by their login and a team by its name, or by its
// slug without the organization when it has none.
func reviewer(r core.ReviewRequest) string {
	if r.Team == "" && r.TeamName == "" {
		return text(r.User.Login)
	}
	if r.TeamName != "" {
		return text(r.TeamName)
	}
	_, slug, ok := strings.Cut(r.Team, "/")
	if !ok {
		slug = r.Team
	}
	return text(slug)
}

// behind says how far the head is behind base: "behind" alone while the count
// is not known.
func behind(n int, base string) string {
	switch {
	case n == 1:
		return "Branch is 1 commit behind " + base
	case n > 1:
		return "Branch is " + strconv.Itoa(n) + " commits behind " + base
	}
	return "Branch is behind " + base
}

// text cleans text from outside for a single line.
func text(s string) string { return strings.TrimSpace(termtext.OneLine(s)) }

// cut cuts s to the width of o with an ellipsis.
func cut(s string, o Options) string {
	if o.Width <= 0 || ansi.StringWidth(s) <= o.Width {
		return s
	}
	return termtext.Truncate(s, o.Width, o.Styles.ellipsis)
}
