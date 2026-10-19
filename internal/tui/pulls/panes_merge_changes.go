package pulls

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
)

// changesMissing is said when the review is not among the pages read so far.
const changesMissing = "This review is not in the conversation read so far. enter opens the Conversation."

// changesBody draws the review of a Changes requested row as the Conversation
// shows it: the latest review of that reviewer that asked for changes, and the
// threads that belong to it. It reads what the Conversation holds and starts no
// read. final is false while the conversation is not read, so that the body is
// drawn again once it is.
func (m *panesModal) changesBody(v core.Verdict, width int) (body string, final bool) {
	c := &m.conv
	if !c.started || !c.model.Ready() {
		return m.theme.Subtle.Render("Reading the conversation…"), false
	}
	login := v.Author.Login
	id, ok := c.model.LatestReviewBy(login, core.ReviewStateChangesRequested)
	if !ok {
		if c.model.Status().Loading {
			return m.theme.Subtle.Render("Reading the conversation…"), false
		}
		return m.dimLines(changesMissing, width), true
	}
	n := conv.Narrow{
		Item:   func(it core.TimelineItem) bool { return it.Kind == core.TimelineReview && it.ID == id },
		Thread: func(t core.Thread) bool { return t.ReviewID == id },
	}
	view, ok := c.model.Peek("changes:"+id, n, width)
	if !ok {
		return m.dimLines(changesMissing, width), true
	}
	return view, true
}

// dimLines wraps s to width and draws it dim.
func (m *panesModal) dimLines(s string, width int) string {
	if width > 0 {
		s = ansi.Wrap(s, width, "")
	}
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = m.theme.Subtle.Render(l)
	}
	return strings.Join(lines, "\n")
}
