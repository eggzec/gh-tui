package pulls

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The unresolved review threads that start the Conversation tab: one row each
// for where it is, who started it and the last reply, and a last row that
// reads the next page while there are more threads than were read. The first
// page is read when the tab first shows, and again when the modal reads the
// pull request again.

const (
	// moreRowID is the id of the row that reads the next page.
	moreRowID = "more"
	// outdatedTag marks a thread whose code has changed since.
	outdatedTag = "outdated"
	// rowPrefix is the width of the cursor, the glyph and the spaces
	// around them, which the rows start with.
	rowPrefix = 4
)

// threadsMsg carries a page of the review threads of a pull request to the
// modal that asked for it.
type threadsMsg struct {
	thread int64
	// gen is the read of the first page the page belongs to, so that what a
	// read that was replaced brings is dropped.
	gen int
	// more reports that it is a page after the first.
	more bool
	page core.ThreadPage
	err  error
}

// threadsState is the state of the unresolved threads of the Conversation
// tab.
type threadsState struct {
	// started is set once a read was started, and head is the head of the
	// pull request the threads are of. dirty says that what was read is out
	// of date and is read again when the tab shows.
	started bool
	head    string
	dirty   bool
	// got says that a first page came in, so that an empty list isn't taken
	// for a read that is still under way.
	got bool
	// jumping is the direction of a jump to the diff that waits for the first
	// page of the threads, or 0.
	jumping int
	// gen counts the reads of the first page.
	gen int
	// all are the threads read, resolved or not, in the order GitHub lists
	// them, and list those of them that are unresolved. read is how many
	// threads the pages held, of total.
	all, list   []core.Thread
	read, total int
	// next is the cursor of the page after those read, empty after the last.
	next string
	// loadingMore is set while the next page is read, and moreErr is why
	// the last read of one failed.
	loadingMore bool
	moreErr     error
	// err is why the first page couldn't be read.
	err error
	// kept reports that the first page was served for want of an answer from
	// GitHub, or a rate limit, so that it is read again once it answers.
	kept bool
	// cursor is the row that has the cursor, and rowID its id, which the
	// cursor follows when the rows change.
	cursor int
	rowID  string
	// rows are the lines of the first line of each row in the header of the
	// thread, as it was last rendered.
	rows []int
	// pending are the ids of the threads whose change is under way.
	pending map[string]bool
}

// rowCount is the number of rows the cursor moves through: a row for each
// thread, and one that reads more if there are more.
func (t *threadsState) rowCount() int {
	n := len(t.list)
	if t.next != "" {
		n++
	}
	return n
}

// rowIDAt returns the id of row i.
func (t *threadsState) rowIDAt(i int) string {
	if i < len(t.list) {
		return t.list[i].ID
	}
	return moreRowID
}

// cursorRow returns the row that has the cursor: the row it was last on, or
// the place where it was if that row is gone.
func (t *threadsState) cursorRow() int {
	n := t.rowCount()
	if t.rowID != "" {
		for i := range n {
			if t.rowIDAt(i) == t.rowID {
				return i
			}
		}
	}
	return min(max(t.cursor, 0), max(n-1, 0))
}

// moveTo puts the cursor on row i.
func (t *threadsState) moveTo(i int) {
	t.cursor = i
	t.rowID = ""
	if i >= 0 && i < t.rowCount() {
		t.rowID = t.rowIDAt(i)
	}
}

// unresolved returns the threads of ts that aren't resolved.
func unresolved(ts []core.Thread) []core.Thread {
	var out []core.Thread
	for i := range ts {
		if !ts[i].Resolved {
			out = append(out, ts[i])
		}
	}
	return out
}

// onConversation reports whether the Conversation tab shows.
func (m *detailModal) onConversation() bool { return m.tab == conversationTab }

// showsThreads reports whether a tab that shows the threads is shown: the
// Conversation tab lists the unresolved ones, and the Files tab puts every
// one in the diff.
func (m *detailModal) showsThreads() bool { return m.onConversation() || m.onFiles() }

// readableThreads reports whether the threads can be read: they are of a
// head, which the detail says.
func (m *detailModal) readableThreads() bool {
	return m.loaded && m.detail.HeadSHA != ""
}

// threadsQuery selects the page of the threads at the cursor.
func (m *detailModal) threadsQuery(cursor string) pulls.ThreadsQuery {
	return pulls.ThreadsQuery{Repo: m.repo, Number: m.number, Head: m.detail.HeadSHA, Cursor: cursor}
}

// showThreads reads the threads when the Conversation or the Files tab shows,
// once the detail has said what the head is, unless it has read them already
// for that head. Nothing is read while another tab shows. The diff of the
// Files tab holds every thread, so there the pages after the first are read
// too.
func (m *detailModal) showThreads() tea.Cmd {
	t := &m.th
	if !m.showsThreads() || !m.readableThreads() {
		return nil
	}
	if t.started && !t.dirty && t.err == nil && t.head == m.detail.HeadSHA {
		if m.onFiles() {
			return m.readMoreThreads()
		}
		return nil
	}
	return m.readThreads()
}

// reloadThreads reads the threads again, as the modal reads the pull request
// again: at once while a tab that shows them is shown, and when one shows next
// otherwise.
func (m *detailModal) reloadThreads() tea.Cmd {
	if !m.th.started {
		return nil
	}
	if !m.showsThreads() {
		m.th.dirty = true
		return nil
	}
	if !m.readableThreads() {
		return nil
	}
	return m.readThreads()
}

// readThreads reads the first page of the threads. What is cached shows at
// once, and the read goes on behind it. The pages after the first are
// dropped, since the read may have changed which threads they hold.
func (m *detailModal) readThreads() tea.Cmd {
	t := &m.th
	moved := t.head != m.detail.HeadSHA
	t.pin()
	t.gen++
	t.started, t.dirty = true, false
	t.head = m.detail.HeadSHA
	t.err, t.moreErr, t.loadingMore = nil, nil, false
	if p, ok := m.svc.CachedThreads(m.threadsQuery("")); ok {
		t.setFirst(p)
	} else if moved {
		// What was read is of a head the pull request left.
		t.all, t.list, t.read, t.total, t.next, t.kept, t.got = nil, nil, 0, 0, "", false, false
	}
	m.applyThreadBlocks()
	return tea.Batch(m.fetchThreads("", t.gen), m.show())
}

// readMoreThreads reads the page after those read, unless that is under
// way.
func (m *detailModal) readMoreThreads() tea.Cmd {
	t := &m.th
	if t.next == "" || t.loadingMore {
		return nil
	}
	t.loadingMore, t.moreErr = true, nil
	return tea.Batch(m.fetchThreads(t.next, t.gen), m.show())
}

// fetchThreads reads the page of the threads at cursor.
func (m *detailModal) fetchThreads(cursor string, gen int) tea.Cmd {
	ctx, svc, q, id := m.ctx, m.svc, m.threadsQuery(cursor), m.thread.ID()
	return func() tea.Msg {
		ctx, end := obs.Begin(ctx, "pull.threads")
		p, err := svc.Threads(ctx, q)
		end(err, "span", "tui", "repo", q.Repo.String(), "number", q.Number, "first", cursor == "", "threads", len(p.Threads), "offline", p.Offline, "limited", p.Limited)
		return threadsMsg{thread: id, gen: gen, more: cursor != "", page: p, err: err}
	}
}

// setFirst shows page p as the first page, in place of what was read.
func (t *threadsState) setFirst(p core.ThreadPage) {
	// A cached page is shared, and the pages after it are added to what is
	// kept here.
	t.all = slices.Clone(p.Threads)
	t.list, t.read, t.total, t.next = unresolved(t.all), len(p.Threads), p.Total, p.Next
	t.kept = p.Offline || p.Limited
	t.loadingMore, t.moreErr = false, nil
	t.got = true
}

// pin makes the cursor follow its row from now on, so that rows added or
// removed above it don't move it onto another thread.
func (t *threadsState) pin() {
	if t.rowCount() > 0 {
		t.moveTo(t.cursorRow())
	}
}

// receiveThreads takes a page of the threads.
func (m *detailModal) receiveThreads(msg threadsMsg) tea.Cmd {
	t := &m.th
	if msg.thread != m.thread.ID() || msg.gen != t.gen {
		return nil
	}
	if msg.err != nil && m.ctx.Err() != nil {
		return nil
	}
	t.pin()
	switch {
	case msg.err != nil && msg.more:
		t.loadingMore, t.moreErr = false, msg.err
	case msg.err != nil:
		t.err = msg.err
	case msg.more:
		t.all = append(t.all, msg.page.Threads...)
		t.list = unresolved(t.all)
		t.read += len(msg.page.Threads)
		t.total, t.next = msg.page.Total, msg.page.Next
		t.loadingMore = false
		t.kept = t.kept || msg.page.Offline || msg.page.Limited
	default:
		t.err = nil
		t.setFirst(msg.page)
	}
	if !m.loaded {
		return nil
	}
	m.applyThreadBlocks()
	jump := m.finishJump(msg)
	if msg.err == nil && m.onFiles() {
		// The diff holds every thread: read the next page behind this one.
		return tea.Batch(m.show(), m.readMoreThreads(), jump)
	}
	return tea.Batch(m.show(), jump)
}

// threadsOnline reads the threads again once GitHub answers again, if they
// failed for want of an answer or were served kept.
func (m *detailModal) threadsOnline() tea.Cmd {
	t := &m.th
	if !t.started || !t.kept && !ui.Unreached(t.err) && !ui.Unreached(t.moreErr) {
		return nil
	}
	return m.reloadThreads()
}

// threadsBlock renders the block of the unresolved threads at the top of the
// Conversation tab, in w cells, whose first line is line base of the
// document, or nothing if there are none. It sets where the rows are.
func (m *detailModal) threadsBlock(w, base int) []string {
	t := &m.th
	t.rows = t.rows[:0]
	heading := func(s string) string { return gutter + m.theme.Muted.Bold(true).Render(s) }
	// A failed read of the first page says so above the threads that were
	// read before, if there are any.
	var failed string
	if t.err != nil {
		text, hint := m.threadsErr(t.err)
		if text != "" {
			line := m.theme.Error.Render(m.icons.Error) + " " + text
			if hint != "" {
				line += m.icons.Separator + hint
			}
			failed = ansi.Truncate(gutter+line, w, m.icons.Ellipsis)
		}
	}
	if len(t.list) == 0 && t.next == "" {
		if failed == "" {
			return nil
		}
		return []string{heading("Unresolved threads"), failed}
	}
	count := strconv.Itoa(len(t.list))
	if t.next != "" {
		count += "+"
	}
	lines := []string{heading("Unresolved threads (" + count + ")")}
	if failed != "" {
		lines = append(lines, failed)
	}
	cur := t.cursorRow()

	// The locations share a column, and so do the names, each no wider than
	// a share of the row.
	locCol, whoCol := 0, 0
	for i := range t.list {
		th := &t.list[i]
		pw := ansi.StringWidth(threadPlace(th))
		if th.Outdated {
			pw += 1 + len(outdatedTag)
		}
		locCol = max(locCol, pw)
		whoCol = max(whoCol, ansi.StringWidth(m.threadWho(firstComment(th), false)))
		if last := lastReply(th); last != nil {
			whoCol = max(whoCol, ansi.StringWidth(m.threadWho(last, false)))
		}
	}
	locCol = min(locCol, max((w-rowPrefix)/3, 10))
	whoCol = min(whoCol, max((w-rowPrefix)/4, 8))
	for i := range t.list {
		t.rows = append(t.rows, base+len(lines))
		lines = append(lines, m.threadLines(&t.list[i], i == cur, locCol, whoCol, w)...)
	}
	if t.next != "" {
		t.rows = append(t.rows, base+len(lines))
		lines = append(lines, m.moreLine(len(t.list) == cur, w))
	}
	return lines
}

// firstComment returns the comment that began thread th, or nil if none was
// read.
func firstComment(th *core.Thread) *core.ThreadComment {
	if len(th.Comments) == 0 {
		return nil
	}
	return &th.Comments[0]
}

// lastReply returns the last comment of thread th that was read, or nil if
// it has only one.
func lastReply(th *core.Thread) *core.ThreadComment {
	if len(th.Comments) < 2 {
		return nil
	}
	return &th.Comments[len(th.Comments)-1]
}

// threadPlace names where thread th is: its file and its line, or the file
// alone for a thread on the whole file.
func threadPlace(th *core.Thread) string {
	where := ui.OneLine(th.Path)
	line := th.Line
	if line < 1 {
		line = th.OriginalLine
	}
	if !th.File && line > 0 {
		where += ":" + strconv.Itoa(line)
	}
	return where
}

// threadWho renders who wrote comment c and how long ago.
func (m *detailModal) threadWho(c *core.ThreadComment, styled bool) string {
	if c == nil {
		return ""
	}
	who, age := ui.OneLine(c.Author.Login), m.icons.Separator+m.dates.Short(c.At, m.now())
	if !styled {
		return who + age
	}
	return m.st.commenter.Render(who) + m.st.age.Render(age)
}

// threadLines renders the rows of thread th in w cells: where it is and what
// it began with, and under it the last reply, if it has one. The places are in
// a column of loc cells, and the names of who.
func (m *detailModal) threadLines(th *core.Thread, cursor bool, loc, who, w int) []string {
	st := &m.st
	mark := " "
	place := st.title
	if cursor {
		mark = m.theme.Accent.Render(m.icons.Cursor)
		place = st.selected
	}
	// The path loses its beginning when it is cut, since its end names the
	// file.
	room := loc
	if th.Outdated {
		room -= 1 + len(outdatedTag)
	}
	where := cutHead(threadPlace(th), room, m.icons.Ellipsis)
	cell := place.Render(where)
	if th.Outdated {
		cell += " " + st.age.Render(outdatedTag)
	}
	head := mark + " " + m.threadMark(th) + " " + ui.Fit(cell, loc) + "  "

	first := firstComment(th)
	body := func(c *core.ThreadComment, style func(...string) string, room int) string {
		return style(termtext.Truncate(ui.OneLine(c.Body), room, m.icons.Ellipsis))
	}
	room = max(w-ansi.StringWidth(head)-who-2, 0)
	line := head
	if first != nil {
		line += ui.Fit(m.threadWho(first, true), who) + "  " + body(first, st.title.Render, room)
	}
	lines := []string{ansi.Truncate(line, w, "")}
	if last := lastReply(th); last != nil {
		indent := strings.Repeat(" ", ansi.StringWidth(head))
		reply := indent + ui.Fit(m.threadWho(last, true), who) + "  " + body(last, st.author.Render, room)
		lines = append(lines, ansi.Truncate(reply, w, ""))
	}
	return lines
}

// moreLine renders the row that reads the next page of threads, or says that
// it is being read, or why it couldn't be.
func (m *detailModal) moreLine(cursor bool, w int) string {
	t := &m.th
	mark := " "
	style := m.st.author
	if cursor {
		mark, style = m.theme.Accent.Render(m.icons.Cursor), m.theme.Accent
	}
	text := "Read more threads"
	var tail string
	switch {
	case t.loadingMore:
		text = "Reading more threads" + m.icons.Ellipsis
	case t.moreErr != nil:
		reason, hint := m.threadsErr(t.moreErr)
		text = "Couldn't read more threads"
		tail = m.icons.Separator + strings.TrimSpace(reason+" "+hint)
	case t.total > t.read:
		tail = m.icons.Separator + strconv.Itoa(t.read) + " of " + strconv.Itoa(t.total) + " read"
	}
	line := mark + " " + m.st.age.Render(m.icons.Down) + " " + style.Render(text) + m.st.age.Render(tail)
	return ansi.Truncate(line, w, m.icons.Ellipsis)
}

// cutHead cuts s to w cells by dropping its start, which an ellipsis takes
// the place of.
func cutHead(s string, w int, ellipsis string) string {
	sw := ansi.StringWidth(s)
	if sw <= w {
		return s
	}
	ew := ansi.StringWidth(ellipsis)
	if w <= ew {
		return termtext.Truncate(s, w, "")
	}
	return ellipsis + ansi.TruncateLeft(s, sw-(w-ew), "")
}

// pressThreads takes a key of the unresolved threads, which move the cursor
// through them while they show, and show the one under it in the diff. It
// reports whether it took the key; otherwise the thread scrolls.
func (m *detailModal) pressThreads(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	t := &m.th
	n := t.rowCount()
	if !m.onConversation() || !t.threadsShow() {
		return nil, false
	}
	k := m.keys
	cur := t.cursorRow()
	top, h := m.thread.YOffset(), m.thread.Height()
	visible := func(i int) bool { return i >= 0 && i < n && t.rows[i] >= top && t.rows[i] < top+h }
	switch {
	case keymap.Matches(msg, k.thread.Down):
		if !visible(cur + 1) {
			return nil, false
		}
		t.moveTo(cur + 1)
		return m.show(), true
	case keymap.Matches(msg, k.thread.Up):
		if !visible(cur - 1) {
			return nil, false
		}
		t.moveTo(cur - 1)
		return m.show(), true
	case keymap.Matches(msg, k.showThread):
		if m.thread.OnDiagram() || !visible(cur) {
			return nil, false
		}
		if cur >= len(t.list) {
			return m.readMoreThreads(), true
		}
		if t.list[cur].Path != "" {
			m.pushReturn()
		}
		return m.showThreadAt(&t.list[cur]), true
	}
	return nil, false
}

// threadsShow reports whether the block of threads shows rows that the cursor
// moves through.
func (t *threadsState) threadsShow() bool {
	n := t.rowCount()
	return n > 0 && len(t.rows) >= n
}

// conversationLayer returns the layer of the keys of the Conversation tab: the
// key that acts on the row under the cursor, matched before the thread's keys,
// and then the thread's. The thread's own use of the same key, to toggle a
// diagram, takes over while one is on screen.
func (m *detailModal) conversationLayer() keyhelp.Layer {
	enter := m.keys.showThread
	enter.SetEnabled(enter.Enabled() && m.th.threadsShow() && !m.thread.OnDiagram())
	if m.th.cursorRow() >= len(m.th.list) {
		enter.SetHelp(enter.Help().Key, "read more")
	}
	resolve := m.keys.resolve
	resolve.SetEnabled(resolve.Enabled() && m.th.threadsShow() && m.th.cursorRow() < len(m.th.list) && !m.thread.OnDiagram())
	own := ui.ContextLayer(ctxConversation, []key.Binding{enter, resolve}, []key.Binding{enter})
	return ui.MergeLayers(ctxConversation, own, ui.ContextHelp(ctxConversation, m.thread, false))
}

// showThreadAt shows the Files tab with the diff's cursor on thread th, which
// sits under its line, or under the header of its file if the diff can't place
// it: a thread whose code has changed since, or that is on the whole file.
func (m *detailModal) showThreadAt(th *core.Thread) tea.Cmd {
	if th.Path == "" {
		return nil
	}
	cmd := m.switchTo(filesTab)
	if m.files == nil {
		return cmd
	}
	// The diff holds the thread as a block, which waits for its file.
	return tea.Batch(cmd, m.seekDiff(th.Path, func(d *diff.Model) { d.SeekBlock(th.ID) }))
}
