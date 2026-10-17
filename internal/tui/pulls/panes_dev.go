package pulls

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/dev"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/foldtree"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// devPane is the Development pane of a modal: the tree of what the pull
// request links, and the service it is read from.
type devPane struct {
	svc   dev.Service
	model dev.Model
}

// devLinksMsg is what a read of the links of a pull request brought.
type devLinksMsg struct {
	id    int64
	links core.PullLinks
	err   error
}

// devRows is the number of rows of the Development pane with every fold
// open, which the layout sizes the pane to: 0 until the links are read.
func (m *panesModal) devRows() int {
	if m.dev == nil {
		return 0
	}
	return m.dev.model.Rows()
}

// devSlot is the Development pane. Without a service, the modal draws the
// placeholder in its place.
func devSlot() slot {
	return slot{
		context: dev.Context,
		content: (*panesModal).devContent,
		press:   (*panesModal).pressDev,
		preview: (*panesModal).placeholderPreview,
	}
}

// openDev gives the modal its Development pane, with the links the cache
// has, and returns the read that brings the current ones. It returns nil
// when the section has no service.
func (m *panesModal) openDev(svc dev.Service, keys config.Keymap) tea.Cmd {
	// The keys are read even without a service, so that the help lists them.
	km := dev.NewKeyMap(keys)
	if svc == nil {
		return nil
	}
	p := &devPane{svc: svc, model: dev.New(dev.Options{Repo: m.repo, Styles: dev.NewStyles(m.theme, m.icons)})}
	p.model.SetKeyMap(km)
	if links, ok := svc.CachedLinks(m.devQuery()); ok {
		p.model.SetLinks(links)
	}
	m.dev = p
	return m.readLinks()
}

func (m *panesModal) devQuery() pulls.LinksQuery {
	return pulls.LinksQuery{Repo: m.repo, Number: m.number}
}

// readLinks reads the links of the pull request, which a message of the
// modal then brings.
func (m *panesModal) readLinks() tea.Cmd {
	svc, ctx, id, q := m.dev.svc, m.ctx, m.id, m.devQuery()
	return func() tea.Msg {
		links, err := svc.Links(ctx, q)
		return devLinksMsg{id: id, links: links, err: err}
	}
}

// devVersion changes when what the Development pane draws does.
func (m *panesModal) devVersion() int {
	if m.dev == nil {
		return 0
	}
	return m.dev.model.Version()
}

// restyleDev draws the pane again after a change of theme or icons.
func (m *panesModal) restyleDev() {
	if m.dev != nil {
		m.dev.model.SetStyles(dev.NewStyles(m.theme, m.icons))
	}
}

// receiveDev takes the links that a read brought. A read that failed keeps
// the links shown, and says why only when there are none.
func (m *panesModal) receiveDev(msg devLinksMsg) {
	if m.dev == nil || msg.id != m.id {
		return
	}
	if msg.err != nil {
		if m.ctx.Err() == nil {
			m.dev.model.SetError(msg.err)
		}
		return
	}
	m.dev.model.SetLinks(msg.links)
	m.follow()
}

// refreshDev shows the links the cache holds now, such as those served kept
// before GitHub answered.
func (m *panesModal) refreshDev() {
	if m.dev == nil {
		return
	}
	if links, ok := m.dev.svc.CachedLinks(m.devQuery()); ok {
		m.dev.model.SetLinks(links)
		m.follow()
	}
}

// devContent is what the Development pane shows in box b.
func (m *panesModal) devContent(b panes.Box) panes.Content {
	if m.dev == nil {
		return m.placeholder(b)
	}
	in := b.Rect.Inner()
	m.dev.model.SetSize(in.W, in.H)
	if m.focus == b.Pane && m.sub[b.Pane-1] == panes.List {
		m.dev.model.Focus()
	} else {
		m.dev.model.Blur()
	}
	c := panes.Content{Body: m.dev.model.View()}
	if n := m.dev.model.Closes(); n > 0 {
		c.Info = []string{fmt.Sprintf("%d closes", n)}
	}
	return c
}

// pressDev gives a key to the tree of the Development pane, which the list
// of the pane has the focus for. It reports whether the pane had a tree to
// take it.
func (m *panesModal) pressDev(k tea.KeyPressMsg) (tea.Cmd, bool) {
	if m.dev == nil || !m.dev.model.Loaded() {
		return nil, false
	}
	m.dev.model.Focus()
	var cmd tea.Cmd
	m.dev.model, cmd = m.dev.model.Update(k)
	return cmd, true
}

// enterDev opens the issue or pull request of the row that enter reached,
// in place of the modal, which the back key returns to. A link that can't be
// read says why instead. The rows of a group, of more and of none do nothing
// here: the tree folds a group itself.
func (m *panesModal) enterDev(msg foldtree.EnterMsg) tea.Cmd {
	if m.dev == nil || msg.Tree != m.dev.model.ID() {
		return nil
	}
	it, ok := m.dev.model.Item(msg.ID)
	if !ok {
		return nil
	}
	switch it.Kind {
	case dev.Issue, dev.Blocker, dev.Pull:
	default:
		return nil
	}
	r := it.Ref
	if r.Problem != "" {
		return ui.Notify(toast.Warning, "Can't read "+r.Target.String()+": "+r.Problem+".")
	}
	if r.Target.Kind == core.KindPull {
		return func() tea.Msg { return ui.OpenPullMsg{Repo: r.Target.Repo, Number: r.Target.Number, Back: m} }
	}
	return func() tea.Msg { return ui.OpenIssueMsg{Repo: r.Target.Repo, Number: r.Target.Number, Back: m} }
}
