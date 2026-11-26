package issues

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/conv"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/dev"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/involved"
	"github.com/eggzec/gh-tui/internal/tui/prpanes/slots"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// drawnPeek is the conversation drawn as a preview, kept until its width or the
// conversation changes.
type drawnPeek struct {
	id    string
	ver   uint64
	width int
	body  string
	set   bool
}

// PreviewService is what the previews of the Development and Blockers rows
// read. *pulls.Service satisfies it.
type PreviewService = dev.PreviewService

// WithPreviews reads the previews of the Development and Blockers rows from
// svc. Without it those panes have none.
func WithPreviews(svc PreviewService) Option {
	return func(s *Section) { s.previews = svc }
}

// paneSlots has one entry per pane, in the order of the panes. Description and
// Conversation have no preview beside them.
var paneSlots = [paneCount]slots.Slot[*detailModal]{
	paneInvolved - 1:    {Hovered: (*detailModal).hoveredInvolved, Preview: (*detailModal).involvedPreview, Info: (*detailModal).involvedInfo},
	paneDevelopment - 1: {Hovered: (*detailModal).hoveredDev, Preview: (*detailModal).devPreview},
	paneBlockers - 1:    {Hovered: (*detailModal).hoveredDeps, Preview: (*detailModal).depsPreview},
}

// involvedOptions are the options of the previews of the Involved rows, drawn
// width cells wide.
func (m *detailModal) involvedOptions(width int) involved.Options {
	return involved.Options{Styles: m.side.invSt, Width: width, HideReviewers: true, Noun: "issue"}
}

// hoveredInvolved names the row of the Involved pane for the preview, and the
// title of the preview's frame.
func (m *detailModal) hoveredInvolved() (id, title string, ok bool) {
	p := m.side
	if p == nil {
		return "", "", false
	}
	id, ok = p.inv.SelectedID()
	if !ok {
		return "", "", false
	}
	pv, ok := involved.PreviewOf(p.invData, id, m.involvedOptions(0))
	return id, pv.Title, ok
}

// involvedInfo is the info of the frame of the preview of the hovered row: the
// roles of a person.
func (m *detailModal) involvedInfo(b panes.Box) []string {
	if m.side == nil {
		return nil
	}
	pv, ok := involved.PreviewOf(m.side.invData, m.slots.Hover(b.Pane).ID, m.involvedOptions(b.Rect.Inner().W))
	if !ok || pv.Info == "" {
		return nil
	}
	return []string{pv.Info}
}

// involvedPreview is the preview of the hovered Involved row: a group's
// members, or a person's roles over what they wrote in the conversation.
func (m *detailModal) involvedPreview(b panes.Box) string {
	if m.side == nil {
		return m.theme.Muted.Render("no preview")
	}
	id, width := m.slots.Hover(b.Pane).ID, b.Rect.Inner().W
	pv, ok := involved.PreviewOf(m.side.invData, id, m.involvedOptions(width))
	if !ok {
		return m.theme.Muted.Render("no preview")
	}
	if len(pv.Head) == 0 {
		return pv.Body
	}
	body, ok := m.peekBody(id, pv, width)
	switch {
	case ok:
	case pv.Key == "":
		body = pv.Nothing
	case !m.convOn || !m.conv.Ready():
		body = m.theme.Muted.Render("loading")
	case conv.TooNarrow(width):
		body = m.theme.Muted.Render("too narrow")
	default:
		body = pv.Nothing
	}
	return strings.Join(append(slices.Clone(pv.Head), body), "\n")
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
		Key: m.slots.Hover(b.Pane).ID, Item: it, Refs: p.dev.Group(it.Group),
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
	f := m.ctl.Nav.Focus()
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
	for p := range paneCount {
		if paneSlots[p].Hovered == nil {
			continue
		}
		pane := panes.Pane(p + 1)
		m.slots.Previews().Reset(slots.Key{Pane: pane, ID: m.slots.Hover(pane).ID})
	}
}

// peekBody draws the conversation of the person of pv, the Involved row id,
// width cells wide, once per row and width until the conversation changes. ok
// is false while it is not read, or has nothing of the person.
func (m *detailModal) peekBody(id string, pv involved.Preview, width int) (string, bool) {
	if !m.convOn || pv.Key == "" {
		return "", false
	}
	d := &m.peek
	if d.set && d.ver == m.peekVer && d.width == width && d.id == id {
		return d.body, true
	}
	body, ok := m.conv.Peek(pv.Key, pv.Narrow, width)
	if ok {
		// Until it is read the text isn't final: draw it again next time.
		*d = drawnPeek{id: id, ver: m.peekVer, width: width, body: body, set: true}
	}
	return body, ok
}
