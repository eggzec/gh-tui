package pulls

import (
	"fmt"
	"time"

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
	svc    dev.Service
	model  dev.Model
	styles dev.Styles
	// hov is the row that the preview follows, which the last row with
	// something to preview stays.
	hov dev.Item
	// issues and pulls are the previews read, by the ID of the hovered row.
	issues map[string]core.IssuePreview
	pulls  map[string]core.PullPreview
	// loading holds the previews with a read out, and failed those whose read
	// failed, which are read again once GitHub answers.
	loading map[string]bool
	failed  map[string]bool
	// drawn is the text of the previews, by ID and width, until the pane or a
	// preview changes.
	drawn    map[drawnPreview]string
	drawnVer int
	// reads counts the changes of what the previews show.
	reads int
}

// drawnPreview names the text of a preview at a width.
type drawnPreview struct {
	id    string
	width int
}

// devPreviewMsg is what a read of the preview of an item brought.
type devPreviewMsg struct {
	id    int64
	key   string
	pull  bool
	issue core.IssuePreview
	prev  core.PullPreview
	err   error
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
		hovered: (*panesModal).hoveredDev,
		preview: (*panesModal).devPreview,
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
	st := dev.NewStyles(m.theme, m.icons)
	p := &devPane{
		svc: svc, model: dev.New(dev.Options{Repo: m.repo, Styles: st}), styles: st,
		issues: map[string]core.IssuePreview{}, pulls: map[string]core.PullPreview{},
		loading: map[string]bool{}, failed: map[string]bool{}, drawn: map[drawnPreview]string{},
	}
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
	return m.dev.model.Version() + m.dev.reads
}

// restyleDev draws the pane again after a change of theme or icons.
func (m *panesModal) restyleDev() {
	if m.dev != nil {
		m.dev.styles = dev.NewStyles(m.theme, m.icons)
		m.dev.model.SetStyles(m.dev.styles)
	}
}

// receiveDev takes the links that a read brought. A read that failed keeps
// the links shown, and says why only when there are none.
func (m *panesModal) receiveDev(msg devLinksMsg) tea.Cmd {
	if m.dev == nil || msg.id != m.id {
		return nil
	}
	if msg.err != nil {
		if m.ctx.Err() == nil {
			m.dev.model.SetError(msg.err)
		}
		return nil
	}
	m.dev.model.SetLinks(msg.links)
	m.follow()
	return m.loadPreview()
}

// refreshDev shows the links the cache holds now, such as those served kept
// before GitHub answered.
func (m *panesModal) refreshDev() {
	if m.dev == nil {
		return
	}
	clear(m.dev.failed)
	if links, ok := m.dev.svc.CachedLinks(m.devQuery()); ok {
		m.dev.model.SetLinks(links)
		m.follow()
	}
}

// hoveredDev names the row of the tree for the preview.
func (m *panesModal) hoveredDev() (id, title string, ok bool) {
	if m.dev == nil || !m.dev.model.Loaded() {
		return "", "", false
	}
	id, title, it, ok := m.dev.model.Hovered()
	if ok {
		m.dev.hov = it
	}
	return id, title, ok
}

// loadPreview returns the read of the preview of the hovered row, when the
// pane has the focus and shows a preview, and neither holds nor is reading it.
// A preview the service has cached needs no read.
func (m *panesModal) loadPreview() tea.Cmd {
	p := m.dev
	if p == nil || m.focus != panes.Development {
		return nil
	}
	if _, ok := m.layout().Box(panes.Development, panes.Preview); !ok {
		return nil
	}
	key := m.hover[panes.Development-1].id
	t := p.hov.Ref.Target
	if p.hov.Ref.Problem != "" || p.loading[key] || p.failed[key] {
		return nil
	}
	q := pulls.PreviewQuery{Repo: t.Repo, Number: t.Number}
	svc, ctx, id := p.svc, m.ctx, m.id
	switch p.hov.Kind {
	case dev.Issue, dev.Blocker:
		if _, ok := p.issues[key]; ok {
			return nil
		}
		if v, ok := svc.CachedIssuePreview(q); ok {
			p.issues[key] = v
			p.reads++
			return nil
		}
		p.loading[key] = true
		return func() tea.Msg {
			v, err := svc.IssuePreview(ctx, q)
			return devPreviewMsg{id: id, key: key, issue: v, err: err}
		}
	case dev.Pull:
		if _, ok := p.pulls[key]; ok {
			return nil
		}
		if v, ok := svc.CachedPullPreview(q); ok {
			p.pulls[key] = v
			p.reads++
			return nil
		}
		p.loading[key] = true
		return func() tea.Msg {
			v, err := svc.PullPreview(ctx, q)
			return devPreviewMsg{id: id, key: key, pull: true, prev: v, err: err}
		}
	case dev.Header, dev.More, dev.None:
	}
	return nil
}

// receivePreview keeps the preview that a read brought, though the cursor has
// moved on since: the preview shown stays where it is.
func (m *panesModal) receivePreview(msg devPreviewMsg) {
	p := m.dev
	if p == nil || msg.id != m.id {
		return
	}
	delete(p.loading, msg.key)
	switch {
	case msg.err != nil:
		if m.ctx.Err() != nil {
			return
		}
		p.failed[msg.key] = true
	case msg.pull:
		p.pulls[msg.key] = msg.prev
	default:
		p.issues[msg.key] = msg.issue
	}
	for k := range p.drawn {
		if k.id == msg.key {
			delete(p.drawn, k)
		}
	}
	p.reads++
}

// devPreview is the body of the preview of the hovered row, drawn once per
// width until what it shows changes.
func (m *panesModal) devPreview(b panes.Box) string {
	p := m.dev
	if p == nil {
		return m.placeholderPreview(b)
	}
	if v := p.model.Version(); v != p.drawnVer {
		clear(p.drawn)
		p.drawnVer = v
	}
	id, w := m.hover[b.Pane-1].id, b.Rect.Inner().W
	o := dev.PreviewOptions{Repo: m.repo, Styles: p.styles, Now: m.clock(), Width: w}
	if id == "" {
		if p.model.Loaded() {
			return dev.Nothing(o)
		}
		return dev.Loading(o)
	}
	key := drawnPreview{id, w}
	if s, ok := p.drawn[key]; ok {
		return s
	}
	var s string
	switch p.hov.Kind {
	case dev.Header:
		s = dev.HeaderPreview(p.hov.Group, p.model.Links(), o)
	case dev.Pull:
		if v, ok := p.pulls[id]; ok {
			s = dev.PullPreview(v, o)
		}
	default:
		if v, ok := p.issues[id]; ok {
			s = dev.IssuePreview(v, o)
		}
	}
	if s == "" {
		if p.failed[id] {
			return dev.Nothing(o)
		}
		return dev.Loading(o)
	}
	p.drawn[key] = s
	return s
}

// clock is the time that ages are told from.
func (m *panesModal) clock() time.Time {
	if m.now == nil {
		return time.Now()
	}
	return m.now()
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
