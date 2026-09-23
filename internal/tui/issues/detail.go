package issues

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/thread"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// issueMsg carries the issue that Get read for the detail opened as gen.
type issueMsg struct {
	gen   int
	issue core.Issue
	err   error
}

// open shows the selected issue with its comments.
func (s *Section) open() tea.Cmd {
	it, ok := s.list.Selected()
	if !ok {
		return nil
	}
	if cached, ok := s.svc.CachedGet(s.repo, it.Number); ok {
		it = cached
	}
	s.issue = it
	s.inDetail = true
	s.detailGen++
	s.detail = s.newThread(it)
	s.list.Blur()
	if s.focused {
		s.detail.Focus()
	}
	s.renderChrome()
	return tea.Batch(s.detail.SetDocument(s.header(it), it.Body), s.get())
}

// back returns to the list, which kept its selection.
func (s *Section) back() {
	if !s.inDetail {
		return
	}
	s.closePrompt()
	s.inDetail = false
	s.cancelDetail()
	s.detail.Blur()
	if s.focused {
		s.list.Focus()
	}
	s.renderChrome()
}

// newThread returns the thread of issue it, sized and styled like the
// section. Its fetches stop when the section goes back to the list.
func (s *Section) newThread(it core.Issue) thread.Model[core.Comment] {
	s.detailCtx, s.cancelDetail = context.WithCancel(s.ctx)
	ctx := s.detailCtx
	svc, q := s.svc, issuesvc.CommentsQuery{Repo: s.repo, Number: it.Number}
	fetch := func(ctx context.Context, cursor string) ([]core.Comment, string, error) {
		q := q
		q.Cursor = cursor
		p, err := svc.Comments(ctx, q)
		return p.Items, p.Next, err
	}
	return thread.New(fetch, s.renderComment,
		thread.WithContext(ctx),
		thread.WithKeyMap(s.keys.thread),
		thread.WithStyles(s.theme.Thread()),
		thread.WithSize(s.width, s.bodyHeight()),
	)
}

// get reads the issue for the open detail, since a list page may carry less
// than the issue itself, such as no body.
func (s *Section) get() tea.Cmd {
	svc, repo, number, gen, ctx := s.svc, s.repo, s.issue.Number, s.detailGen, s.detailCtx
	return func() tea.Msg {
		it, err := svc.Get(ctx, repo, number)
		return issueMsg{gen: gen, issue: it, err: err}
	}
}

// gotIssue shows the issue that Get read, if its detail is still open.
func (s *Section) gotIssue(msg issueMsg) tea.Cmd {
	if !s.inDetail || msg.gen != s.detailGen {
		return nil
	}
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			return nil
		}
		return ui.Notify(toast.Error, "Couldn't load #"+strconv.Itoa(s.issue.Number)+": "+msg.err.Error())
	}
	s.issue = msg.issue
	return s.detail.SetDocument(s.header(msg.issue), msg.issue.Body)
}

// header renders the head of the detail: the title, the state, who opened
// it and when, the counts, the labels and the assignees.
func (s *Section) header(it core.Issue) string {
	t := s.theme
	now := s.now()
	dot := t.Subtle.Render(" · ")
	var b strings.Builder

	b.WriteString("  ")
	b.WriteString(t.Title.Render(clean(it.Title)))
	b.WriteString(t.Muted.Render("  #" + strconv.Itoa(it.Number)))
	b.WriteString("\n  ")

	if it.State == core.StateOpen {
		b.WriteString(s.rows.openBadge)
	} else {
		b.WriteString(s.rows.closedBadge)
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
			b.WriteString(s.chip(l).text)
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
func (s *Section) renderComment(c core.Comment, width int) string {
	t := s.theme
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
	b.WriteString(t.Subtle.Render(" · " + ago(c.CreatedAt, s.now())))
	b.WriteByte('\n')
	b.WriteString(s.markdown(c.Body, width))
	return b.String()
}

// markdown renders a comment body at width with the thread's markdown
// style. The renderer is kept until the width or the theme changes.
func (s *Section) markdown(body string, width int) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	if s.md == nil || s.mdWidth != width {
		md, err := glamour.NewTermRenderer(
			glamour.WithStyles(s.theme.Thread().Markdown),
			glamour.WithWordWrap(width),
		)
		if err != nil {
			return body
		}
		s.md, s.mdWidth = md, width
	}
	out, err := s.md.Render(body)
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
