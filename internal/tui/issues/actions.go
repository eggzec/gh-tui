package issues

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/confirm"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// deleteComment asks to delete the comment of the viewer under the cursor of
// the Conversation pane. Only your own comment can go.
func (m *detailModal) deleteComment() tea.Cmd {
	s, ok := m.conv.Current()
	if !ok || s.Kind != conv.KindComment || s.Item == nil {
		return ui.Notify(toast.Info, "select your comment to delete")
	}
	if m.viewer == "" || s.Item.Actor.Login != m.viewer {
		return ui.Notify(toast.Info, "you can only delete your own comments")
	}
	id := s.Item.ID
	return m.asks(confirm.Ask{
		Question: "Delete your comment?",
		Run: func() tea.Cmd {
			op := m.svc.DeleteComment(m.repo, m.number, id)
			return tea.Batch(m.buildConv(), m.changed(), m.send(op, "delete comment"))
		},
	})
}
