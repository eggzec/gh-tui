package pulls

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
)

// Heights of the composer of a comment, which takes a third of the modal
// within these bounds.
const (
	minCommentHeight = 5
	maxCommentHeight = 10
)

// commentedMsg reports that GitHub answered the comment sent by the modal's
// thread, which then reads the comments again, since a refusal took the
// pending one out.
type commentedMsg struct{ thread int64 }

// draftKey is the key of the draft of a comment on this pull request: the
// host and account first, so that no account reads another's.
func (m *detailModal) draftKey() string {
	return strings.Join([]string{m.draftScope, m.repo.String(), strconv.Itoa(m.number), "comment"}, "/")
}

// startComment opens the composer of a comment under the thread and gives it
// the focus, if the viewer may comment. The draft of an earlier try is loaded
// into it.
func (m *detailModal) startComment() tea.Cmd {
	if !m.loaded || m.comp != nil {
		return nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.detail.Issue); refused {
		return cmd
	}
	opts := []composer.Option{
		composer.WithTitle("Comment on #" + strconv.Itoa(m.number)),
		composer.WithPlaceholder("Write a comment in markdown."),
		composer.WithKeyMap(m.keys.composer),
		composer.WithStyles(m.theme.Prompt(m.icons)),
	}
	if m.drafts != nil {
		opts = append(opts, composer.WithDrafts(m.drafts))
	}
	c := composer.New(m.draftKey(), opts...)
	m.comp = &c
	m.layout()
	m.thread.Blur()
	return tea.Batch(c.Init(), m.comp.Focus())
}

// saveComment keeps the text of the open composer of a comment as its draft,
// so that the next one on this pull request starts from it. Without a
// composer, or drafts, there is nothing to keep.
func (m *detailModal) saveComment() error {
	if m.comp == nil || m.drafts == nil {
		return nil
	}
	return m.comp.Save()
}

// closeComposer closes the composer and gives the focus back to the thread.
// The draft stays where it is.
func (m *detailModal) closeComposer() {
	if m.comp == nil {
		return
	}
	m.comp.Blur()
	m.comp = nil
	m.layout()
	m.thread.Focus()
}

// layout shares the height of the modal between the thread and the composer,
// if it is open.
func (m *detailModal) layout() {
	h := m.height
	ch := 0
	if m.comp != nil {
		ch = min(max(h/3, minCommentHeight), maxCommentHeight, h)
		m.comp.SetSize(m.width, ch)
	}
	m.thread.SetSize(m.width, h-ch)
}

// composed renders the thread, and the composer under it while one is open.
func (m *detailModal) composed() string {
	if m.comp == nil {
		return m.thread.View()
	}
	if m.thread.Height() == 0 {
		return m.comp.View()
	}
	return m.thread.View() + "\n" + m.comp.View()
}

// updateComposer takes what the composer reports, if it is this modal's. A
// submit asks the user to confirm what it sends, and the composer stays open
// with the text behind the question, so that a no goes back to it.
func (m *detailModal) updateComposer(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case composer.CancelMsg:
		if m.comp != nil && msg.ID == m.comp.ID() {
			m.closeComposer()
		}
	case composer.SubmitMsg:
		// A second submit, such as a repeated key, arrives while the first
		// one asks.
		if m.comp == nil || msg.ID != m.comp.ID() || m.ask != nil {
			return nil
		}
		return m.submitComment(msg.Text)
	}
	return nil
}

// submitComment asks to post typed as a comment on the pull request.
func (m *detailModal) submitComment(typed string) tea.Cmd {
	body := strings.TrimSpace(typed)
	c, ok := m.postRequest(body)
	if !ok {
		return nil
	}
	ask := ui.Recheck(c.Question, "#"+strconv.Itoa(m.number), func() (ui.Confirm, bool, tea.Cmd) {
		if m.comp == nil || strings.TrimSpace(m.comp.Value()) != body {
			return ui.Confirm{}, false, nil
		}
		c, ok := m.postRequest(body)
		if !ok {
			return ui.Confirm{}, false, nil
		}
		if cmd, refused := m.gate().Refuse(ui.ActComment, &m.detail.Issue); refused {
			return ui.Confirm{}, false, cmd
		}
		return c, true, nil
	})
	m.ask = &ask
	return nil
}

// postRequest returns posting body as a comment on the pull request as a
// question, or ok unset when there is nothing to post. Its run closes the
// composer, and the service shows the comment at once, as sending, so the
// thread reads the comments again to show it. Once GitHub answers, the draft
// goes, unless it refused: then the comment is taken out again and the draft
// stays for another try.
func (m *detailModal) postRequest(body string) (c ui.Confirm, ok bool) {
	if body == "" || !m.loaded || m.comp == nil {
		return ui.Confirm{}, false
	}
	n := m.number
	num := "#" + strconv.Itoa(n)
	return ui.Confirm{
		Question: "Post this comment on " + num + "?",
		Run: func() tea.Cmd {
			discard := m.comp.Discard()
			m.closeComposer()
			op := m.svc.Comment(m.repo, n, body, m.viewer)
			do := ui.Do(m.sendCtx, ui.PullsTitle, ui.About(m.subject(), op), "comment on "+num)
			from := m.thread.ID()
			send := func() tea.Msg {
				msg := do()
				if done, _ := msg.(ui.DoneMsg); done.Err == nil {
					_ = discard()
				}
				return tea.BatchMsg{func() tea.Msg { return msg }, func() tea.Msg { return commentedMsg{thread: from} }}
			}
			return tea.Batch(m.thread.Reload(), send)
		},
	}, true
}
