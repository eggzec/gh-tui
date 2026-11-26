package pulls

import (
	"context"
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	refssvc "github.com/eggzec/gh-tui/internal/service/refs"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/dev"
	"github.com/eggzec/gh-tui/internal/tui/refs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
	"github.com/eggzec/gh-tui/pkg/bubbles/treeview"
)

// DevService is what the Development pane reads. *pulls.Service satisfies it.
type DevService interface {
	dev.PreviewService
	CachedLinks(q pulls.LinksQuery) (core.PullLinks, bool)
	Links(ctx context.Context, q pulls.LinksQuery) (core.PullLinks, error)
}

// devPane is the Development pane of a modal: the tree of what the pull
// request links, and the service it is read from.
type devPane struct {
	svc    DevService
	refSvc refs.Service
	model  dev.Model
	// links is what the pull request closes, refs and mentions what its text
	// names and the first page of what names it.
	links     core.PullLinks
	haveLinks bool
	refs      core.References
	mentions  core.Page[core.Reference]
	styles    dev.Styles
	// hov is the row that the preview follows, which the last row with
	// something to preview stays.
	hov dev.Item
	// previews reads and draws the preview of the hovered row.
	previews *dev.Previews
}

// devRefsMsg is what a read of what a pull request names brought.
type devRefsMsg struct {
	id   int64
	refs core.References
	err  error
}

// devMentionsMsg is what a read of the items that name a pull request brought.
type devMentionsMsg struct {
	id   int64
	page core.Page[core.Reference]
	err  error
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
		Context: dev.Context,
		Content: (*panesModal).devContent,
		Press:   (*panesModal).pressDev,
		Hovered: (*panesModal).hoveredDev,
		Preview: (*panesModal).devPreview,
	}
}

// openDev gives the modal its Development pane, with the links the cache
// has, and returns the read that brings the current ones. It returns nil
// when the section has no service.
func (m *panesModal) openDev(svc DevService, refSvc refs.Service, keys config.Keymap) tea.Cmd {
	// The keys are read even without a service, so that the help lists them.
	km := dev.NewKeyMap(keys)
	if svc == nil {
		return nil
	}
	st := dev.NewStyles(m.theme, m.icons)
	p := &devPane{
		svc: svc, refSvc: refSvc, model: dev.New(dev.Options{Repo: m.repo, Styles: st, HideClosedBy: true}), styles: st,
		previews: dev.NewPreviews(dev.ServiceReader(svc), dev.WithContext(m.ctx)),
	}
	p.model.SetKeyMap(km)
	m.dev = p
	m.cachedDev()
	return tea.Batch(m.readLinks(), m.readRefs(false))
}

// cachedDev shows what the caches hold of the links, the references and the
// mentions.
func (m *panesModal) cachedDev() {
	p := m.dev
	if links, ok := p.svc.CachedLinks(m.devQuery()); ok {
		p.links, p.haveLinks = links, true
	}
	if p.refSvc != nil {
		if pg, ok := p.refSvc.CachedMentions(m.mentionsQuery(false)); ok {
			p.mentions = pg
		}
		if r, ok := p.refSvc.CachedReferences(m.refQuery(false)); ok {
			p.refs = r
		}
	}
	p.showLinks()
}

// showLinks lists what the pull request closes, what its own text names and
// what names it. The pull requests that close the issues it closes are not
// listed, so what other pull requests link stays out.
func (p *devPane) showLinks() {
	if !p.haveLinks && p.refs.Written == nil && p.mentions.Items == nil {
		return
	}
	links := p.links
	links.Linked, links.LinkedCapped = nil, false
	links.Mentions = p.written()
	links.MentionsCapped = p.refs.Unresolved > 0 || p.refs.Failed > 0 || p.refs.CommentsRead < p.refs.CommentsTotal
	links.Mentioned = p.mentions.Items
	links.MentionedTotal = p.refs.Mentioned
	links.MentionedCapped = p.mentions.Next != ""
	p.model.SetLinks(links)
}

// written are the items that the body and the comments of the pull request
// name, those that could be read.
func (p *devPane) written() []core.Reference {
	var out []core.Reference
	for i := range p.refs.Written {
		if p.refs.Written[i].Problem == "" {
			out = append(out, p.refs.Written[i])
		}
	}
	return out
}

func (m *panesModal) refQuery(again bool) refssvc.Query {
	return refssvc.Query{Repo: m.repo, Number: m.number, Pull: true, Updated: m.detail.UpdatedAt, Again: again}
}

func (m *panesModal) mentionsQuery(again bool) refssvc.MentionsQuery {
	return refssvc.MentionsQuery{Repo: m.repo, Number: m.number, Pull: true, Again: again}
}

// readRefs reads what the pull request names, which a message of the modal
// then brings.
func (m *panesModal) readRefs(again bool) tea.Cmd {
	if m.dev == nil || m.dev.refSvc == nil {
		return nil
	}
	svc, ctx, id, q := m.dev.refSvc, m.ctx, m.id, m.refQuery(again)
	return func() tea.Msg {
		r, err := svc.References(ctx, q)
		return devRefsMsg{id: id, refs: r, err: err}
	}
}

// readMentions reads the first page of the items that name the pull request.
func (m *panesModal) readMentions(again bool) tea.Cmd {
	if m.dev == nil || m.dev.refSvc == nil {
		return nil
	}
	svc, ctx, id, q := m.dev.refSvc, m.ctx, m.id, m.mentionsQuery(again)
	return func() tea.Msg {
		pg, err := svc.Mentions(ctx, q)
		return devMentionsMsg{id: id, page: pg, err: err}
	}
}

// receiveRefs takes the references that a read brought. A read that failed
// leaves the two groups out.
func (m *panesModal) receiveRefs(msg devRefsMsg) tea.Cmd {
	if m.dev == nil || msg.id != m.id || msg.err != nil {
		return nil
	}
	m.dev.refs = msg.refs
	m.dev.showLinks()
	m.follow()
	if msg.refs.Stale {
		return m.readRefs(true)
	}
	return m.readMentions(false)
}

// receiveMentions takes the page of mentions that a read brought.
func (m *panesModal) receiveMentions(msg devMentionsMsg) tea.Cmd {
	if m.dev == nil || msg.id != m.id || msg.err != nil {
		return nil
	}
	m.dev.mentions = msg.page
	m.dev.showLinks()
	m.follow()
	if msg.page.Stale {
		return m.readMentions(true)
	}
	return m.loadPreview()
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
	return m.dev.model.Version() + m.dev.previews.Version()
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
	m.dev.links, m.dev.haveLinks = msg.links, true
	m.dev.showLinks()
	m.follow()
	return m.loadPreview()
}

// refreshDev shows the links the cache holds now, such as those served kept
// before GitHub answered.
func (m *panesModal) refreshDev() {
	if m.dev == nil {
		return
	}
	m.dev.previews.Retry()
	m.cachedDev()
	m.follow()
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
// pane has the focus and shows a preview.
func (m *panesModal) loadPreview() tea.Cmd {
	p := m.dev
	if p == nil || m.nav.Focus() != paneDevelopment {
		return nil
	}
	if _, ok := m.layout().Box(paneDevelopment, panes.Preview); !ok {
		return nil
	}
	return p.previews.Load(m.slots.Hover(paneDevelopment).ID, p.hov)
}

// devPreview is the body of the preview of the hovered row.
func (m *panesModal) devPreview(b panes.Box) string {
	p := m.dev
	if p == nil {
		return m.placeholderPreview(b)
	}
	o := dev.PreviewOptions{Repo: m.repo, Styles: p.styles, Now: m.clock(), Width: b.Rect.Inner().W}
	return p.previews.View(dev.Row{
		Key: m.slots.Hover(b.Pane).ID, Item: p.hov, Refs: p.model.Group(p.hov.Group),
		Loaded: p.model.Loaded(), Stamp: p.model.Version(),
	}, o)
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
	if m.nav.Focus() == b.Pane && m.nav.Sub(b.Pane) == panes.List {
		m.dev.model.Focus()
	} else {
		m.dev.model.Blur()
	}
	c := panes.Content{Body: m.dev.model.View()}
	if n := m.dev.model.Closes(); n > 0 {
		c.TopRight = []string{fmt.Sprintf("%d closes", n)}
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
func (m *panesModal) enterDev(msg treeview.OpenMsg) tea.Cmd {
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
		return func() tea.Msg { return ui.OpenPullMsg{Repo: r.Target.Repo, Number: r.Target.Number, Over: m} }
	}
	return func() tea.Msg { return ui.OpenIssueMsg{Repo: r.Target.Repo, Number: r.Target.Number, Over: m} }
}
