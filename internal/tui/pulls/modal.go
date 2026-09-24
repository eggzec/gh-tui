package pulls

import (
	"context"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// detailMsg carries the detail of a pull request to the thread that asked
// for it.
type detailMsg struct {
	thread int64
	detail core.PullRequestDetail
	err    error
}

// changedMsg tells the section that a modal changed a pull request of repo
// in the cache, so that the list behind it shows the change at once.
type changedMsg struct {
	repo core.RepoRef
}

// detailModal shows a pull request with its comments in a modal, and
// changes it. It is opened with [Section.openDetail].
type detailModal struct {
	svc         Service
	keys        keyMap
	now         func() time.Time
	mergeMethod core.MergeMethod
	// sendCtx bounds the changes, which go on after the modal closes.
	sendCtx context.Context

	repo   core.RepoRef
	number int
	// detail is what is known of the pull request, and loaded says whether
	// anything is: a modal opened from the search starts with nothing.
	detail core.PullRequestDetail
	loaded bool

	thread thread.Model[core.Comment]
	// ctx bounds the reads of the modal and is cancelled when it closes.
	ctx    context.Context
	cancel context.CancelFunc
	closed bool

	width, height int
	st            styles
}

// openDetail opens a modal on pull request number of repo. pr is the list
// item, shown until the detail arrives, or nil when there is none.
func (s *Section) openDetail(repo core.RepoRef, number int, pr *core.PullRequest) tea.Cmd {
	ctx, cancel := context.WithCancel(s.ctx)
	m := &detailModal{
		svc:         s.svc,
		keys:        s.keys,
		now:         s.now,
		mergeMethod: s.mergeMethod,
		sendCtx:     s.ctx,
		repo:        repo,
		number:      number,
		ctx:         ctx,
		cancel:      cancel,
		st:          s.st,
	}
	svc := s.svc
	fetch := func(ctx context.Context, cursor string) ([]core.Comment, string, error) {
		p, err := svc.Comments(ctx, pulls.CommentsQuery{Repo: repo, Number: number, Cursor: cursor})
		return p.Items, p.Next, err
	}
	m.thread = thread.New(fetch, m.renderComment,
		thread.WithContext(ctx),
		thread.WithStyles(s.theme.Thread()),
		thread.WithKeyMap(s.keys.thread),
		thread.WithFocused(true),
	)
	switch d, ok := svc.CachedGet(repo, number); {
	case ok:
		m.detail, m.loaded = d, true
	case pr != nil:
		m.detail, m.loaded = core.PullRequestDetail{PullRequest: *pr}, true
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
	if !m.loaded || m.detail.Title == "" {
		return n
	}
	return n + " " + m.detail.Title
}

// SetSize implements ui.Modal.
func (m *detailModal) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.thread.SetSize(m.width, m.height)
	if m.loaded {
		// The next Update loads what the new size shows.
		_ = m.show()
	}
}

// SetTheme implements ui.Modal.
func (m *detailModal) SetTheme(t ui.Theme) {
	m.st = newStyles(t)
	m.thread.SetStyles(t.Thread())
	if m.loaded {
		_ = m.show()
	}
}

// View implements ui.Modal.
func (m *detailModal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	return m.thread.View()
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
	case detailMsg:
		return m.receive(msg)
	case ui.DoneMsg:
		// The change was confirmed or rolled back; either way the cache
		// has the outcome.
		if msg.From != ui.PullsTitle {
			return nil
		}
		return m.reload()
	case ui.SyncMsg:
		if msg.Err != nil || msg.Key != pulls.SyncKey(m.repo) {
			return nil
		}
		return tea.Batch(m.get(), m.thread.Reload())
	}
	var cmd tea.Cmd
	m.thread, cmd = m.thread.Update(msg)
	return cmd
}

func (m *detailModal) press(msg tea.KeyPressMsg) tea.Cmd {
	k := m.keys
	switch {
	case key.Matches(msg, k.Back):
		m.closed = true
		m.cancel()
		return ui.CloseModal(m)
	case key.Matches(msg, k.Refresh):
		m.svc.Invalidate(m.repo)
		return tea.Batch(m.get(), m.thread.Reload())
	case key.Matches(msg, k.Open):
		if m.detail.URL != "" {
			return ui.Open(m.detail.URL)
		}
		return nil
	case k.isChange(msg):
		if !m.loaded {
			return nil
		}
		op, what, warn := k.change(m.svc, m.mergeMethod, m.repo, m.detail.PullRequest, msg)
		if op == nil {
			return warn
		}
		repo := m.repo
		return tea.Batch(m.reload(),
			func() tea.Msg { return changedMsg{repo: repo} },
			ui.Do(m.sendCtx, ui.PullsTitle, op, what))
	}
	var cmd tea.Cmd
	m.thread, cmd = m.thread.Update(msg)
	return cmd
}

// get fetches the detail. A fresh cached detail costs no request.
func (m *detailModal) get() tea.Cmd {
	svc, ctx, repo, number, id := m.svc, m.ctx, m.repo, m.number, m.thread.ID()
	return func() tea.Msg {
		d, err := svc.Get(ctx, repo, number)
		return detailMsg{thread: id, detail: d, err: err}
	}
}

func (m *detailModal) receive(msg detailMsg) tea.Cmd {
	if msg.thread != m.thread.ID() {
		return nil
	}
	if msg.err != nil {
		if m.ctx.Err() != nil {
			return nil
		}
		return ui.Notify(toast.Error, "Couldn't load #"+strconv.Itoa(m.number)+": "+msg.err.Error())
	}
	m.detail, m.loaded = msg.detail, true
	return m.show()
}

// reload shows the detail from the cache again, which a change has just
// updated or rolled back.
func (m *detailModal) reload() tea.Cmd {
	d, ok := m.svc.CachedGet(m.repo, m.number)
	if !ok {
		return nil
	}
	m.detail, m.loaded = d, true
	return m.show()
}

// show sets the document of the thread from the detail.
func (m *detailModal) show() tea.Cmd {
	return m.thread.SetDocument(m.detailHeader(m.width), m.detail.Body)
}

// Help implements ui.Modal.
func (m *detailModal) Help() help.KeyMap {
	k, t := m.keys, m.keys.thread
	changes := k.changeHelp(m.detail.PullRequest, m.loaded)
	merge, closing, reopen := changes[0], changes[1], changes[2]
	return keyHelp{
		short: []key.Binding{t.Up, t.Down, k.Back, merge, closing, reopen, k.Open},
		full: [][]key.Binding{
			{t.Up, t.Down, t.PageUp, t.PageDown},
			{t.HalfPageUp, t.HalfPageDown, t.Top, t.Bottom},
			{k.Back, k.Refresh, k.Open},
			changes,
		},
	}
}

// detailHeader renders the head of the pull request at width: the title,
// its state, author and age, the refs and stats, the checks and the labels,
// over a rule.
func (m *detailModal) detailHeader(width int) string {
	d, st := &m.detail, &m.st
	inner := max(width-len(gutter), 1)
	now := m.now()
	var lines []string
	line := func(parts ...string) {
		lines = append(lines, gutter+strings.Join(parts, ""))
	}

	for l := range strings.SplitSeq(ansi.Wrap(d.Title, inner, ""), "\n") {
		line(st.selected.Render(l))
	}
	lines = append(lines, "")

	dot := st.sep.Render(" · ")
	line(st.badge(d.PullRequest), "  ",
		st.age.Render("#"+strconv.Itoa(d.Number)), dot,
		st.title.Render(d.Author.Login), st.author.Render(" opened "+ui.Ago(d.CreatedAt, now)+" ago"), dot,
		st.author.Render("updated "+ui.Ago(d.UpdatedAt, now)+" ago"))

	stats := []string{
		st.title.Render(d.HeadRef) + st.sep.Render(" → ") + st.title.Render(d.BaseRef),
		st.added.Render("+"+strconv.Itoa(d.Additions)) + " " + st.deleted.Render("−"+strconv.Itoa(d.Deletions)),
		st.author.Render(plural(d.ChangedFiles, "file")),
	}
	if r := st.reviewText(d.ReviewDecision); r != "" {
		stats = append(stats, r)
	}
	line(strings.Join(stats, dot))

	if c := st.checksSummary(d); c != "" {
		line(c)
	}
	if len(d.Labels) > 0 {
		names := make([]string, 0, len(d.Labels))
		for _, l := range d.Labels {
			names = append(names, st.label.Render(l.Name))
		}
		line(strings.Join(names, "  "))
	}
	line(st.rule.Render(strings.Repeat("─", inner)))
	return strings.Join(lines, "\n")
}

func plural(n int, noun string) string {
	s := strconv.Itoa(n) + " " + noun
	if n != 1 {
		s += "s"
	}
	return s
}

// renderComment renders a comment as its author and age over its body,
// wrapped to width behind a bar.
func (m *detailModal) renderComment(c core.Comment, width int) string {
	st := &m.st
	var b strings.Builder
	b.WriteString(gutter + st.commenter.Render(c.Author.Login) + st.age.Render(" · "+ui.Ago(c.CreatedAt, m.now())))
	body := strings.TrimSpace(strings.ReplaceAll(c.Body, "\r\n", "\n"))
	bar := gutter + st.bar
	for l := range strings.SplitSeq(ansi.Wrap(body, max(width-len(gutter)-2, 1), ""), "\n") {
		b.WriteString("\n" + bar + st.title.Render(strings.TrimRight(l, " ")))
	}
	return b.String()
}
