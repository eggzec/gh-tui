package issues

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// openDescEdit opens the editor of the title and the description over the
// panes, if the viewer may change them.
func (m *detailModal) openDescEdit() tea.Cmd {
	if !m.loaded {
		return ui.Notify(toast.Info, "Still reading #"+strconv.Itoa(m.number)+".")
	}
	if cmd, refused := m.gate().Refuse(ui.ActDraft, &m.issue); refused {
		return cmd
	}
	o := prpanes.NewDescEdit("Edit #"+strconv.Itoa(m.number), "issue", ctxDescEdit,
		m.keys.descEdit, m.theme.Prompt(m.icons), m.issue.Title, m.issue.Body)
	o.Send = m.saveDescEdit
	m.over = o
	return o.Init()
}

// saveDescEdit sends the title and the description as they are. The service
// shows them at once, so the header and the pane do too.
func (m *detailModal) saveDescEdit(title, body string) (tea.Cmd, bool) {
	if cmd, refused := m.gate().Refuse(ui.ActDraft, &m.issue); refused {
		return cmd, false
	}
	op := m.svc.EditIssue(m.repo, m.number, title, body)
	return tea.Batch(m.reload(), m.changed(), m.send(op, "edit #"+strconv.Itoa(m.number))), true
}
