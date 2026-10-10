package pulls

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/diff"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// The Overview tab of the modal, which it opens on: the state of the pull
// request at a glance, what needs attention with a row for each thing that
// does, whether it can merge and why not, who reviews it, and its
// description. It reads only what the detail carries.

const (
	// overviewSidebar is the width inside the frame from which the
	// reviewers stand in a sidebar, and sidebarWidth is its width.
	overviewSidebar = 110
	sidebarWidth    = 32
	// overviewTall is the height from which the header has three rows; a
	// shorter modal drops the line of who opened it, which then follows
	// what needs attention, since what to do matters more than who opened
	// it.
	overviewTall = 24
	// overviewLong is the width from which the header and the strip name
	// things in full.
	overviewLong = 100
	// attentionCap is how many failing checks the list names, and the
	// rows after them stand for the rest.
	attentionCap = 5
	// minDescription is how many rows the description keeps at least
	// before the list of what needs attention takes more.
	minDescription = 3
)

// attKind is what an Attention row is about, which decides where its row
// goes.
type attKind int

const (
	attReview attKind = iota
	attFailing
	attMoreFailing
	attChanges
	attThreads
	attConflict
	attBehind
)

// attention is a row of the Attention list. Its target is where the enter
// key goes: the log of a failing check, the diff at a thread, or the page
// of the pull request on GitHub where it can be fixed.
type attention struct {
	kind attKind
	// id names what the row is about, so that the cursor stays on it when
	// the list changes.
	id    string
	glyph string
	// subject is what the row is about, tag follows it in a quieter
	// style, and reason is the best one line about it.
	subject, tag, reason string
	// check is the failing check of an attFailing row, and thread the
	// review thread that an attChanges or attThreads row leads to, which
	// has no path when there is none.
	check  core.FailingCheck
	thread core.ReviewThread
}

// overviewState is the state of the Overview tab.
type overviewState struct {
	// cursor is the row of the Attention list that has the cursor.
	cursor int
	// rowID is the id of the row that has the cursor, which the cursor
	// follows when the rows change. It is empty until a message pinned it.
	rowID string
	// md renders the description, and desc and descW, descSrc keep what it
	// rendered, for the body and width it was rendered for.
	md      *markdown.Renderer
	desc    []string
	descW   int
	descSrc string
}

// viewerMsg tells the modal who the signed-in user is.
type viewerMsg struct {
	thread int64
	login  string
}

// readViewer reads who the signed-in user is, which the Attention list
// needs to tell a review asked of them from one asked of others.
func (m *detailModal) readViewer() tea.Cmd {
	read, id := m.whoAmI, m.thread.ID()
	if read == nil {
		return nil
	}
	ctx := m.ctx
	return func() tea.Msg {
		login, _ := read(ctx)
		return viewerMsg{thread: id, login: login}
	}
}

// onOverview reports whether the Overview tab shows.
func (m *detailModal) onOverview() bool { return m.tab == overviewTab }

// is reports whether login is the viewer.
func (m *detailModal) is(login string) bool {
	return m.viewer != "" && strings.EqualFold(m.viewer, login)
}

// authored reports whether the viewer opened the pull request.
func (m *detailModal) authored() bool {
	return m.detail.Caps.Authored || m.is(m.detail.Author.Login)
}

// asked reports whether a review is asked of the viewer. A review asked of a
// team the viewer belongs to is not counted: the detail doesn't say who is in
// a team.
func (m *detailModal) asked() bool {
	return slices.ContainsFunc(m.detail.Reviewers.Requested, func(r core.ReviewRequest) bool {
		return r.User.Login != "" && m.is(r.User.Login)
	})
}

// attentionRows lists what needs the viewer, in the order of their role:
// an author sees what keeps their pull request from merging first, a
// reviewer who was asked sees that first, and anyone else sees what
// stands in the way of merging.
func (m *detailModal) attentionRows() []attention {
	d := &m.detail
	if !m.loaded || d.State != core.StateOpen {
		return nil
	}
	by := map[attKind][]attention{}
	failing := m.failingRows()
	for i := range failing {
		by[failing[i].kind] = append(by[failing[i].kind], failing[i])
	}
	by[attChanges] = m.changeRows()
	if a, ok := m.threadsRow(); ok {
		by[attThreads] = []attention{a}
	}
	base := ui.OneLine(d.BaseRef)
	if d.Merge.Status == core.MergeDirty || d.Merge.Mergeable == core.MergeableConflicting {
		by[attConflict] = []attention{{
			kind: attConflict, id: "conflict", glyph: m.theme.Error.Render(m.icons.Run(ui.RunFailure)),
			subject: "Conflicts with " + base, reason: "resolve them on GitHub",
		}}
	} else if d.Merge.Status == core.MergeBehind {
		by[attBehind] = []attention{{
			kind: attBehind, id: "behind", glyph: m.theme.Warning.Render(m.icons.Warning),
			subject: "Behind " + base, reason: "update the branch on GitHub",
		}}
	}
	var order []attKind
	switch {
	case m.authored():
		order = []attKind{attFailing, attMoreFailing, attChanges, attThreads, attConflict, attBehind}
	case m.asked():
		by[attReview] = []attention{{
			kind: attReview, id: "review", glyph: m.theme.Accent.Render(m.icons.Ring),
			subject: "Review requested from you", reason: "read the changes in Files",
		}}
		order = []attKind{attReview, attFailing, attMoreFailing, attChanges, attThreads, attConflict, attBehind}
	default:
		order = []attKind{attConflict, attBehind, attFailing, attMoreFailing, attChanges, attThreads}
	}
	var rows []attention
	for _, k := range order {
		rows = append(rows, by[k]...)
	}
	return rows
}

// failingRows returns a row for each of the first failing checks, and one
// for the failing checks that none of them names.
func (m *detailModal) failingRows() []attention {
	d := &m.detail
	var rows []attention
	for i, c := range d.FailingChecks {
		if i == attentionCap {
			break
		}
		a := attention{
			kind: attFailing, id: "check:" + strconv.FormatInt(c.ID, 10) + ":" + c.Name, glyph: m.theme.Error.Render(m.icons.Run(ui.RunFailure)),
			subject: ui.OneLine(c.Name), reason: ui.OneLine(c.Reason), check: c,
		}
		if c.Required {
			a.tag = "required"
		}
		if a.reason == "" {
			a.reason = "failed"
		}
		rows = append(rows, a)
	}
	// The checks as the step last read them may count more than the detail.
	failing, _, _, _ := m.checkTally()
	if more := failing - len(rows); more > 0 {
		rows = append(rows, attention{
			kind: attMoreFailing, id: "more", glyph: m.theme.Error.Render(m.icons.Run(ui.RunFailure)),
			subject: plural(more, "more failing check"), reason: "see them in Checks",
		})
	}
	return rows
}

// changeRows returns a row for each reviewer whose latest verdict asks for
// changes, or one for the decision if the verdicts don't name who.
func (m *detailModal) changeRows() []attention {
	d := &m.detail
	glyph := m.theme.Warning.Render(m.icons.ChangesRequested)
	var rows []attention
	for _, v := range d.Reviewers.Verdicts {
		if v.State != core.ReviewStateChangesRequested {
			continue
		}
		a := attention{kind: attChanges, id: "changes:" + strings.ToLower(v.Author.Login), glyph: glyph, subject: ui.OneLine(v.Author.Login) + " requested changes"}
		var reason []string
		if t, ok := m.firstThread(v.Author.Login); ok {
			a.thread = t
		}
		if n := m.unresolvedBy(v.Author.Login); n > 0 {
			reason = append(reason, plural(n, "unresolved thread"))
		}
		if !v.SubmittedAt.IsZero() {
			reason = append(reason, m.dates.Short(v.SubmittedAt, m.now()))
		}
		a.reason = strings.Join(reason, m.icons.Separator)
		rows = append(rows, a)
	}
	if len(rows) == 0 && d.ReviewDecision == core.ReviewChangesRequested {
		rows = append(rows, attention{kind: attChanges, id: "changes", glyph: glyph, subject: "Changes requested", reason: "read the conversation"})
	}
	return rows
}

// firstThread returns the first unresolved review thread of login, or of
// anyone if login is empty.
func (m *detailModal) firstThread(login string) (core.ReviewThread, bool) {
	threads := m.detail.Threads.Threads
	for i := range threads {
		if !threads[i].Resolved && (login == "" || strings.EqualFold(threads[i].Author.Login, login)) {
			return threads[i], true
		}
	}
	return core.ReviewThread{}, false
}

// unresolvedBy counts the unresolved review threads that login started.
func (m *detailModal) unresolvedBy(login string) int {
	n := 0
	threads := m.detail.Threads.Threads
	for i := range threads {
		if !threads[i].Resolved && strings.EqualFold(threads[i].Author.Login, login) {
			n++
		}
	}
	return n
}

// threadsRow returns the row for the unresolved review threads, which says
// where the first one is.
func (m *detailModal) threadsRow() (attention, bool) {
	th := m.detail.Threads
	if th.Unresolved == 0 {
		return attention{}, false
	}
	count := strconv.Itoa(th.Unresolved)
	if th.Truncated {
		count += "+"
	}
	noun := "threads"
	if th.Unresolved == 1 && !th.Truncated {
		noun = "thread"
	}
	a := attention{kind: attThreads, id: "threads", glyph: m.theme.Warning.Render(m.icons.Dot), subject: count + " unresolved " + noun}
	if t, ok := m.firstThread(""); ok {
		a.thread = t
		where := ui.OneLine(t.Path)
		if t.Line > 0 {
			where += ":" + strconv.Itoa(t.Line)
		}
		a.reason = strings.TrimSpace(where + " " + ui.OneLine(t.Excerpt))
	}
	return a, true
}

// attend goes to the target of the row of the Attention list under the
// cursor: the log of a failing check, the diff where a thread is, the
// checks, the changes to review, or the pull request on GitHub, where
// conflicts and a branch that is behind are fixed.
func (m *detailModal) attend() tea.Cmd {
	rows := m.attentionRows()
	if len(rows) == 0 {
		return nil
	}
	a := rows[m.cursorRow(rows)]
	switch a.kind {
	case attFailing:
		if !m.hasChecks() {
			return m.openPage("/checks")
		}
		cmd := m.switchTo(checksTab)
		if m.checks == nil {
			return cmd
		}
		return tea.Batch(cmd, m.checks.Open(a.check.Name, checkID(a.check)))
	case attMoreFailing:
		if !m.hasChecks() {
			return m.openPage("/checks")
		}
		return m.switchTo(checksTab)
	case attChanges, attThreads:
		return m.showThread(a.thread)
	case attReview:
		return m.switchTo(filesTab)
	case attConflict, attBehind:
		return m.openPage("")
	}
	return nil
}

// checkID returns the ID of the check run that c stands for, or 0 for a
// commit status.
func checkID(c core.FailingCheck) int64 {
	if !c.Run {
		return 0
	}
	return c.ID
}

// openPage opens the pull request on GitHub, at path below its page.
func (m *detailModal) openPage(path string) tea.Cmd {
	if m.detail.URL == "" {
		return nil
	}
	return ui.Open(m.detail.URL + path)
}

// showThread shows the Files tab with the diff's cursor on the line of a
// review thread. The Conversation tab doesn't list review threads, so it is
// where a thread goes that the diff can't place: one with no file, no line
// (the code it was about has changed), or none at all.
func (m *detailModal) showThread(t core.ReviewThread) tea.Cmd {
	if t.Path == "" || t.Line < 1 || t.Outdated {
		return m.switchTo(conversationTab)
	}
	cmd := m.switchTo(filesTab)
	if m.files == nil {
		return cmd
	}
	return tea.Batch(cmd, m.showLine(diff.Pos{Path: t.Path, Side: diff.NewSide, Line: t.Line}))
}

// pressOverview takes a key on the Overview tab: those that close,
// refresh and open, then the cursor's.
func (m *detailModal) pressOverview(msg tea.KeyPressMsg) tea.Cmd {
	k := m.keys
	switch {
	case key.Matches(msg, k.Back):
		return m.close()
	case key.Matches(msg, k.Refresh):
		m.svc.Invalidate(m.repo)
		return tea.Batch(m.get(), m.thread.Reload())
	case key.Matches(msg, k.Open):
		return m.openPage("")
	case key.Matches(msg, k.attend):
		return m.attend()
	}
	rows := m.attentionRows()
	page := max(m.height/3, 1)
	o := k.overview
	switch {
	case key.Matches(msg, o.Up):
		m.moveCursor(-1, rows)
	case key.Matches(msg, o.Down):
		m.moveCursor(1, rows)
	case key.Matches(msg, o.PageUp):
		m.moveCursor(-page, rows)
	case key.Matches(msg, o.PageDown):
		m.moveCursor(page, rows)
	case key.Matches(msg, o.HalfPageUp):
		m.moveCursor(-max(page/2, 1), rows)
	case key.Matches(msg, o.HalfPageDown):
		m.moveCursor(max(page/2, 1), rows)
	case key.Matches(msg, o.Home):
		m.moveCursor(-len(rows), rows)
	case key.Matches(msg, o.End):
		m.moveCursor(len(rows), rows)
	}
	return nil
}

// moveCursor moves the cursor d rows through n, and no further than the
// ends.
func (m *detailModal) moveCursor(d int, rows []attention) {
	i := min(max(m.cursorRow(rows)+d, 0), max(len(rows)-1, 0))
	m.ov.cursor = i
	m.ov.rowID = ""
	if i < len(rows) {
		m.ov.rowID = rows[i].id
	}
}

// cursorRow returns the index of the row that has the cursor: the row it
// was last on, or the place where it was if that row is gone.
func (m *detailModal) cursorRow(rows []attention) int {
	if m.ov.rowID != "" {
		if i := slices.IndexFunc(rows, func(a attention) bool { return a.id == m.ov.rowID }); i >= 0 {
			return i
		}
	}
	return min(max(m.ov.cursor, 0), max(len(rows)-1, 0))
}

// pinCursor makes the cursor follow its row from now on, so that rows added
// or removed above it, as when the viewer is found or a check stops failing,
// don't move it onto another row. It runs before a message changes the rows.
func (m *detailModal) pinCursor() {
	if rows := m.attentionRows(); len(rows) > 0 {
		m.moveCursor(0, rows)
	}
}

// overviewLayer returns the layer of the keys of the Overview tab.
func (m *detailModal) overviewLayer() []key.Binding {
	o := m.keys.overview
	attend := m.keys.attend
	attend.SetEnabled(attend.Enabled() && len(m.attentionRows()) > 0)
	return []key.Binding{attend, o.Up, o.Down, o.PageUp, o.PageDown, o.HalfPageUp, o.HalfPageDown, o.Home, o.End}
}

// overviewView renders the Overview tab in exactly the modal's size.
func (m *detailModal) overviewView() string {
	w, h := m.width, m.height
	if !m.loaded {
		return strings.Join(ui.FitLines([]string{m.filesWaiting()}, w, h), "\n")
	}
	tall := h >= overviewTall
	head := m.overviewHeader(tall)
	bh := max(h-len(head), 0)
	var body []string
	if w >= overviewSidebar {
		mw := w - sidebarWidth - 1
		left := m.overviewMain(mw, bh, tall, false)
		right := ui.FitLines(m.sidebar(sidebarWidth), sidebarWidth, bh)
		sep := m.st.sepLine.Render(m.icons.Border.Left)
		for i := range bh {
			body = append(body, left[i]+sep+right[i])
		}
	} else {
		body = m.overviewMain(w, bh, tall, true)
	}
	return strings.Join(ui.FitLines(append(head, body...), w, h), "\n")
}

// overviewHeader renders the rows over the body: the title, who opened it
// and where it comes from unless the modal is short, the strip that says
// where it stands, and a rule.
func (m *detailModal) overviewHeader(tall bool) []string {
	w, st := m.width, &m.st
	d := &m.detail
	inner := max(w-len(gutter), 1)
	title := termtext.Link(d.URL, st.selected.Render(termtext.Truncate(ui.OneLine(d.Title), inner, m.icons.Ellipsis)))
	lines := []string{gutter + title}
	if tall {
		lines = append(lines, gutter+m.identity(inner))
	}
	if s := m.strip(w < overviewLong); s != "" {
		lines = append(lines, gutter+ansi.Truncate(s, inner, m.icons.Ellipsis))
	}
	return append(lines, gutter+st.rule.Render(strings.Repeat(st.ic.Border.Top, inner)))
}

// identity renders who opened the pull request and where its branches go,
// cut to width: in full where there is room, else the number, the author
// and the branches.
func (m *detailModal) identity(width int) string {
	st := &m.st
	dot := st.sep.Render(st.ic.Separator)
	if m.width >= overviewLong {
		line := m.stateLine() + dot + strings.Join(m.refStats(), dot)
		return ansi.Truncate(line, width, m.icons.Ellipsis)
	}
	d := &m.detail
	parts := make([]string, 0, 5)
	parts = append(parts,
		st.badge(d.PullRequest)+" "+termtext.Link(d.URL, st.age.Render("#"+strconv.Itoa(d.Number))),
		st.title.Render(ui.OneLine(d.Author.Login)),
		st.author.Render(m.dates.Short(d.CreatedAt, m.now())))
	parts = append(parts, m.refStats()[:2]...)
	return ansi.Truncate(strings.Join(parts, dot), width, m.icons.Ellipsis)
}

// strip renders where the pull request stands in a fixed order, the worst
// first: whether it can merge, its checks, the decision of its reviewers
// and its unresolved threads. What has nothing to say is left out.
func (m *detailModal) strip(short bool) string {
	var parts []string
	if t, st, ok := m.mergeWord(); ok {
		parts = append(parts, st.Render(t))
	}
	parts = append(parts, m.stripChecks(short)...)
	d := &m.detail
	if d.State == core.StateOpen {
		switch d.ReviewDecision {
		case core.ReviewApproved:
			parts = append(parts, m.st.approved+m.st.author.Render(" approved"))
		case core.ReviewChangesRequested:
			text := " changes"
			if !short {
				text += " requested"
			}
			parts = append(parts, m.st.changes+m.st.author.Render(text))
		case core.ReviewRequired:
			text := " review"
			if !short {
				text += " required"
			}
			parts = append(parts, m.st.reviewRequired+m.st.author.Render(text))
		case core.ReviewNone:
		}
		if n := d.Threads.Unresolved; n > 0 {
			count := strconv.Itoa(n)
			if d.Threads.Truncated {
				count += "+"
			}
			if !short {
				count += " unresolved"
			}
			parts = append(parts, m.theme.Warning.Render(m.icons.Dot)+m.st.author.Render(" "+count))
		}
	}
	return strings.Join(parts, m.st.sep.Render(m.icons.Separator))
}

// stripChecks renders how the checks stand: the failing counted, then the
// pending, else that they pass.
func (m *detailModal) stripChecks(short bool) []string {
	failing, pending, passing, state := m.checkTally()
	if m.detail.State != core.StateOpen {
		return nil
	}
	say := func(glyph string, n int, word string) string {
		text := " " + strconv.Itoa(n)
		if !short {
			text += " " + word
		}
		return glyph + m.st.author.Render(text)
	}
	var out []string
	switch {
	case failing > 0:
		out = append(out, say(m.st.checksFail, failing, "failing"))
	case state == core.ChecksFailure:
		out = append(out, m.st.checksFail+m.st.author.Render(" failing"))
	}
	if pending > 0 && state == core.ChecksNone {
		out = append(out, say(m.theme.Warning.Render(m.icons.Run(ui.RunInProgress)), pending, "pending"))
	} else if pending > 0 {
		out = append(out, m.theme.Warning.Render(m.icons.Run(ui.RunInProgress))+m.st.author.Render(" pending"))
	}
	if failing == 0 && pending == 0 && passing > 0 {
		if state == core.ChecksSuccess {
			out = append(out, m.st.checksOK+m.st.author.Render(" checks"))
		} else {
			out = append(out, say(m.st.checksOK, passing, "checks"))
		}
	}
	return out
}

// mergeWord says whether the pull request can merge, in one word, as the
// strip begins with it, and ok is unset when it is merged or closed.
func (m *detailModal) mergeWord() (text string, style lipgloss.Style, ok bool) {
	d := &m.detail
	if d.State != core.StateOpen {
		return "", lipgloss.Style{}, false
	}
	switch {
	case d.Draft || d.Merge.Status == core.MergeDraft:
		return "Draft", m.theme.Muted.Bold(true), true
	case d.Merge.Status == core.MergeDirty || d.Merge.Mergeable == core.MergeableConflicting:
		return "Conflicts", m.theme.Error.Bold(true), true
	}
	switch d.Merge.Status {
	case core.MergeBehind:
		return "Behind", m.theme.Warning.Bold(true), true
	case core.MergeBlocked:
		return "Blocked", m.theme.Error.Bold(true), true
	case core.MergeClean, core.MergeHasHooks:
		return "Ready", m.theme.Success.Bold(true), true
	case core.MergeUnstable:
		return "Ready", m.theme.Warning.Bold(true), true
	case core.MergeUnknown, core.MergeDirty, core.MergeDraft:
	}
	return "Checking" + m.icons.Ellipsis, m.theme.Muted, true
}

// readyItem is a line of what stands between the pull request and its
// merge: a glyph, what it is about, how it stands, and the same in a few
// words for a short modal, which leaves it out when empty.
type readyItem struct {
	glyph               string
	label, detail, text string
}

// readiness lists what the merge needs and how each stands, from what the
// detail knows of the branch rules, the checks, the reviews and the
// threads.
func (m *detailModal) readiness() []readyItem {
	d := &m.detail
	mi := &d.Merge
	ok := m.st.checksOK
	bad := m.st.checksFail
	warn := func(g string) string { return m.theme.Warning.Render(g) }
	base := ui.OneLine(d.BaseRef)
	var items []readyItem
	if d.Draft || mi.Status == core.MergeDraft {
		items = append(items, readyItem{bad, "draft", "mark it ready first", "draft"})
	}
	if rc := mi.RequiredChecks; rc.Total() > 0 || len(mi.Rules.Checks) > 0 {
		total := max(rc.Total(), len(mi.Rules.Checks))
		switch {
		case rc.Failed > 0:
			items = append(items, readyItem{bad, "required checks",
				fmt.Sprintf("%d of %d failing", rc.Failed, total), fmt.Sprintf("checks %d/%d", rc.Failed, total)})
		case rc.Pending > 0:
			items = append(items, readyItem{warn(m.icons.Run(ui.RunInProgress)), "required checks",
				fmt.Sprintf("%d of %d pending", rc.Pending, total), fmt.Sprintf("checks %d pending", rc.Pending)})
		case rc.Total() == 0:
			items = append(items, readyItem{m.st.reviewRequired, "required checks", strconv.Itoa(total) + " required", "checks"})
		default:
			items = append(items, readyItem{ok, "required checks", fmt.Sprintf("all %d pass", total), "checks ok"})
		}
	}
	if it, found := m.reviewItem(); found {
		items = append(items, it)
	}
	if n := d.Threads.Unresolved; n > 0 || mi.Rules.Conversations {
		switch {
		case n > 0 && mi.Rules.Conversations:
			items = append(items, readyItem{bad, "conversations", strconv.Itoa(n) + " unresolved", strconv.Itoa(n) + " unresolved"})
		case n > 0:
			// The strip and the list of what needs attention say it already.
			items = append(items, readyItem{warn(m.icons.Dot), "conversations", strconv.Itoa(n) + " unresolved", ""})
		default:
			items = append(items, readyItem{ok, "conversations", "all resolved", "resolved"})
		}
	}
	switch {
	case mi.Status == core.MergeDirty || mi.Mergeable == core.MergeableConflicting:
		items = append(items, readyItem{bad, "conflicts", "with " + base, "conflicts"})
	case mi.Status == core.MergeBehind:
		items = append(items, readyItem{bad, "branch", "behind " + base, "behind"})
	case mi.Mergeable == core.MergeableYes || mi.Status == core.MergeClean || mi.Status == core.MergeBlocked || mi.Status == core.MergeUnstable:
		items = append(items, readyItem{ok, "up to date", "with " + base + ", no conflicts", "up to date"})
	}
	if a := mi.AutoMerge; a != nil {
		detail := "on"
		if a.Method != "" {
			detail += ", " + string(a.Method)
		}
		items = append(items, readyItem{warn(m.icons.Run(ui.RunInProgress)), "auto-merge", detail, "auto-merge"})
	}
	if q := mi.Queue; q.Queued {
		detail := "in the merge queue"
		if q.Position > 0 {
			detail += " at " + strconv.Itoa(q.Position)
		}
		items = append(items, readyItem{warn(m.icons.Run(ui.RunInProgress)), "merge queue", detail, "queued"})
	}
	return items
}

// reviewItem says how the reviews stand against what the branch needs.
func (m *detailModal) reviewItem() (readyItem, bool) {
	d := &m.detail
	var approvals, changes int
	for _, v := range d.Reviewers.Verdicts {
		switch v.State {
		case core.ReviewStateApproved:
			approvals++
		case core.ReviewStateChangesRequested:
			changes++
		case core.ReviewStatePending, core.ReviewStateCommented, core.ReviewStateDismissed:
		}
	}
	need := d.Merge.Rules.Approvals
	if need == 0 && approvals == 0 && changes == 0 && d.ReviewDecision != core.ReviewRequired {
		return readyItem{}, false
	}
	var detail, text string
	switch {
	case need > 0:
		detail = fmt.Sprintf("%d of %d approvals", approvals, need)
		text = fmt.Sprintf("%d/%d approved", approvals, need)
	case approvals > 0:
		detail, text = plural(approvals, "approval"), strconv.Itoa(approvals)+" approved"
	default:
		detail, text = "review required", "review required"
	}
	glyph := m.st.reviewRequired
	switch {
	case changes > 0:
		detail += ", " + plural(changes, "change request")
		text += ", " + strconv.Itoa(changes) + " change"
		glyph = m.st.changes
	case d.ReviewDecision == core.ReviewApproved || need > 0 && approvals >= need:
		glyph = m.st.approved
	}
	return readyItem{glyph, "reviews", detail, text}, true
}

// mergeMethodLine names how a merge would be made, and which methods the
// repository allows.
func (m *detailModal) mergeMethodLine() string {
	allowed := m.detail.Merge.Methods
	if len(allowed) == 0 {
		allowed = m.caps.MergeMethods()
	}
	if len(allowed) == 0 {
		return ""
	}
	use := allowed[0]
	switch {
	case slices.Contains(allowed, m.mergeMethod):
		use = m.mergeMethod
	case slices.Contains(allowed, core.MergeSquash):
		use = core.MergeSquash
	}
	name := string(use)
	if use == core.MergeCommit {
		name = "merge commit"
	}
	if len(allowed) > 1 {
		names := make([]string, len(allowed))
		for i, a := range allowed {
			names[i] = string(a)
		}
		return name + " (the repository allows " + strings.Join(names, ", ") + ")"
	}
	return name
}

// overviewMain renders the column of the Overview that has the Attention
// list: bh lines of w cells. Without the sidebar it holds the reviewers
// as well.
func (m *detailModal) overviewMain(w, bh int, tall, withReviewers bool) []string {
	st := &m.st
	heading := func(s string) string { return gutter + m.theme.Muted.Bold(true).Render(s) }
	open := m.detail.State == core.StateOpen
	rows := m.attentionRows()

	// What stands under the list, so that it knows how much room it has.
	var under []string
	if open {
		under = append(under, "")
		under = append(under, m.mergeBlock(w, tall)...)
	}
	if !tall {
		under = append(under, gutter+ansi.Truncate(m.identity(max(w-len(gutter), 1)), max(w-len(gutter), 1), ""))
	}
	if withReviewers {
		under = append(under, m.reviewersBlock(w, tall)...)
	}

	var lines []string
	if open {
		if tall {
			lines = append(lines, heading("Needs you"))
		}
		room := max(bh-len(lines)-len(under)-1-minDescription, minDescription)
		lines = append(lines, m.attentionLines(rows, w, room)...)
	}
	lines = append(lines, under...)
	if bh > len(lines)+1 {
		lines = append(lines, gutter+st.rule.Render(strings.Repeat(st.ic.Border.Top, max(w-len(gutter)*2, 1))))
		lines = append(lines, m.descriptionLines(w, bh-len(lines))...)
	}
	return ui.FitLines(lines, w, bh)
}

// attentionLines renders at most room rows of the Attention list, with the
// cursor in view, or says that nothing needs the viewer.
func (m *detailModal) attentionLines(rows []attention, w, room int) []string {
	if len(rows) == 0 {
		if !m.seen {
			return []string{gutter + m.theme.Muted.Render("Checking"+m.icons.Ellipsis)}
		}
		return []string{gutter + m.theme.Muted.Render("Nothing needs attention")}
	}
	cursor := m.cursorRow(rows)
	top := 0
	if cursor >= room {
		top = cursor - room + 1
	}
	shown := rows[top:min(top+room, len(rows))]
	// The subjects share a column, no wider than a share of the row.
	col := 0
	for i := range shown {
		col = max(col, ansi.StringWidth(shown[i].subject)+len(shown[i].tag)+min(len(shown[i].tag), 1))
	}
	col = min(col, max((w-len(gutter))/2, 12))
	out := make([]string, 0, len(shown))
	for i := range shown {
		out = append(out, m.attentionLine(&shown[i], i+top == cursor, col, w))
	}
	return out
}

// attentionLine renders a row of the Attention list in w cells, its
// subject in a column of col cells.
func (m *detailModal) attentionLine(a *attention, cursor bool, col, w int) string {
	st := &m.st
	mark := " "
	subject := st.title
	if cursor {
		mark = m.theme.Accent.Render(m.icons.Cursor)
		subject = st.selected
	}
	text := termtext.Truncate(a.subject, col, m.icons.Ellipsis)
	cell := subject.Render(text)
	if a.tag != "" {
		room := col - ansi.StringWidth(text) - 1
		if room > 0 {
			cell += " " + st.age.Render(termtext.Truncate(a.tag, room, ""))
		}
	}
	cell = ui.Fit(cell, col)
	head := mark + " " + a.glyph + " " + cell + "  "
	room := max(w-ansi.StringWidth(head), 0)
	return head + st.author.Render(termtext.Truncate(a.reason, room, m.icons.Ellipsis))
}

// mergeBlock renders what stands between the pull request and its merge:
// a line for each item where there is room, else one line that sums them
// up.
func (m *detailModal) mergeBlock(w int, tall bool) []string {
	st := &m.st
	if !m.seen && m.detail.Merge.Status == "" {
		return []string{gutter + m.theme.Muted.Bold(true).Render("Merge") + "  " + m.theme.Muted.Render("Checking"+m.icons.Ellipsis)}
	}
	items := m.readiness()
	method := m.mergeMethodLine()
	if !tall {
		parts := make([]string, 0, len(items)+1)
		for _, it := range items {
			if it.text != "" {
				parts = append(parts, it.glyph+" "+st.author.Render(it.text))
			}
		}
		if method != "" {
			parts = append(parts, st.author.Render(strings.SplitN(method, " (", 2)[0]))
		}
		line := gutter + m.theme.Muted.Bold(true).Render("Merge") + "  " + strings.Join(parts, st.sep.Render(m.icons.Separator))
		return []string{ansi.Truncate(line, w, m.icons.Ellipsis)}
	}
	lines := []string{gutter + m.theme.Muted.Bold(true).Render("Merge")}
	col := 0
	for _, it := range items {
		col = max(col, ansi.StringWidth(it.label))
	}
	for _, it := range items {
		line := gutter + it.glyph + " " + ui.Fit(st.title.Render(it.label), col) + "  " + st.author.Render(it.detail)
		lines = append(lines, ansi.Truncate(line, w, m.icons.Ellipsis))
	}
	if method != "" {
		line := gutter + "  " + ui.Fit(st.title.Render("method"), col) + "  " + st.author.Render(method)
		lines = append(lines, ansi.Truncate(line, w, m.icons.Ellipsis))
	}
	return lines
}

// reviewerRows renders each reviewer's verdict, and each review that is
// asked and not answered, in w cells.
func (m *detailModal) reviewerRows(w int) []string {
	st := &m.st
	rv := &m.detail.Reviewers
	var lines []string
	now := m.now()
	for _, v := range rv.Verdicts {
		var glyph, word string
		switch v.State {
		case core.ReviewStateApproved:
			glyph, word = st.approved, "approved"
		case core.ReviewStateChangesRequested:
			glyph, word = st.changes, "changes"
		case core.ReviewStateDismissed:
			glyph, word = m.theme.Muted.Render(m.icons.Run(ui.RunCancelled)), "dismissed"
		case core.ReviewStatePending:
			glyph, word = st.reviewRequired, "pending"
		case core.ReviewStateCommented:
			glyph, word = st.reviewRequired, "commented"
		}
		line := glyph + " " + st.title.Render(ui.OneLine(v.Author.Login)) + " " + st.author.Render(word)
		if !v.SubmittedAt.IsZero() {
			line += st.sep.Render(m.icons.Separator) + st.age.Render(m.dates.Short(v.SubmittedAt, now))
		}
		lines = append(lines, line)
	}
	if more := rv.VerdictsTotal - len(rv.Verdicts); more > 0 {
		lines = append(lines, st.age.Render("+"+strconv.Itoa(more)+" more"))
	}
	ring := m.theme.Muted.Render(m.icons.Ring)
	for _, r := range rv.Requested {
		var name string
		switch {
		case r.Team != "":
			name = "@" + r.Team
		case r.User.Login != "":
			name = r.User.Login
		default:
			name = "a reviewer you can't see"
		}
		word := "requested"
		if r.CodeOwner {
			word = "code owner"
		}
		lines = append(lines, ring+" "+st.title.Render(ui.OneLine(name))+" "+st.author.Render(word))
	}
	if more := rv.RequestedTotal - len(rv.Requested); more > 0 {
		lines = append(lines, st.age.Render("+"+strconv.Itoa(more)+" more requested"))
	}
	if len(lines) == 0 {
		lines = []string{st.age.Render("No reviewers")}
	}
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, w, m.icons.Ellipsis)
	}
	return lines
}

// metaRows renders the assignees, the labels and the milestone, which are
// left out when empty, in w cells.
func (m *detailModal) metaRows(w int) []string {
	st := &m.st
	d := &m.detail
	var lines []string
	add := func(name, value string) {
		if value != "" {
			lines = append(lines, ansi.Truncate(st.age.Render(ui.Fit(name, 10))+value, w, m.icons.Ellipsis))
		}
	}
	people := make([]string, 0, len(d.Assignees))
	for _, a := range d.Assignees {
		people = append(people, st.title.Render(ui.OneLine(a.Login)))
	}
	add("Assignees", strings.Join(people, "  "))
	names := make([]string, 0, len(d.Labels))
	for _, l := range d.Labels {
		names = append(names, st.label.Render(ui.OneLine(l.Name)))
	}
	add("Labels", strings.Join(names, "  "))
	if d.Milestone != "" {
		add("Milestone", st.title.Render(ui.OneLine(d.Milestone)))
	}
	return lines
}

// sidebar renders the reviewers and the rest of who and what the pull
// request involves, at width w.
func (m *detailModal) sidebar(w int) []string {
	lines := []string{m.theme.Muted.Bold(true).Render("Reviewers")}
	lines = append(lines, m.reviewerRows(w-1)...)
	if meta := m.metaRows(w - 1); len(meta) > 0 {
		lines = append(lines, "")
		lines = append(lines, meta...)
	}
	for i, l := range lines {
		lines[i] = " " + l
	}
	return lines
}

// reviewersBlock renders the reviewers where there is no sidebar: as one
// line in a short modal, and else as a block of lines under their heading.
func (m *detailModal) reviewersBlock(w int, tall bool) []string {
	st := &m.st
	inner := max(w-len(gutter), 1)
	if !tall {
		rows := m.reviewerRows(inner)
		line := m.theme.Muted.Bold(true).Render("Reviewers") + " " + strings.Join(rows, "  ")
		if meta := m.metaRows(inner); len(meta) > 0 {
			labels := make([]string, 0, len(m.detail.Labels))
			for _, l := range m.detail.Labels {
				labels = append(labels, st.label.Render(ui.OneLine(l.Name)))
			}
			if len(labels) > 0 {
				line += st.sep.Render(m.icons.Separator) + st.age.Render("Labels ") + strings.Join(labels, " ")
			}
		}
		return []string{gutter + ansi.Truncate(line, inner, m.icons.Ellipsis)}
	}
	lines := []string{"", gutter + m.theme.Muted.Bold(true).Render("Reviewers")}
	for _, l := range slices.Concat(m.reviewerRows(inner-2), m.metaRows(inner-2)) {
		lines = append(lines, gutter+"  "+l)
	}
	return lines
}

// descriptionLines renders the description of the pull request in the
// room of n lines of w cells, with a line that says there is more when it
// is cut.
func (m *detailModal) descriptionLines(w, n int) []string {
	if n <= 0 {
		return nil
	}
	body := m.detail.Body
	if strings.TrimSpace(body) == "" {
		return []string{gutter + m.theme.Muted.Render("No description")}
	}
	width := markdown.Room(w, 2*len(gutter))
	if m.ov.md == nil {
		m.ov.md = markdown.New(m.theme.Thread(m.icons).Markdown)
		m.ov.descW, m.ov.descSrc = 0, ""
	}
	if width != m.ov.descW || body != m.ov.descSrc {
		m.ov.desc = strings.Split(strings.Trim(m.ov.md.Render(body, width), "\n"), "\n")
		m.ov.descW, m.ov.descSrc = width, body
	}
	desc := m.ov.desc
	if len(desc) > n {
		desc = append(slices.Clone(desc[:max(n-1, 0)]), m.theme.Muted.Render(m.icons.Ellipsis+" the rest is in Conversation"))
	}
	out := make([]string, len(desc))
	for i, l := range desc {
		out[i] = gutter + l
	}
	return out
}
