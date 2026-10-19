package pulls

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
)

const (
	// threadPrefix starts the ID of the block of a review thread in the diff.
	threadPrefix = "thread:"
	// threadsFailedID is the ID of the line that says the threads were not read.
	threadsFailedID = "note:threads"
)

// threadsState is how far the read of the review threads of a diff is.
type threadsState uint8

const (
	threadsUnread threadsState = iota
	threadsLoading
	threadsReady
	threadsFailed
)

// diffThreadsMsg is the answer to the read of the review threads of a diff.
type diffThreadsMsg struct {
	o       *diffOverlay
	threads []core.Thread
	err     error
}

// isThread says whether b is the block of a review thread.
func isThread(b diff.Block) bool { return strings.HasPrefix(b.ID, threadPrefix) }

// isNote says whether b is the block of a draft or a review thread, which the
// keys that move between them stop on.
func isNote(b diff.Block) bool { return isDraft(b) || isThread(b) }

// threadQuery asks for the page of review threads at cursor.
func (m *panesModal) threadQuery(cursor string) pulls.ThreadsQuery {
	return pulls.ThreadsQuery{Repo: m.repo, Number: m.number, Head: m.detail.HeadSHA, Cursor: cursor}
}

// cachedThreads returns every thread when all its pages are cached.
func (o *diffOverlay) cachedThreads() ([]core.Thread, bool) {
	var all []core.Thread
	seen := map[string]bool{}
	cursor := ""
	for {
		page, ok := o.m.svc.CachedThreads(o.m.threadQuery(cursor))
		if !ok {
			return nil, false
		}
		all = append(all, page.Threads...)
		if page.Last() || seen[page.Next] {
			return all, true
		}
		seen[page.Next] = true
		cursor = page.Next
	}
}

// loadThreads puts the review threads of the pull request in the diff, once:
// at once when they are cached, else by a read that settles, failed or not.
func (o *diffOverlay) loadThreads() tea.Cmd {
	if o.threadState != threadsUnread || o.m.detail.HeadSHA == "" {
		return nil
	}
	if all, ok := o.cachedThreads(); ok {
		o.threads, o.threadState = all, threadsReady
		return nil
	}
	o.threadState = threadsLoading
	m, ctx := o.m, o.ctx
	return func() tea.Msg {
		var all []core.Thread
		seen := map[string]bool{}
		cursor := ""
		for {
			page, err := m.svc.Threads(ctx, m.threadQuery(cursor))
			if err != nil {
				return diffThreadsMsg{o: o, err: err}
			}
			all = append(all, page.Threads...)
			if page.Last() || seen[page.Next] {
				return diffThreadsMsg{o: o, threads: all}
			}
			seen[page.Next] = true
			cursor = page.Next
		}
	}
}

// receiveThreads keeps the threads that a read brought, and shows them.
func (o *diffOverlay) receiveThreads(msg diffThreadsMsg) {
	if o.done || o.ctx.Err() != nil {
		return
	}
	if msg.err != nil {
		o.threadState = threadsFailed
	} else {
		o.threads, o.threadState = msg.threads, threadsReady
	}
	o.setBlocks()
}

// threadShown says whether the diff shows thread t: one that is outdated
// belongs to the commit it was made on.
func threadShown(t *core.Thread) bool { return !t.Outdated }

// threadsHere says whether the diff is of the whole pull request, which the
// lines of the threads belong to. The diff of a range of commits has other
// lines, so it shows no thread.
func (o *diffOverlay) threadsHere() bool {
	_, _, all, ok := o.m.rangeEnds()
	return all || !ok
}

// markThreads works out the lines that the threads are on, when the diff is
// the one they belong to.
func (o *diffOverlay) markThreads() {
	o.marks = map[diff.Pos]struct{}{}
	if !o.threadsHere() {
		return
	}
	for i := range o.threads {
		t := &o.threads[i]
		if !threadShown(t) || t.File || t.Line <= 0 {
			continue
		}
		side := threadSide(t.Side)
		first := t.Line
		if t.StartLine > 0 && t.StartLine < t.Line {
			first = t.StartLine
		}
		for line := first; line <= t.Line; line++ {
			o.marks[diff.Pos{Path: t.Path, Side: side, Line: line}] = struct{}{}
		}
	}
}

// threadMark is the mark of a line in the gutter: the accent bar of a line a
// thread is on.
func (o *diffOverlay) threadMark(p diff.Pos) string {
	if _, ok := o.marks[p]; !ok {
		return ""
	}
	return o.m.theme.Accent.Render("▎")
}

// threadSide is the side of the diff a thread on side s is on.
func threadSide(s core.Side) diff.Side {
	if s == core.SideLeft {
		return diff.OldSide
	}
	return diff.NewSide
}

// threadBlocks returns one block for each thread the diff shows, and the line
// that says the read failed. A diff of a range of commits has neither.
func (o *diffOverlay) threadBlocks() []diff.Block {
	if !o.threadsHere() {
		return nil
	}
	var blocks []diff.Block
	if o.threadState == threadsFailed {
		blocks = append(blocks, o.threadsFailedBlock())
	}
	for i := range o.threads {
		t := &o.threads[i]
		if threadShown(t) && len(t.Comments) > 0 {
			blocks = append(blocks, o.threadBlock(t))
		}
	}
	return blocks
}

// threadsFailedBlock is the dim line on top of the diff that says the threads
// could not be read.
func (o *diffOverlay) threadsFailedBlock() diff.Block {
	m := o.m
	return diff.Block{
		ID:     threadsFailedID,
		Anchor: diff.Pos{Path: o.path},
		Render: func(width int, _ bool) []string {
			return []string{ansi.Truncate(m.theme.Subtle.Render("review threads could not be read"), width, m.icons.Ellipsis)}
		},
	}
}

// threadBlock is the rows that show a review thread: who wrote it and when,
// then the text of each comment.
func (o *diffOverlay) threadBlock(t *core.Thread) diff.Block {
	m := o.m
	anchor := diff.Pos{Path: t.Path}
	if !t.File && t.Line > 0 {
		anchor = diff.Pos{Path: t.Path, Side: threadSide(t.Side), Line: t.Line}
	}
	now := time.Now
	if m.now != nil {
		now = m.now
	}
	comments := t.Comments
	resolved := t.Resolved
	return diff.Block{
		ID:     threadPrefix + t.ID,
		Anchor: anchor,
		Render: func(width int, _ bool) []string {
			var rows []string
			add := func(s string) { rows = append(rows, ansi.Truncate(s, width, m.icons.Ellipsis)) }
			bar := m.theme.Accent.Render("┃ ")
			at := now()
			for i := range comments {
				c := &comments[i]
				head := m.theme.Accent.Render(c.Author.Login) + " " + m.theme.Subtle.Render(ui.Ago(c.At, at))
				if i == 0 && resolved {
					head += " " + m.theme.Subtle.Render("resolved")
				}
				add(bar + head)
				body := strings.ReplaceAll(strings.TrimSpace(c.Body), "\r", "")
				for line := range strings.SplitSeq(strings.ReplaceAll(body, "\t", "    "), "\n") {
					add(bar + m.theme.Text.Render(line))
				}
			}
			return rows
		},
	}
}
