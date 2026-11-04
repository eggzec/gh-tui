package issues

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// ownComment returns the comment of the viewer under the cursor of the
// Conversation pane, or the notice that says why there is none.
func (m *detailModal) ownComment(verb string) (*core.TimelineItem, tea.Cmd) {
	s, ok := m.conv.Current()
	if !ok || (s.Kind != conv.KindComment && s.Kind != conv.KindRunComment) || s.Item == nil {
		return nil, ui.Notify(toast.Info, "select your comment to "+verb)
	}
	if m.viewer == "" || s.Item.Actor.Login != m.viewer {
		return nil, ui.Notify(toast.Info, "you can only "+verb+" your own comments")
	}
	return s.Item, nil
}

// editComment opens the prompt with the viewer's comment under the cursor.
func (m *detailModal) editComment() tea.Cmd {
	it, refusal := m.ownComment("edit")
	if it == nil {
		return refusal
	}
	if strings.HasPrefix(it.ID, "pending:") {
		return ui.Notify(toast.Info, "the comment is still being sent")
	}
	m.editID, m.editFrom = it.ID, it.Body
	return m.compose(composeEdit, ui.ActComment)
}

// sameText reports whether two bodies read the same: the prompt widens tabs
// and drops surrounding blank space, which is no edit.
func sameText(a, b string) bool {
	norm := func(s string) string {
		return strings.TrimSpace(strings.ReplaceAll(s, "\t", "    "))
	}
	return norm(a) == norm(b)
}

// submitEdit asks to save typed as the body of the comment being edited.
// An empty or unchanged body asks nothing and closes the prompt.
func (m *detailModal) submitEdit(typed string) tea.Cmd {
	body := strings.TrimSpace(typed)
	if body == "" || sameText(body, m.editFrom) {
		m.closePrompt()
		return nil
	}
	id := m.editID
	return m.asks(confirm.Ask{
		Question: "Save your changes to the comment?",
		Run: func() tea.Cmd {
			m.closePrompt()
			op := m.svc.EditComment(m.repo, m.number, id, body)
			return tea.Batch(m.buildConv(), m.changed(), m.send(op, "edit comment"))
		},
	})
}

// deleteComment asks to delete the comment of the viewer under the cursor of
// the Conversation pane. Only your own comment can go.
func (m *detailModal) deleteComment() tea.Cmd {
	it, refusal := m.ownComment("delete")
	if it == nil {
		return refusal
	}
	id := it.ID
	return m.asks(confirm.Ask{
		Question: "Delete your comment?",
		Run: func() tea.Cmd {
			op := m.svc.DeleteComment(m.repo, m.number, id)
			return tea.Batch(m.buildConv(), m.changed(), m.send(op, "delete comment"))
		},
	})
}
