package issues

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
	"github.com/eggzec/gh-tui/pkg/markdown"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// issueMsg carries the issue that Get read for the thread that asked.
type issueMsg struct {
	thread int64
	issue  core.Issue
	err    error
}

// changedMsg tells the section that a modal changed an issue of repo in the
// cache, so that the list behind it shows the change at once.
type changedMsg struct {
	repo core.RepoRef
}

// detailModal shows an issue with its comments in a modal, and changes it.
// It is opened with [Section.openDetail].
type detailModal struct {
	svc  Service
	keys keyMap
	now  func() time.Time
	// sendCtx bounds the changes, which go on after the modal closes.
	sendCtx context.Context

	repo   core.RepoRef
	number int
	// other reports whether repo isn't the one selected, which the title
	// then names.
	other bool
	// caps is what the viewer may do in repo, as far as it is known, and
	// viewer who they are, or empty.
	caps   core.RepoCaps
	viewer string
	// token is what the token may do.
	token *ui.Token
	// issue is what is known of the issue, and loaded says whether
	// anything is: a modal opened from the search starts with nothing.
	issue  core.Issue
	loaded bool
	// failed is why the last read of the issue failed, or nil.
	failed error

	thread thread.Model[core.Comment]
	// ctx bounds the reads of the modal and is cancelled when it closes.
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
	// resume lets the reads ahead of the list go on, once the issue has
	// loaded or the modal closed.
	resume func()

	// prompt is where a comment or the labels are written, under the
	// thread, while composing says what for.
	prompt    prompt.Model
	composing composing
	// ask is the change waiting for the user to confirm it, on the last
	// line, in the styles of confirmSt.
	ask       *ui.Confirm
	confirmSt ui.ConfirmStyles

	width, height int
	theme         ui.Theme
	rows          rowStyles
	icons         ui.Icons
	dates         ui.Dates
	chips         chipCache
}

// openDetail opens a modal on issue number of repo. it is the list item,
// shown until the issue arrives, or nil when there is none.
func (s *Section) openDetail(repo core.RepoRef, number int, it *core.Issue, showRepo bool, from ui.Pauser) tea.Cmd {
	// The reads of the modal are one trace, however many pages it reads.
	ctx, cancel := context.WithCancel(obs.WithTrace(s.ctx, "open.issue"))
	s.ahead.Opened(commentsQuery(repo, number))
	// The reads ahead wait, so that the issue's requests go first: the
	// list's, and those of the list it was opened from.
	resume := ui.PauseAll(s.ahead, from)
	_, cached := s.svc.CachedGet(repo, number)
	slog.InfoContext(ctx, "open", "span", "tui", "kind", "issue", "repo", repo.String(), "number", number, "cached", cached)
	m := &detailModal{
		svc:     s.svc,
		keys:    s.keys,
		now:     s.now,
		sendCtx: s.ctx,
		repo:    repo,
		number:  number,
		other:   showRepo || !s.hasRepo || !repo.Same(s.repo),
		caps:    s.capsOf(repo),
		viewer:  s.viewer,
		token:   s.voice.Token,
		ctx:     ctx,
		cancel:  cancel,
		resume:  resume,
		theme:   s.theme,
		rows:    s.rows,
		icons:   s.icons,
		dates:   s.dates,
		chips:   newChipCache(s.rows),
	}
	m.confirmSt = s.theme.Confirm()
	svc, q := s.svc, commentsQuery(repo, number)
	fetch := func(ctx context.Context, cursor string) ([]core.Comment, string, error) {
		q := q
		q.Cursor = cursor
		p, err := svc.Comments(ctx, q)
		return p.Items, p.Next, err
	}
	// The thread fetches failed comments again with its own retry key.
	v := s.voice
	v.Retry = s.keys.thread.Retry
	m.thread = thread.New(fetch, m.renderComment,
		thread.WithContext(ctx),
		thread.WithKeyMap(s.keys.thread),
		thread.WithStyles(s.theme.Thread(s.icons)),
		thread.WithFocused(true),
		thread.WithErrorText(ui.ErrorText("load the comments", core.Target{Repo: repo, Number: number}.String(), v)),
	)
	m.thread.SetCutHint(ui.OpenHint(s.keys.Open))
	switch cached, ok := svc.CachedGet(repo, number); {
	case ok:
		m.issue, m.loaded = cached, true
	case it != nil:
		m.issue, m.loaded = *it, true
	}
	var caps tea.Cmd
	if !m.caps.Known {
		// The app reads those of the selected repository.
		caps = ui.LoadCaps(ctx, s.repos, repo)
	}
	if !m.loaded {
		// The loads start once the modal is open, so that the app has it
		// to pass their results to.
		return tea.Sequence(ui.OpenModal(m), tea.Batch(m.thread.Init(), m.get(), caps))
	}
	// What is cached shows at once, and is read again behind it.
	cp, primed := svc.CachedComments(q)
	if primed {
		m.thread.SetFirst(cp.Items, cp.Next)
	}
	loads := []tea.Cmd{m.show(), m.get(), caps}
	if primed {
		loads = append(loads, m.thread.Reload())
	}
	return tea.Sequence(ui.OpenModal(m), tea.Batch(loads...))
}

// capsOf returns what the viewer may do in repo, as far as it is known.
func (s *Section) capsOf(repo core.RepoRef) core.RepoCaps {
	if s.hasRepo && repo.Same(s.repo) {
		return s.caps
	}
	return ui.CachedCaps(s.repos, repo)
}

// commentsQuery selects the first page of the comments on issue number of
// repo, as the modal reads it, so that what is read ahead is what the modal
// finds in the cache.
func commentsQuery(repo core.RepoRef, number int) issuesvc.CommentsQuery {
	return issuesvc.CommentsQuery{Repo: repo, Number: number}
}

// Title implements ui.Modal.
func (m *detailModal) Title() string {
	n := "#" + strconv.Itoa(m.number)
	if m.other {
		n = m.repo.String() + n
	}
	if !m.loaded || m.issue.Title == "" {
		return n
	}
	return n + " " + ui.OneLine(m.issue.Title)
}

// Link implements ui.Linked.
func (m *detailModal) Link() string { return m.issue.URL }

// SetSize implements ui.Modal.
func (m *detailModal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.layout()
}

// SetTheme implements ui.Modal. It builds every style the modal uses.
func (m *detailModal) SetTheme(t ui.Theme) {
	m.theme = t
	m.confirmSt = t.Confirm()
	m.rows = newRowStyles(t, m.icons)
	m.chips = newChipCache(m.rows)
	m.thread.SetStyles(t.Thread(m.icons))
	if m.composing != composeNone {
		m.prompt.SetStyles(t.Prompt())
	}
	if m.loaded {
		// The header is styled too. The thread loads whatever the new
		// layout needs on its next message, so the command can go.
		_ = m.show()
	}
}

// View implements ui.Modal. It renders exactly the modal's size.
func (m *detailModal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	v := m.composed()
	if m.ask == nil {
		return v
	}
	// The question lines up with the issue, two cells in. Over a prompt,
	// it takes the place of the prompt's keys, which wait for the answer.
	lines := m.ask.Lines(m.confirmSt, m.keys.confirm, max(m.width-2, 0), min(m.height, ui.ConfirmLines))
	for i, l := range lines {
		lines[i] = ansi.Truncate("  "+l, m.width, "")
	}
	return ui.OverLastLines(v, lines)
}

// composed renders the thread, and the prompt under it while one is open.
func (m *detailModal) composed() string {
	if m.composing == composeNone {
		return m.thread.View()
	}
	if m.thread.Height() == 0 {
		return m.prompt.View()
	}
	return m.thread.View() + "\n" + m.prompt.View()
}

// Update implements ui.Modal. After the modal closed, it ignores what
// arrives late.
func (m *detailModal) Update(msg tea.Msg) tea.Cmd {
	if m.closed {
		return nil
	}
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		return m.press(msg)
	case issueMsg:
		return m.gotIssue(msg)
	case ui.SyncMsg:
		if msg.Err != nil || msg.Key != issuesvc.SyncKey(m.repo) {
			return nil
		}
		return tea.Batch(m.thread.Reload(), m.get())
	case ui.CapsMsg:
		if msg.Repo.Same(m.repo) {
			m.caps = msg.Caps
		}
		return nil
	case viewerMsg:
		m.viewer = msg.login
		return nil
	case ui.OnlineMsg:
		return m.online()
	case ui.DoneMsg:
		// The thread reloads too, since the change may be a comment.
		if msg.From != ui.IssuesTitle {
			return nil
		}
		return tea.Batch(m.reload(), m.thread.Reload())
	case prompt.SubmitMsg, prompt.CancelMsg:
		return m.promptDone(msg)
	case tea.PasteMsg:
		if m.composing != composeNone {
			// A paste while the prompt's text is being confirmed would
			// change what the question asks about.
			if m.ask != nil {
				return nil
			}
			var cmd tea.Cmd
			m.prompt, cmd = m.prompt.Update(msg)
			return cmd
		}
	}
	var cmd tea.Cmd
	m.thread, cmd = m.thread.Update(msg)
	return cmd
}

func (m *detailModal) press(msg tea.KeyPressMsg) tea.Cmd {
	if m.ask != nil {
		return m.answer(msg)
	}
	if m.composing != composeNone {
		// Every key is typing, even the modal's own; esc cancels the
		// prompt, not the modal.
		var cmd tea.Cmd
		m.prompt, cmd = m.prompt.Update(msg)
		return cmd
	}
	k := m.keys
	switch {
	case key.Matches(msg, k.Back):
		m.closed = true
		m.cancel()
		m.resume()
		return ui.CloseModal(m)
	case key.Matches(msg, k.Comment):
		return m.compose(composeComment, ui.ActComment)
	case key.Matches(msg, k.Label):
		return m.compose(composeLabels, ui.ActLabel)
	case key.Matches(msg, k.Close):
		return m.setState(core.StateClosed)
	case key.Matches(msg, k.Reopen):
		return m.setState(core.StateOpen)
	case key.Matches(msg, k.Refresh):
		m.svc.Invalidate(m.repo)
		return tea.Batch(m.thread.Reload(), m.get())
	case key.Matches(msg, k.Open):
		if m.issue.URL != "" {
			return ui.Open(m.issue.URL)
		}
		return nil
	}
	var cmd tea.Cmd
	m.thread, cmd = m.thread.Update(msg)
	return cmd
}

// get reads the issue, since a list page may carry less than the issue
// itself, such as no body. What failed before is forgotten, so that GitHub
// answering again doesn't read it once more while this read is under way.
func (m *detailModal) get() tea.Cmd {
	m.failed = nil
	svc, repo, number, id, ctx, resume := m.svc, m.repo, m.number, m.thread.ID(), m.ctx, m.resume
	return func() tea.Msg {
		start := time.Now()
		it, err := svc.Get(ctx, repo, number)
		resume()
		obs.End(ctx, start, err, "span", "tui", "repo", repo.String(), "number", number)
		return issueMsg{thread: id, issue: it, err: err}
	}
}

// online reads again, now that GitHub answers again, the issue and the
// comments that failed for want of an answer from it.
func (m *detailModal) online() tea.Cmd {
	var get tea.Cmd
	if ui.Unreached(m.failed) {
		get = m.get()
	}
	return tea.Batch(get, ui.RetryUnreached(&m.thread))
}

// gotIssue shows the issue that Get read for this modal.
func (m *detailModal) gotIssue(msg issueMsg) tea.Cmd {
	if msg.thread != m.thread.ID() {
		return nil
	}
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			return nil
		}
		m.failed = msg.err
		return ui.Fail("load #"+strconv.Itoa(m.number), core.About(m.subject(), msg.err))
	}
	m.issue, m.loaded, m.failed = msg.issue, true, nil
	return m.show()
}

// setState closes or reopens the issue, once the user confirms it on the
// last line of the modal. The service shows the change in its cache at
// once, so the modal and the list behind it show it from there, then the
// change is sent.
func (m *detailModal) setState(state core.State) tea.Cmd {
	if !m.loaded {
		return nil
	}
	c, ok, refusal := stateChange(m.svc, m.gate(), m.issue, state,
		func() (core.Issue, ui.Gate, bool) { return m.issue, m.gate(), m.loaded },
		func(op *optimistic.Op, what string) tea.Cmd {
			return tea.Batch(m.reload(), m.changed(), m.send(op, what))
		})
	if !ok {
		return refusal
	}
	m.ask = &c
	return nil
}

// answer takes the answer to the question on the last line: yes makes the
// change, and no steps back to the issue. Other keys do nothing.
func (m *detailModal) answer(msg tea.KeyPressMsg) tea.Cmd {
	cmd, done := m.keys.confirm.Answer(*m.ask, msg)
	if done {
		m.ask = nil
	}
	return cmd
}

// gate decides what the viewer may do in the repository.
func (m *detailModal) gate() ui.Gate {
	return ui.Gate{Repo: m.repo, Caps: m.caps, Viewer: m.viewer, Token: m.token}
}

// changed tells the section that the issue changed in the cache.
func (m *detailModal) changed() tea.Cmd {
	repo := m.repo
	return func() tea.Msg { return changedMsg{repo: repo} }
}

// send sends op, the change what, such as "close #5", of the issue, whose
// failure the app tells of naming the issue.
func (m *detailModal) send(op ui.Op, what string) tea.Cmd {
	return ui.Do(m.sendCtx, ui.IssuesTitle, ui.About(m.subject(), op), what)
}

// subject names the issue in what the user reads of a failure, such as
// "eggzec/gh-tui#5", since GitHub may not find it or refuse access to it
// while it finds its repository.
func (m *detailModal) subject() string {
	return core.Target{Repo: m.repo, Number: m.number}.String()
}

// reload shows the issue from the cache again.
func (m *detailModal) reload() tea.Cmd {
	it, ok := m.svc.CachedGet(m.repo, m.number)
	if !ok {
		return nil
	}
	m.issue, m.loaded = it, true
	return m.show()
}

// show sets the document of the thread from the issue.
func (m *detailModal) show() tea.Cmd {
	return m.thread.SetDocument(m.header(m.issue), m.issue.Body)
}

// header renders the head of the detail: the title, the state, who opened
// it and when, the counts, the labels and the assignees.
func (m *detailModal) header(it core.Issue) string {
	t := m.theme
	now := m.now()
	dot := t.Subtle.Render(" · ")
	var b strings.Builder

	b.WriteString("  ")
	// The title and the number link to the issue's page.
	b.WriteString(termtext.Link(it.URL,
		t.Title.Render(ui.OneLine(it.Title))+t.Muted.Render("  #"+strconv.Itoa(it.Number))))
	b.WriteString("\n  ")

	b.WriteString(m.rows.badges[ui.IssueState(it)])
	b.WriteString("  ")
	b.WriteString(t.Muted.Render(login(it.Author)))
	b.WriteString(t.Subtle.Render(" opened " + m.dates.Prose(it.CreatedAt, now)))
	if it.UpdatedAt.After(it.CreatedAt) {
		b.WriteString(dot)
		b.WriteString(t.Subtle.Render("updated " + m.dates.Prose(it.UpdatedAt, now)))
	}
	b.WriteString(dot)
	b.WriteString(t.Muted.Render(commentMark + plural(it.Comments, "comment")))

	if len(it.Labels) > 0 {
		b.WriteString("\n  ")
		for i, l := range it.Labels {
			if i > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(m.chips.get(l).text)
		}
	}
	if len(it.Assignees) > 0 {
		names := make([]string, len(it.Assignees))
		for i, u := range it.Assignees {
			names[i] = login(u)
		}
		b.WriteString("\n  ")
		b.WriteString(t.Subtle.Render("assigned to "))
		b.WriteString(t.Muted.Render(strings.Join(names, ", ")))
	}
	return b.String()
}

// renderComment renders a comment as its author and age over its body,
// markdown rendered like the issue's. A comment GitHub hasn't confirmed
// yet says it is sending.
func (m *detailModal) renderComment(c core.Comment, width int) string {
	t := m.theme
	var b strings.Builder
	b.WriteString("  ")
	if issuesvc.IsPending(c) {
		// The service may not know who the viewer is.
		who := c.Author.Login
		if who == "" {
			who = "you"
		}
		b.WriteString(t.Subtle.Render(who + " · sending…"))
	} else {
		b.WriteString(t.Title.Render(login(c.Author)))
		b.WriteString(t.Subtle.Render(" · " + m.dates.Prose(c.CreatedAt, m.now())))
	}
	// The body is indented by two cells, with as much room on the right.
	if body := m.thread.Markdown(c.Body, markdown.Room(width, 4)); body != "" {
		b.WriteByte('\n')
		b.WriteString(markdown.Indent(body, "  "))
	}
	return b.String()
}

func login(u core.User) string {
	if u.Login == "" {
		return "ghost"
	}
	return u.Login
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
