// Package files holds the rows of the Files pane of the pull request modal:
// the commits of the pull request, with the range of them the pane shows the
// files of.
package files

import (
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
)

// Hint says why a change of the range did or did not do what the user
// pressed for. It is empty when the change needs no word. [Hint.Text] gives
// its words.
type Hint string

// The hints of [Range.Mark] and of the commits since the last review.
const (
	// HintOne is given when the only selected commit is marked.
	HintOne Hint = "one"
	// HintContiguous is given when a commit inside the range is marked.
	HintContiguous Hint = "contiguous"
	// HintNotACommit is given when the user marks a row that is no commit.
	HintNotACommit Hint = "not a commit"
	// HintNoneSince is given when no commit came after the last review.
	HintNoneSince Hint = "none since"
	// HintGone is given when the reviewed commit is gone from GitHub.
	HintGone Hint = "gone"
)

// Keys names the keys that the hints point to, as the user has bound them.
// A key that has no name is left out of the words, and a hint that is only
// about that key has none.
type Keys struct {
	// Mark selects a commit or marks a file, and SelectAll selects every commit.
	Mark, SelectAll string
}

// Text is the words of the hint, naming the keys of k. It is empty when the
// hint has nothing to say without a key that is unbound.
func (h Hint) Text(k Keys) string {
	switch h {
	case HintOne:
		text := "at least one commit stays selected"
		if k.SelectAll != "" {
			text += " · " + k.SelectAll + " selects all"
		}
		return text
	case HintContiguous:
		return "a range has to be contiguous: only this commit is selected now"
	case HintNotACommit:
		if k.Mark == "" {
			return ""
		}
		return k.Mark + " selects a commit or marks a file viewed"
	case HintNoneSince:
		return "no commits since your last review"
	case HintGone:
		return "your reviewed commit is gone: selected the commits made after your review"
	}
	return ""
}

// Range is the contiguous run of commits, as indexes into the commits oldest
// first, whose files the pane shows. Both ends are included.
type Range struct {
	First, Last int
}

// All is the range of all n commits.
func All(n int) Range { return Range{First: 0, Last: n - 1} }

// Contains reports whether commit i is in the range.
func (r Range) Contains(i int) bool { return i >= r.First && i <= r.Last }

// Len is how many commits the range holds.
func (r Range) Len() int {
	if r.Last < r.First {
		return 0
	}
	return r.Last - r.First + 1
}

// Mark returns the range after the user marks commit i of n commits, and a
// hint when the change is not what the key says. A range always holds at
// least one commit and is contiguous.
func (r Range) Mark(i, n int) (Range, Hint) {
	if i < 0 || i >= n {
		return r, ""
	}
	switch {
	case !r.Contains(i):
		return Range{First: min(r.First, i), Last: max(r.Last, i)}, ""
	case r.Len() == 1:
		return r, HintOne
	case i == r.First:
		return Range{First: r.First + 1, Last: r.Last}, ""
	case i == r.Last:
		return Range{First: r.First, Last: r.Last - 1}, ""
	default:
		return Range{First: i, Last: i}, HintContiguous
	}
}

// Since is the range of the commits that came after the viewer's last review,
// running to the head so that it stays contiguous. It is false when there is
// no review, when the review is on the head, and when no commit is dated after
// a review whose commit is gone.
func Since(pc core.PullCommits) (Range, bool) {
	first, ok := sinceFirst(pc)
	if !ok {
		return Range{}, false
	}
	return Range{First: first, Last: len(pc.Commits) - 1}, true
}

// Gone reports whether the commit the last review was made on is no longer
// listed, so that [Since] and [Divider] go by the dates of the commits.
func Gone(pc core.PullCommits) bool {
	return core.SinceLastReview(pc).Reason == core.SinceApproximate
}

// Divider is the index of the commit the "your last review" row sits above:
// the first commit after the reviewed one. It is false when the row is not
// shown.
func Divider(pc core.PullCommits) (before int, ok bool) {
	return sinceFirst(pc)
}

// sinceFirst is the index of the first commit after the last review.
func sinceFirst(pc core.PullCommits) (int, bool) {
	res := core.SinceLastReview(pc)
	if len(res.Commits) == 0 {
		return 0, false
	}
	oid := res.Commits[0].OID
	for i, c := range pc.Commits {
		if strings.EqualFold(c.OID, oid) {
			return i, true
		}
	}
	return 0, false
}
