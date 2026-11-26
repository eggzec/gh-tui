package issues

import (
	"context"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/dev"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/slots"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// peekKey names the view of the conversation that the previews draw, which
// keeps its own folds apart from the view the Conversation pane shows.
const peekKey = "preview"

// drawnPeek is the conversation drawn as a preview, kept until its width or the
// conversation changes.
type drawnPeek struct {
	ver   uint64
	width int
	body  string
	set   bool
}

// PreviewService is what the previews of the Development and Blockers rows
// read. *pulls.Service satisfies it.
type PreviewService interface {
	CachedIssuePreview(q pulls.PreviewQuery) (core.IssuePreview, bool)
	IssuePreview(ctx context.Context, q pulls.PreviewQuery) (core.IssuePreview, error)
	CachedPullPreview(q pulls.PreviewQuery) (core.PullPreview, bool)
	PullPreview(ctx context.Context, q pulls.PreviewQuery) (core.PullPreview, error)
}

// WithPreviews reads the previews of the Development and Blockers rows from
// svc. Without it those panes have none.
func WithPreviews(svc PreviewService) Option {
	return func(s *Section) { s.previews = svc }
}

// devReader adapts a PreviewService to what dev.Previews reads.
type devReader struct{ s PreviewService }

func query(t core.Target) pulls.PreviewQuery {
	return pulls.PreviewQuery{Repo: t.Repo, Number: t.Number}
}

func (r devReader) CachedIssue(t core.Target) (core.IssuePreview, bool) {
	return r.s.CachedIssuePreview(query(t))
}

func (r devReader) Issue(ctx context.Context, t core.Target) (core.IssuePreview, error) {
	return r.s.IssuePreview(ctx, query(t))
}

func (r devReader) CachedPull(t core.Target) (core.PullPreview, bool) {
	return r.s.CachedPullPreview(query(t))
}

func (r devReader) Pull(ctx context.Context, t core.Target) (core.PullPreview, error) {
	return r.s.PullPreview(ctx, query(t))
}

// paneSlots has one entry per pane, in the order of the panes. Description and
// Conversation have no preview beside them.
var paneSlots = [paneCount]slots.Slot[*detailModal]{
	paneInvolved - 1:    {Hovered: (*detailModal).hoveredInvolved, Preview: (*detailModal).involvedPreview},
	paneDevelopment - 1: {Hovered: (*detailModal).hoveredDev, Preview: (*detailModal).devPreview},
	paneBlockers - 1:    {Hovered: (*detailModal).hoveredDeps, Preview: (*detailModal).depsPreview},
}

// hoveredInvolved names what the Involved preview shows: the conversation of
// the issue, whichever person is under the cursor.
func (m *detailModal) hoveredInvolved() (id, title string, ok bool) {
	return "conversation", "#" + strconv.Itoa(m.number), true
}

// involvedPreview is the conversation of the issue, read-only.
func (m *detailModal) involvedPreview(b panes.Box) string {
	if body, ok := m.peekBody(b.Rect.Inner().W); ok {
		return body
	}
	text := "no conversation"
	if !m.convOn || !m.conv.Ready() {
		text = "loading"
	}
	return m.theme.Muted.Render(text)
}

// hoveredDev names the row of the Development tree for the preview.
func (m *detailModal) hoveredDev() (id, title string, ok bool) {
	p := m.side
	if p == nil || !p.dev.Loaded() {
		return "", "", false
	}
	id, title, it, ok := p.dev.Hovered()
	if ok {
		p.devHov = it
	}
	return id, title, ok
}

// hoveredDeps names the issue under the cursor of the Blockers tree for the
// preview. The roots have nothing to preview.
func (m *detailModal) hoveredDeps() (id, title string, ok bool) {
	p := m.side
	if p == nil {
		return "", "", false
	}
	n, found := p.deps.Selected()
	v, isDep := n.Value.(depNode)
	if !found || !isDep || v.dep.Number == 0 {
		return "", "", false
	}
	d := v.dep
	t := core.Target{Repo: refOf(m.repo, d).repo, Number: d.Number, Kind: core.KindIssue}
	kind, word := dev.Issue, "Issue "
	if d.IsPullRequest {
		t.Kind, kind, word = core.KindPull, dev.Pull, "Pull request "
	}
	p.depHov = dev.Item{Kind: kind, Ref: core.Reference{Target: t, Title: d.Title, URL: d.URL}}
	return "dep:" + core.RefKey(t), word + refOf(m.repo, d).label(m.repo), true
}

// devPreview is the body of the preview of the hovered Development row.
func (m *detailModal) devPreview(b panes.Box) string {
	return m.hoverPreview(b, m.side.devHov, true)
}

// depsPreview is the body of the preview of the hovered Blockers row.
func (m *detailModal) depsPreview(b panes.Box) string {
	return m.hoverPreview(b, m.side.depHov, false)
}

func (m *detailModal) hoverPreview(b panes.Box, it dev.Item, tree bool) string {
	p := m.side
	if p == nil || p.previews == nil {
		return m.theme.Muted.Render("no preview")
	}
	o := dev.PreviewOptions{Repo: m.repo, Styles: p.devSt, Now: m.now(), Width: b.Rect.Inner().W}
	return p.previews.View(dev.Row{
		Key: m.slots.Hover(b.Pane).ID, Item: it, Links: p.dev.Links(),
		Loaded: !tree || p.dev.Loaded(), Stamp: p.dev.Version(),
	}, o)
}

// follow moves the previews to the rows the lists have under their cursors, and
// returns the read of the preview of the focused pane's row, when that pane
// shows a preview.
func (m *detailModal) follow() tea.Cmd {
	if m.slots == nil {
		return nil
	}
	m.slots.Follow(m)
	p := m.side
	if p == nil || p.previews == nil {
		return nil
	}
	f := m.nav.Focus()
	var it dev.Item
	switch f {
	case paneDevelopment:
		it = p.devHov
	case paneBlockers:
		it = p.depHov
	default:
		return nil
	}
	id := m.slots.Hover(f).ID
	if id == "" {
		return nil
	}
	if _, ok := m.paneLayout().Box(f, panes.Preview); !ok {
		return nil
	}
	return p.previews.Load(id, it)
}

// resetPreviews scrolls the previews back to their tops.
func (m *detailModal) resetPreviews() {
	for p := paneInvolved; p <= paneBlockers; p++ {
		m.slots.Previews().Reset(slots.Key{Pane: p, ID: m.slots.Hover(p).ID})
	}
}

// peekBody draws the conversation width cells wide, once per width until the
// conversation changes. ok is false while it is not read.
func (m *detailModal) peekBody(width int) (string, bool) {
	if !m.convOn {
		return "", false
	}
	d := &m.peek
	if d.set && d.ver == m.peekVer && d.width == width {
		return d.body, true
	}
	body, ok := m.conv.Peek(peekKey, conv.Narrow{}, width)
	if ok {
		// Until it is read the text isn't final: draw it again next time.
		*d = drawnPeek{ver: m.peekVer, width: width, body: body, set: true}
	}
	return body, ok
}
