package pulls

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/composer"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// ctxDescEdit is the context of the keys of the editor of the title and the
// description. The editor types, so they are control keys.
const ctxDescEdit = "pull_desc_edit"

// quote returns body as a markdown quote, ready to be answered under: each
// line behind a "> ", and a blank line after.
func quote(body string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight("> "+l, " ")
	}
	return strings.Join(lines, "\n") + "\n\n"
}

// openDescEdit opens the editor of the title and the description, if the
// viewer may change them.
func (m *panesModal) openDescEdit() tea.Cmd {
	if !m.loaded {
		return ui.Notify(toast.Info, checking(m.icons, m.number))
	}
	if cmd, refused := m.gate().Refuse(ui.ActDraft, &m.detail.Issue); refused {
		return cmd
	}
	o := prpanes.NewDescEdit("Edit #"+strconv.Itoa(m.number), "pull request", ctxDescEdit,
		m.keys.descEdit, m.theme.Prompt(m.icons), m.detail.Title, m.detail.Body)
	o.Send = m.saveDescEdit
	m.open(o)
	return o.Init()
}

// saveDescEdit sends the title and the description as they are. The service
// shows them at once, so the header and the pane do too.
func (m *panesModal) saveDescEdit(title, body string) (tea.Cmd, bool) {
	if cmd, refused := m.gate().Refuse(ui.ActDraft, &m.detail.Issue); refused {
		return cmd, false
	}
	op := m.svc.EditPull(m.repo, m.number, title, body)
	if d, ok := m.svc.CachedGet(m.repo, m.number); ok {
		m.detail = d
	}
	return m.sendOp(op, "edit #"+strconv.Itoa(m.number)), true
}

// quoteDescription opens the composer of a new comment, which starts with the
// description quoted.
func (m *panesModal) quoteDescription() tea.Cmd {
	if !m.loaded {
		return ui.Notify(toast.Info, checking(m.icons, m.number))
	}
	body := strings.TrimSpace(m.detail.Body)
	if body == "" {
		return ui.Notify(toast.Info, "There is no description to quote.")
	}
	return m.openComment(quote(body))
}

// openComment opens the composer of a new comment on the pull request, which
// starts with text.
func (m *panesModal) openComment(text string) tea.Cmd {
	if !m.loaded {
		return ui.Notify(toast.Info, checking(m.icons, m.number))
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.detail.Issue); refused {
		return cmd
	}
	num := "#" + strconv.Itoa(m.number)
	c := composer.New("comment/"+m.repo.String()+"/"+strconv.Itoa(m.number),
		composer.WithTitle("Comment on "+num),
		composer.WithPlaceholder("Write the comment in markdown."),
		composer.WithKeyMap(m.keys.composer),
		composer.WithStyles(m.theme.Prompt(m.icons)),
		composer.WithSize(m.width, m.areaHeight()),
		m.drafts(),
	)
	c.SetValue(text)
	o := m.newComposer(c, "Comment on "+num, 0, m.postComment)
	m.open(o)
	return tea.Batch(o.Comp.Init(), o.Comp.Focus())
}

// postComment adds the comment the composer holds to the conversation. The
// service shows it at once and the modal reads the pull request once GitHub
// answered.
func (m *panesModal) postComment(o *composerOverlay) tea.Cmd {
	body := strings.TrimSpace(o.Comp.Value())
	if body == "" {
		return ui.Notify(toast.Info, "Nothing to send: the comment is empty.")
	}
	if cmd, refused := m.gate().Refuse(ui.ActComment, &m.detail.Issue); refused {
		return cmd
	}
	o.Close()
	op := m.svc.Comment(m.repo, m.number, body, m.conv.viewer)
	// The conversation reads the cache again, which holds the comment now and
	// shows a rollback, and the network once the comment went through, since
	// the service forgets the timeline then.
	send := m.sendConv(op, "comment on #"+strconv.Itoa(m.number), "", true)
	return tea.Batch(m.list.reload(), m.recacheConv(), send)
}

// sendOp sends a change of the pull request, and reads it again once it went
// through or was rolled back.
func (m *panesModal) sendOp(op ui.Op, what string) tea.Cmd {
	about := core.Target{Repo: m.repo, Number: m.number}.String()
	do := ui.Do(m.sendCtx, ui.PullsTitle, ui.About(about, op), what)
	return tea.Batch(m.list.reload(), tea.Sequence(do, m.revalidate()))
}
