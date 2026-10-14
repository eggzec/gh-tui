package pulls

import (
	"log/slog"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Heights of the composer of a comment, which takes a third of the modal
// within these bounds.
const (
	minCommentHeight = 5
	maxCommentHeight = 10
)

// commentedMsg reports that GitHub answered the comment sent by the modal's
// thread, which then reads the comments again, since a refusal took the
// pending one out. It also settles the text: a sent one's draft goes, and a
// refused one comes back.
type commentedMsg struct {
	thread int64
	// key and body are the draft's key and the text that was posted.
	key, body string
	// failed is set when the comment did not go through.
	failed bool
}

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
	return m.openComment()
}

// openComment opens the composer of a comment. Text that GitHub refused and
// no draft holds is put in it, ahead of the draft, which then doesn't
// replace it.
func (m *detailModal) openComment() tea.Cmd {
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
	if m.unsent != "" {
		c.SetValue(m.unsent)
		m.unsent = ""
	}
	m.comp = &c
	m.layout()
	m.thread.Blur()
	return tea.Batch(c.Init(), m.comp.Focus())
}

// saveComment keeps the text of the open composer of a comment as its draft,
// so that the next one on this pull request starts from it. Without a
// composer, or drafts, there is nothing to keep. A blank composer is left
// alone: the draft may not have reached it yet, and saving the blank would
// delete the draft. Clearing the text on purpose is kept by the composer's own
// cancel.
func (m *detailModal) saveComment() error {
	if m.comp == nil || m.drafts == nil || strings.TrimSpace(m.comp.Value()) == "" {
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
// with the text behind the question, so that a no goes back to it. A draft
// that could not be saved is warned of, and the text stays in the composer.
func (m *detailModal) updateComposer(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case composer.CancelMsg:
		if m.comp != nil && msg.ID == m.comp.ID() {
			m.closeComposer()
			return m.draftWarning(msg.Err)
		}
	case composer.SubmitMsg:
		// A second submit, such as a repeated key, arrives while the first
		// one asks.
		if m.comp == nil || msg.ID != m.comp.ID() || m.ask != nil {
			return nil
		}
		m.submitComment(msg.Text)
		return m.draftWarning(msg.Err)
	}
	return nil
}

// draftWarning warns that the draft of the comment could not be saved, if err
// says so.
func (m *detailModal) draftWarning(err error) tea.Cmd {
	if err == nil {
		return nil
	}
	slog.WarnContext(m.ctx, "comment draft not saved", "span", "tui", "err", ui.ShortPath(err.Error()))
	return ui.Notify(toast.Warning, "Couldn't save the draft of the comment.")
}

// commented settles the text of a comment once GitHub answered. A comment
// that went through removes its draft, unless the draft is another text by
// now, and clears the composer that was opened meanwhile if it still holds
// what was sent, as it loaded it from that draft. A refused one comes back:
// into a new composer when it is safe to open one, else it waits for the next.
func (m *detailModal) commented(msg commentedMsg) tea.Cmd {
	if !msg.failed {
		if m.comp != nil && m.comp.Key() == msg.key && strings.TrimSpace(m.comp.Value()) == msg.body {
			m.comp.SetValue("")
		}
		if m.drafts == nil {
			return nil
		}
		text, err := m.drafts.Load(msg.key)
		if err == nil && strings.TrimSpace(text) == msg.body {
			err = m.drafts.Delete(msg.key)
		}
		if err != nil {
			slog.WarnContext(m.ctx, "comment draft not removed", "span", "tui", "err", ui.ShortPath(err.Error()))
		}
		return nil
	}
	switch {
	case m.comp != nil && strings.TrimSpace(m.comp.Value()) == msg.body:
		// The composer opened meanwhile loaded the draft, which is this text.
		return nil
	case m.comp != nil && strings.TrimSpace(m.comp.Value()) == "":
		m.comp.SetValue(msg.body)
		return nil
	case m.comp == nil && m.onConversation() && m.tray.comp == nil && m.find == nil && m.ask == nil:
		m.unsent = msg.body
		return m.openComment()
	}
	m.unsent = msg.body
	return ui.Notify(toast.Warning, "The comment was not posted; open the composer to get its text back.")
}

// submitComment asks to post typed as a comment on the pull request.
func (m *detailModal) submitComment(typed string) {
	body := strings.TrimSpace(typed)
	c, ok := m.postRequest(body)
	if !ok {
		return
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
}

// postRequest returns posting body as a comment on the pull request as a
// question, or ok unset when there is nothing to post. Its run closes the
// composer, and the service shows the comment at once, as sending, so the
// thread reads the comments again to show it. Once GitHub answers, the draft
// goes, unless it refused: then the comment is taken out again and its text
// comes back (see [detailModal.commented]).
func (m *detailModal) postRequest(body string) (c ui.Confirm, ok bool) {
	if body == "" || !m.loaded || m.comp == nil {
		return ui.Confirm{}, false
	}
	n := m.number
	num := "#" + strconv.Itoa(n)
	return ui.Confirm{
		Question: "Post this comment on " + num + "?",
		Run: func() tea.Cmd {
			key := m.comp.Key()
			m.closeComposer()
			op := m.svc.Comment(m.repo, n, body, m.viewer)
			do := ui.Do(m.sendCtx, ui.PullsTitle, ui.About(m.subject(), op), "comment on "+num)
			from := m.thread.ID()
			send := func() tea.Msg {
				msg := do()
				done, _ := msg.(ui.DoneMsg)
				sent := commentedMsg{thread: from, key: key, body: body, failed: done.Err != nil}
				return tea.BatchMsg{func() tea.Msg { return msg }, func() tea.Msg { return sent }}
			}
			return tea.Batch(m.thread.Reload(), send)
		},
	}, true
}
