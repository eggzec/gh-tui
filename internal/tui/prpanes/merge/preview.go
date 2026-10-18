package merge

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/checktree"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
)

// shortSHA is how many characters of a commit's SHA a preview names it by.
const shortSHA = 7

// Preview is what a row of the pane has to show beside it. Title and Info are
// plain text for the frame to style. Body is empty for a row that has no
// preview yet, and the host then shows a placeholder under the title.
type Preview = checktree.Preview

// PreviewOf returns the preview of the row with the ID id, as [Rows] names it,
// drawn to the width and with the styles of o. ok is false for an ID that no
// row of d has. It does no I/O and reads only d.
func PreviewOf(d Data, id string, o Options) (p Preview, ok bool) {
	st := &o.Styles
	base := text(d.Detail.BaseRef)
	switch {
	case id == idConflict && d.Detail.Merge.Mergeable == core.MergeableConflicting:
		return Preview{
			Title: "Conflicts with " + base,
			Body:  wrap("This branch has conflicts with "+base+". Resolve them on GitHub or locally.", o.Width),
		}, true
	case id == idBehind && d.Detail.Merge.Status == core.MergeBehind:
		return behindPreview(d, base, o), true
	case id == idDraft && d.Detail.Draft:
		return Preview{
			Title: "Draft",
			Body:  wrap("The pull request is a draft; drafts cannot be merged.", o.Width) + "\n\n" + wrap("enter marks it ready for review (asks y/n)", o.Width),
		}, true
	case id == idReview && d.Detail.ReviewDecision == core.ReviewRequired:
		return Preview{Title: "Review required"}, true
	case strings.HasPrefix(id, idChanges):
		for i, v := range changers(d.Detail.Reviewers) {
			if id == idChanges+v.Author.Login || (v.Author.Login == "" && id == idChanges+strconv.Itoa(i)) {
				return Preview{Title: "Changes requested: " + conv.Name(v.Author)}, true
			}
		}
		return Preview{}, false
	}
	if !d.ChecksLoaded {
		return Preview{}, false
	}
	co := checktree.Options{Mode: checktree.Merge, Now: o.Now, Width: o.Width}
	return checktree.PreviewOf(d.Checks, id, co, st.Checks)
}

// behindPreview explains why a branch that is behind its base is held, and
// lists the commits of the base that it lacks.
func behindPreview(d Data, base string, o Options) Preview {
	st := &o.Styles
	p := Preview{Title: "Behind " + base}
	switch n := d.Behind; {
	case n == 1:
		p.Info = "1 commit"
	case n > 1:
		p.Info = strconv.Itoa(n) + " commits"
	}
	out := []string{
		wrap("GitHub can merge "+base+" into the branch, so that the checks run on the code as the merge would leave it.", o.Width),
	}
	if len(d.BaseCommits.Commits) > 0 {
		out = append(out, "")
		for l := range strings.SplitSeq(wrap(base+" has, since the branch started:", o.Width), "\n") {
			out = append(out, st.dim.Render(l))
		}
	}
	for i := range d.BaseCommits.Commits {
		c := &d.BaseCommits.Commits[i]
		sha := c.SHA[:min(len(c.SHA), shortSHA)]
		out = append(out, cut(st.accent.Render(sha)+" "+text(c.Subject), o))
	}
	if more := d.BaseCommits.AheadBy - len(d.BaseCommits.Commits); more > 0 && len(d.BaseCommits.Commits) > 0 {
		out = append(out, st.dim.Render("and "+strconv.Itoa(more)+" more"))
	}
	hint := " updates the branch with " + base
	asks := "(asks y/n)"
	if k := o.Keys.Update; k != "" {
		asks = "(asks y/n; " + text(k) + " does the same)"
	}
	out = append(out, "", wrap(st.bold.Render("enter")+hint+" "+st.dim.Render(asks), o.Width))
	p.Body = strings.Join(out, "\n")
	return p
}

// wrap wraps s to width, which is zero for no limit.
func wrap(s string, width int) string {
	if width <= 0 {
		return s
	}
	return ansi.Wrap(s, width, "")
}
