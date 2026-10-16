package checktree

import (
	"strconv"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
)

// ChecksNode returns the row that heads the checks of c in the merge pane: how
// many fail, and how many still run, with the rows of [Rows] in [Merge] mode
// under it. It returns the zero row when nothing fails or runs, and then the
// caller shows no node.
func ChecksNode(c core.CommitChecks, o Options, st Styles) foldtree.Row {
	var failing, running int
	count := func(s core.CheckState) {
		switch s {
		case core.CheckFailing:
			failing++
		case core.CheckPending:
			running++
		case core.CheckNone, core.CheckPassing:
		}
	}
	for i := range c.Runs {
		count(c.Runs[i].Outcome())
	}
	for i := range c.Statuses {
		count(c.Statuses[i].Outcome())
	}
	b := &builder{c: c, o: o, st: &st}
	var text string
	switch {
	case failing > 0:
		text = b.line(st.icon(core.CheckFailing), "Checks failing") + b.dim(" "+strconv.Itoa(failing))
		if running > 0 {
			text += b.dim(st.Glyphs.Separator + strconv.Itoa(running) + " running")
		}
	case running > 0:
		text = b.line(st.icon(core.CheckPending), "Checks running") + b.dim(" "+strconv.Itoa(running))
	default:
		return foldtree.Row{}
	}
	return b.fold(o.Prefix+"checks", o.Depth, text, "")
}
