package pulls

import (
	"github.com/eggzec/gh-tui/internal/tui/prpanes/involved"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// drawnInvolved names a preview drawn: the row it shows and its width.
type drawnInvolved struct {
	id    string
	width int
}

// hoveredInvolved names the row of the tree for the preview, and the title of
// the preview's frame.
func (m *panesModal) hoveredInvolved() (id, title string, ok bool) {
	p := m.inv
	if p == nil {
		return "", "", false
	}
	id, ok = p.tree.SelectedID()
	if !ok {
		return "", "", false
	}
	pv, ok := involved.PreviewOf(p.data, id, involved.Options{Styles: p.st})
	return id, pv.Title, ok
}

// involvedPreviewOf is the preview of the hovered row in box b, drawn once per
// width until the people or the conversation change.
func (m *panesModal) involvedPreviewOf(b panes.Box) (involved.Preview, string, bool) {
	p := m.inv
	if p == nil {
		return involved.Preview{}, "", false
	}
	if p.drawnVer != p.dver || p.drawnConv != m.conv.version {
		clear(p.drawn)
		p.drawnVer, p.drawnConv = p.dver, m.conv.version
	}
	dk := drawnInvolved{m.slots.Hover(b.Pane).ID, b.Rect.Inner().W}
	if v, ok := p.drawn[dk]; ok {
		return v.preview, v.body, true
	}
	pv, ok := involved.PreviewOf(p.data, dk.id, involved.Options{Styles: p.st, Width: dk.width})
	if !ok {
		return involved.Preview{}, "", false
	}
	c := &m.conv
	body, final := involved.Draw(pv, c.started && c.model.Ready(), c.model.Peek, func(s string) string { return m.theme.Subtle.Render(s) }, dk.width)
	if final {
		p.drawn[dk] = drawnPerson{pv, body}
	}
	return pv, body, true
}

// drawnPerson is a preview with the text it was drawn as.
type drawnPerson struct {
	preview involved.Preview
	body    string
}

// involvedPreview is the body of the preview of the hovered row, or the
// placeholder for a row with no preview.
func (m *panesModal) involvedPreview(b panes.Box) string {
	if _, body, ok := m.involvedPreviewOf(b); ok {
		return body
	}
	return m.placeholderPreview(b)
}

// involvedInfo is the info of the frame of the preview of the hovered row: the
// roles of a person.
func (m *panesModal) involvedInfo(b panes.Box) []string {
	if pv, _, ok := m.involvedPreviewOf(b); ok && pv.Info != "" {
		return []string{pv.Info}
	}
	return nil
}
