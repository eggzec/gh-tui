package pulls

import (
	"context"
	"strconv"
	"strings"

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

// open shows pr with its comments. The list item is shown at once, and the
// detail with its body replaces it when it arrives.
func (s *Section) open(pr core.PullRequest) tea.Cmd {
	s.closeDetail()
	ctx, cancel := context.WithCancel(s.ctx)
	svc, repo, number := s.svc, s.repo, pr.Number
	fetch := func(ctx context.Context, cursor string) ([]core.Comment, string, error) {
		p, err := svc.Comments(ctx, pulls.CommentsQuery{Repo: repo, Number: number, Cursor: cursor})
		return p.Items, p.Next, err
	}
	t := thread.New(fetch, s.renderComment,
		thread.WithContext(ctx),
		thread.WithSize(s.width, s.height),
		thread.WithStyles(s.theme.Thread()),
		thread.WithKeyMap(s.keys.thread),
		thread.WithFocused(s.focused),
	)
	s.thread, s.detailCtx, s.cancelDetail = &t, ctx, cancel
	s.detail = core.PullRequestDetail{PullRequest: pr}
	if d, ok := svc.CachedGet(repo, number); ok {
		s.detail = d
	}
	s.feed.Blur()
	return tea.Batch(s.showDetail(), s.get())
}

// closeDetail goes back to the list, whose selection is where it was, and
// cancels what the detail was loading.
func (s *Section) closeDetail() {
	if s.thread == nil {
		return
	}
	s.cancelDetail()
	s.thread, s.detailCtx, s.cancelDetail = nil, nil, nil
	if s.focused && s.feed != nil {
		s.feed.Focus()
	}
}

// get fetches the detail of the open pull request. A fresh cached detail
// costs no request.
func (s *Section) get() tea.Cmd {
	svc, ctx, repo, number, id := s.svc, s.detailCtx, s.repo, s.detail.Number, s.thread.ID()
	return func() tea.Msg {
		d, err := svc.Get(ctx, repo, number)
		return detailMsg{thread: id, detail: d, err: err}
	}
}

func (s *Section) receive(msg detailMsg) tea.Cmd {
	if s.thread == nil || msg.thread != s.thread.ID() {
		return nil
	}
	if msg.err != nil {
		if s.detailCtx.Err() != nil {
			return nil
		}
		return ui.Notify(toast.Error, "Couldn't load #"+strconv.Itoa(s.detail.Number)+": "+msg.err.Error())
	}
	s.detail = msg.detail
	return s.showDetail()
}

// showDetail sets the document of the thread from the detail.
func (s *Section) showDetail() tea.Cmd {
	return s.thread.SetDocument(s.detailHeader(s.width), s.detail.Body)
}

// detailHeader renders the head of the open pull request at width: the
// title, its state, author and age, the refs and stats, the checks and the
// labels, over a rule.
func (s *Section) detailHeader(width int) string {
	d, st := &s.detail, &s.st
	inner := max(width-len(gutter), 1)
	now := s.now()
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
func (s *Section) renderComment(c core.Comment, width int) string {
	st := &s.st
	var b strings.Builder
	b.WriteString(gutter + st.commenter.Render(c.Author.Login) + st.age.Render(" · "+ui.Ago(c.CreatedAt, s.now())))
	body := strings.TrimSpace(strings.ReplaceAll(c.Body, "\r\n", "\n"))
	bar := gutter + st.bar
	for l := range strings.SplitSeq(ansi.Wrap(body, max(width-len(gutter)-2, 1), ""), "\n") {
		b.WriteString("\n" + bar + st.title.Render(strings.TrimRight(l, " ")))
	}
	return b.String()
}
