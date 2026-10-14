package pulls

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

// The review threads in the diff of the Files tab. Every thread is a block
// under the line it is on, under the line it was on if that is on the
// deleted side, and under the header of its file if its code has changed
// since or it is on the whole file. A block starts folded: a resolved thread
// is one line, an unresolved one its first comment and how many answered it.
// The fold key of the diff opens it to every comment.

// threadCount is how many threads a file has, and how many of them are not
// resolved.
type threadCount struct{ n, open int }

// threadAnchor returns where in the diff thread th sits.
func threadAnchor(th *core.Thread) diff.Pos {
	if th.Outdated || th.File || th.Line < 1 {
		return diff.Pos{Path: th.Path}
	}
	side := diff.NewSide
	if th.Side == core.SideLeft {
		side = diff.OldSide
	}
	return diff.Pos{Path: th.Path, Side: side, Line: th.Line}
}

// threadBlocks returns a block for every thread read that has a file.
func (m *detailModal) threadBlocks() []diff.Block {
	all := m.th.all
	blocks := make([]diff.Block, 0, len(all))
	for i := range all {
		th := &all[i]
		if th.Path == "" {
			continue
		}
		blocks = append(blocks, diff.Block{
			ID:     th.ID,
			Anchor: threadAnchor(th),
			Folded: true,
			Render: func(w int, folded bool) []string { return m.threadRows(th, w, folded) },
		})
	}
	return append(blocks, m.draftBlocks()...)
}

// applyThreadBlocks gives the diff the threads read, and the tree the number
// of threads of each file. It is a no-op before the Files tab has started.
func (m *detailModal) applyThreadBlocks() {
	f := m.files
	if f == nil {
		return
	}
	f.counts = map[string]threadCount{}
	for i := range m.th.all {
		th := &m.th.all[i]
		if th.Path == "" {
			continue
		}
		c := f.counts[th.Path]
		c.n++
		if !th.Resolved {
			c.open++
		}
		f.counts[th.Path] = c
	}
	f.diff.SetBlocks(m.threadBlocks())
	f.tree.SetIcons(f.icon)
}

// threadByID returns the thread read with the given id.
func (m *detailModal) threadByID(id string) (*core.Thread, bool) {
	for i := range m.th.all {
		if m.th.all[i].ID == id {
			return &m.th.all[i], true
		}
	}
	return nil, false
}

// threadRows renders the lines of thread th in w cells.
func (m *detailModal) threadRows(th *core.Thread, w int, folded bool) []string {
	if folded {
		return m.threadFolded(th, w)
	}
	return m.threadOpen(th, w)
}

// threadTags returns the notes after a thread: that it is resolved, and that
// its code has changed since, which a thread on a whole file never has.
func (m *detailModal) threadTags(th *core.Thread) string {
	var tags string
	if th.Resolved {
		tags += " " + m.st.age.Render("resolved")
	}
	if th.Outdated && !th.File {
		tags += " " + m.st.age.Render(outdatedTag)
	}
	return tags
}

// threadMark is the dot a thread starts with: the warning colour while it is
// unresolved, and dim once it is resolved. An ellipsis stands for it while a
// change of it is under way.
func (m *detailModal) threadMark(th *core.Thread) string {
	if m.th.pending[th.ID] {
		return m.theme.Warning.Render(m.icons.Ellipsis)
	}
	if th.Resolved {
		return m.theme.Subtle.Render(m.icons.Dot)
	}
	return m.theme.Warning.Render(m.icons.Dot)
}

// threadFolded renders thread th as one line, and for an unresolved thread
// with answers a second one that counts them.
func (m *detailModal) threadFolded(th *core.Thread, w int) []string {
	first := firstComment(th)
	head := " " + m.threadMark(th) + " "
	line := head
	if first != nil {
		body := ui.OneLine(first.Body)
		if th.Resolved {
			line += m.st.age.Render(m.threadWho(first, false)) + "  " + m.st.age.Render(body)
		} else {
			line += m.threadWho(first, true) + "  " + m.st.title.Render(body)
		}
	}
	tags := m.threadTags(th)
	room := max(w-ansi.StringWidth(tags), 1)
	lines := []string{ansi.Truncate(line, room, m.icons.Ellipsis) + tags}
	if n := max(th.TotalComments, len(th.Comments)) - 1; n > 0 && !th.Resolved {
		word := " replies"
		if n == 1 {
			word = " reply"
		}
		lines = append(lines, ansi.Truncate(strings.Repeat(" ", ansi.StringWidth(head))+m.st.age.Render(strconv.Itoa(n)+word), w, ""))
	}
	return lines
}

// fitTags truncates line, not its tags, to leave them whole within w columns.
// The tags go only when less than a few columns of line remain beside them,
// which is at very narrow widths; lead is the width of the line's fixed start.
func (m *detailModal) fitTags(line, tags string, w, lead int) string {
	tw := ansi.StringWidth(tags)
	room := w - tw
	if room < lead+2 {
		return ansi.Truncate(line+tags, w, m.icons.Ellipsis)
	}
	return ansi.Truncate(line, room, m.icons.Ellipsis) + tags
}

// threadOpen renders every comment of thread th: who wrote it and when, and
// the text wrapped under that.
func (m *detailModal) threadOpen(th *core.Thread, w int) []string {
	head := " " + m.threadMark(th) + " "
	indent := strings.Repeat(" ", ansi.StringWidth(head))
	var lines []string
	for i := range th.Comments {
		c := &th.Comments[i]
		lead := indent
		if i == 0 {
			lead = head
		}
		line := lead + m.threadWho(c, true)
		if i == 0 {
			line = m.fitTags(line, m.threadTags(th), w, ansi.StringWidth(lead))
		} else {
			line = ansi.Truncate(line, w, m.icons.Ellipsis)
		}
		lines = append(lines, line)
		for raw := range strings.SplitSeq(c.Body, "\n") {
			for _, l := range ui.Wrap(ui.OneLine(raw), max(w-len(indent), 1)) {
				lines = append(lines, indent+m.st.title.Render(strings.TrimRight(l, " ")))
			}
		}
	}
	if more := th.TotalComments - len(th.Comments); more > 0 || th.MoreComments {
		text := "More comments on GitHub"
		if more > 0 {
			text = strconv.Itoa(more) + " more on GitHub"
		}
		lines = append(lines, indent+m.st.age.Render(text))
	}
	if len(lines) == 0 {
		lines = []string{head + m.st.age.Render("No comments")}
	}
	return lines
}

// threadMarkOf renders the mark after a file's name in the tree: the number of
// its threads, red while one is unresolved and dim after.
func (f *filesState) threadMarkOf(path, dot string) string {
	c, ok := f.counts[path]
	if !ok || c.n == 0 {
		return ""
	}
	st := f.theme.Subtle
	if c.open > 0 {
		st = f.theme.Error
	}
	return st.Render(dot + strconv.Itoa(c.n))
}

// threadUnderCursor returns the thread the diff's cursor is on.
func (m *detailModal) threadUnderCursor() (*core.Thread, bool) {
	f := m.files
	if f == nil || f.focus != diffPane {
		return nil, false
	}
	b, ok := f.diff.CursorBlock()
	if !ok {
		return nil, false
	}
	return m.threadByID(b.ID)
}

// openThread opens the thread under the cursor of the diff on GitHub, at its
// first comment. It reports whether it did.
func (m *detailModal) openThread() (tea.Cmd, bool) {
	th, ok := m.threadUnderCursor()
	if !ok {
		return nil, false
	}
	c := firstComment(th)
	if c == nil || c.URL == "" {
		return nil, false
	}
	return ui.Open(c.URL), true
}
