package issues

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
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
	// issue is what is known of the issue, and loaded says whether
	// anything is: a modal opened from the search starts with nothing.
	issue  core.Issue
	loaded bool

	thread thread.Model[core.Comment]
	// ctx bounds the reads of the modal and is cancelled when it closes.
	ctx    context.Context
	cancel context.CancelFunc
	closed bool

	// prompt is where a comment or the labels are written, under the
	// thread, while composing says what for.
	prompt    prompt.Model
	composing composing
	// md renders comment bodies at mdWidth.
	md      *glamour.TermRenderer
	mdWidth int

	width, height int
	theme         ui.Theme
	rows          rowStyles
	chips         chipCache
}

// openDetail opens a modal on issue number of repo. it is the list item,
// shown until the issue arrives, or nil when there is none.
func (s *Section) openDetail(repo core.RepoRef, number int, it *core.Issue) tea.Cmd {
	ctx, cancel := context.WithCancel(s.ctx)
	m := &detailModal{
		svc:     s.svc,
		keys:    s.keys,
		now:     s.now,
		sendCtx: s.ctx,
		repo:    repo,
		number:  number,
		ctx:     ctx,
		cancel:  cancel,
		theme:   s.theme,
		rows:    s.rows,
		chips:   newChipCache(s.rows),
	}
	svc, q := s.svc, issuesvc.CommentsQuery{Repo: repo, Number: number}
	fetch := func(ctx context.Context, cursor string) ([]core.Comment, string, error) {
		q := q
		q.Cursor = cursor
		p, err := svc.Comments(ctx, q)
		return p.Items, p.Next, err
	}
	m.thread = thread.New(fetch, m.renderComment,
		thread.WithContext(ctx),
		thread.WithKeyMap(s.keys.thread),
		thread.WithStyles(s.theme.Thread()),
		thread.WithFocused(true),
	)
	switch cached, ok := svc.CachedGet(repo, number); {
	case ok:
		m.issue, m.loaded = cached, true
	case it != nil:
		m.issue, m.loaded = *it, true
	}
	show := m.thread.Init()
	if m.loaded {
		show = m.show()
	}
	// The loads start once the modal is open, so that the app has it to
	// pass their results to.
	return tea.Sequence(ui.OpenModal(m), tea.Batch(show, m.get()))
}

// Title implements ui.Modal.
func (m *detailModal) Title() string {
	n := "#" + strconv.Itoa(m.number)
	if !m.loaded || m.issue.Title == "" {
		return n
	}
	return n + " " + clean(m.issue.Title)
}

// SetSize implements ui.Modal.
func (m *detailModal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.layout()
}

// SetTheme implements ui.Modal. It builds every style the modal uses.
func (m *detailModal) SetTheme(t ui.Theme) {
	m.theme = t
	m.rows = newRowStyles(t)
	m.chips = newChipCache(m.rows)
	m.md = nil
	m.thread.SetStyles(t.Thread())
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
		return ui.CloseModal(m)
	case key.Matches(msg, k.Comment):
		return m.compose(composeComment)
	case key.Matches(msg, k.Label):
		return m.compose(composeLabels)
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
// itself, such as no body.
func (m *detailModal) get() tea.Cmd {
	svc, repo, number, id, ctx := m.svc, m.repo, m.number, m.thread.ID(), m.ctx
	return func() tea.Msg {
		it, err := svc.Get(ctx, repo, number)
		return issueMsg{thread: id, issue: it, err: err}
	}
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
		return ui.Notify(toast.Error, "Couldn't load #"+strconv.Itoa(m.number)+": "+msg.err.Error())
	}
	m.issue, m.loaded = msg.issue, true
	return m.show()
}

// setState closes or reopens the issue. The service shows the change in
// its cache at once, so the modal and the list behind it show it from
// there, then the change is sent.
func (m *detailModal) setState(state core.State) tea.Cmd {
	if !m.loaded {
		return nil
	}
	op, what, ok := stateChange(m.svc, m.repo, m.issue, state)
	if !ok {
		return nil
	}
	return tea.Batch(m.reload(), m.changed(), ui.Do(m.sendCtx, ui.IssuesTitle, op, what))
}

// changed tells the section that the issue changed in the cache.
func (m *detailModal) changed() tea.Cmd {
	repo := m.repo
	return func() tea.Msg { return changedMsg{repo: repo} }
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
	b.WriteString(t.Title.Render(clean(it.Title)))
	b.WriteString(t.Muted.Render("  #" + strconv.Itoa(it.Number)))
	b.WriteString("\n  ")

	if it.State == core.StateOpen {
		b.WriteString(m.rows.openBadge)
	} else {
		b.WriteString(m.rows.closedBadge)
	}
	b.WriteString("  ")
	b.WriteString(t.Muted.Render(login(it.Author)))
	b.WriteString(t.Subtle.Render(" opened " + ago(it.CreatedAt, now)))
	if it.UpdatedAt.After(it.CreatedAt) {
		b.WriteString(dot)
		b.WriteString(t.Subtle.Render("updated " + ago(it.UpdatedAt, now)))
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

// renderComment renders a comment as its author and age over its body. A
// comment GitHub hasn't confirmed yet is subtle and says it is sending.
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
		body := t.Subtle.Width(max(width-4, 1)).Render(strings.TrimSpace(c.Body))
		for l := range strings.SplitSeq(body, "\n") {
			b.WriteString("\n  ")
			b.WriteString(l)
		}
		return b.String()
	}
	b.WriteString(t.Title.Render(login(c.Author)))
	b.WriteString(t.Subtle.Render(" · " + ago(c.CreatedAt, m.now())))
	b.WriteByte('\n')
	b.WriteString(m.markdown(c.Body, width))
	return b.String()
}

// markdown renders a comment body at width with the thread's markdown
// style. The renderer is kept until the width or the theme changes.
func (m *detailModal) markdown(body string, width int) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	if m.md == nil || m.mdWidth != width {
		md, err := glamour.NewTermRenderer(
			glamour.WithStyles(m.theme.Thread().Markdown),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return body
		}
		m.md, m.mdWidth = md, width
	}
	out, err := m.md.Render(body)
	if err != nil {
		return body
	}
	return trimBlank(out)
}

// trimBlank drops the blank lines around s.
func trimBlank(s string) string {
	lines := strings.Split(s, "\n")
	blank := func(l string) bool { return strings.TrimSpace(ansi.Strip(l)) == "" }
	for len(lines) > 0 && blank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

func login(u core.User) string {
	if u.Login == "" {
		return "ghost"
	}
	return u.Login
}

// ago is ui.Ago as prose, such as "3d ago" or "just now".
func ago(t, now time.Time) string {
	a := ui.Ago(t, now)
	if a == "now" {
		return "just now"
	}
	return a + " ago"
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
