package issues

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/prompt"
)

// composing is what the prompt under the thread of a modal is for.
type composing int

const (
	composeNone composing = iota
	composeComment
	composeEdit
)

// Heights of the comment prompt, which takes a third of the modal within
// these bounds.
const (
	minCommentHeight = 5
	maxCommentHeight = 10
)

// compose opens the prompt for what under the thread, and gives it the
// focus, if the viewer may take a, what the prompt sends.
func (m *detailModal) compose(what composing, a ui.Action) tea.Cmd {
	if !m.loaded {
		return nil
	}
	if cmd, refused := m.gate().Refuse(a, &m.issue); refused {
		return cmd
	}
	num := "#" + strconv.Itoa(m.number)
	opts := []prompt.Option{prompt.WithStyles(m.theme.Prompt(m.icons)), prompt.WithKeyMap(m.keys.prompt)}
	switch what {
	case composeComment:
		opts = append(opts,
			prompt.WithTitle("Comment on "+num),
			prompt.WithPlaceholder("Write a comment in markdown."))
	case composeEdit:
		opts = append(opts,
			prompt.WithTitle("Edit your comment on "+num),
			prompt.WithValue(m.editFrom))
	default:
		return nil
	}
	m.prompt = prompt.New(opts...)
	m.composing = what
	m.layout()
	if m.convOn {
		m.conv.Blur()
	}
	return m.prompt.Focus()
}

// HasDraft reports whether a comment is being written that the modal would
// lose if it closed: the prompt keeps no draft.
func (m *detailModal) HasDraft() bool {
	if e, ok := m.over.(*prpanes.DescEdit); ok && e.Changed(m.issue.Title, m.issue.Body) {
		return true
	}
	switch m.composing {
	case composeComment:
		return strings.TrimSpace(m.prompt.Value()) != ""
	case composeEdit:
		return !sameText(m.prompt.Value(), m.editFrom)
	case composeNone:
	}
	return false
}

// closePrompt closes the prompt and gives the focus back to the thread.
func (m *detailModal) closePrompt() {
	if m.composing == composeNone {
		return
	}
	m.composing = composeNone
	m.prompt.Blur()
	m.layout()
	if m.convOn {
		m.conv.Focus()
	}
}

// promptHeight is the height of the prompt, or 0 while none is open.
func (m *detailModal) promptHeight() int {
	h := m.height
	switch m.composing {
	case composeComment, composeEdit:
		return min(max(h/3, minCommentHeight), maxCommentHeight, h)
	case composeNone:
	}
	return 0
}

// layout shares the height of the modal between the header, the conversation
// and the prompt, if it is open.
func (m *detailModal) layout() {
	if ph := m.promptHeight(); ph > 0 {
		m.prompt.SetSize(m.width, ph)
	}
	if m.convOn {
		r := m.convRect()
		m.conv.SetSize(r.W, r.H)
	}
}

// promptDone handles what the prompt reports, if it is this modal's. A
// submit asks the user to confirm what it sends, and the prompt stays
// open with the text behind the question, so that a no goes back to it.
func (m *detailModal) promptDone(msg tea.Msg) tea.Cmd {
	if m.composing == composeNone {
		return nil
	}
	switch msg := msg.(type) {
	case prompt.CancelMsg:
		if msg.ID == m.prompt.ID() {
			m.closePrompt()
		}
	case prompt.SubmitMsg:
		// A second submit, such as a repeated key, arrives while the first
		// one asks.
		if msg.ID != m.prompt.ID() || m.ask != nil {
			return nil
		}
		switch m.composing {
		case composeComment:
			return m.submitComment(msg.Value)
		case composeEdit:
			return m.submitEdit(msg.Value)
		case composeNone:
		}
	}
	return nil
}

// submitComment asks to post typed as a comment on the issue. An empty
// comment asks nothing, and the prompt stays open.
func (m *detailModal) submitComment(typed string) tea.Cmd {
	body := strings.TrimSpace(typed)
	c, ok, refusal := m.comment(body)
	if !ok {
		return refusal
	}
	ask := ui.Recheck(c.Question, "#"+strconv.Itoa(m.number), func() (confirm.Ask, bool, tea.Cmd) {
		if m.composing != composeComment || strings.TrimSpace(m.prompt.Value()) != body {
			return confirm.Ask{}, false, nil
		}
		return m.comment(body)
	})
	return m.asks(ask)
}

// comment returns posting body as a comment on the issue as a question,
// or ok unset when there is nothing to post or the viewer may not, which
// refusal then says. Its run closes the prompt, and the service shows the
// comment in the cache at once, as sending, so the thread reloads to show
// it.
func (m *detailModal) comment(body string) (c confirm.Ask, ok bool, refusal tea.Cmd) {
	if body == "" || !m.loaded {
		return confirm.Ask{}, false, nil
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.issue); refused {
		return confirm.Ask{}, false, cmd
	}
	n := m.number
	num := "#" + strconv.Itoa(n)
	return confirm.Ask{
		Question: "Post this comment on " + num + "?",
		Run: func() tea.Cmd {
			m.closePrompt()
			op := m.svc.Comment(m.repo, n, body)
			m.addPending(body)
			return tea.Batch(m.reload(), m.buildConv(), m.changed(),
				m.send(op, "comment on "+num))
		},
	}, true, nil
}

// addPending shows body as a comment being sent, which the conversation
// draws last until GitHub answers.
func (m *detailModal) addPending(body string) {
	who := m.viewer
	if who == "" {
		who = "you"
	}
	now := m.now()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent++
	m.pending = append(m.pending, core.TimelineItem{
		Kind: core.TimelineComment, ID: "pending:" + strconv.Itoa(m.sent),
		Actor: core.User{Login: who}, At: now, UpdatedAt: now, Body: body,
	})
}
