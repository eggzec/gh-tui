package pulls

import (
	"slices"
	"strings"

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
	dk := drawnInvolved{m.hover[b.Pane-1].id, b.Rect.Inner().W}
	if v, ok := p.drawn[dk]; ok {
		return v.preview, v.body, true
	}
	pv, ok := involved.PreviewOf(p.data, dk.id, involved.Options{Styles: p.st, Width: dk.width})
	if !ok {
		return involved.Preview{}, "", false
	}
	body, final := m.involvedBody(pv, dk.width)
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

// involvedBody draws pv: its text, or its head over the conversation narrowed
// to the person. final is false while the conversation isn't read, so that the
// text is drawn again once it is.
func (m *panesModal) involvedBody(pv involved.Preview, width int) (body string, final bool) {
	if len(pv.Head) == 0 {
		return pv.Body, true
	}
	lines := slices.Clone(pv.Head)
	c := &m.conv
	switch {
	case pv.Key == "":
		lines = append(lines, pv.Nothing)
	case !c.started || !c.model.Ready():
		return strings.Join(append(lines, m.theme.Subtle.Render("loading")), "\n"), false
	default:
		if view, ok := c.model.Peek(pv.Key, pv.Narrow, width); ok {
			lines = append(lines, view)
		} else {
			lines = append(lines, pv.Nothing)
		}
	}
	return strings.Join(lines, "\n"), true
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
