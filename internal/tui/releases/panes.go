package releases

import (
	"errors"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// The panes of the release modal, in digit order: the notes over the files.
const (
	paneNotes panes.Pane = iota + 1
	paneFiles
)

const paneCount = int(paneFiles)

var paneDefs = []panes.Def{
	{Name: "Notes", Short: "Notes"},
	{Name: "Files", Short: "Files"},
}

// showsOne reports whether only the focused pane shows: when it is zoomed,
// or the area is too narrow for the stack.
func showsOne(w int, zoom bool) bool { return zoom || w < 2*panes.MinWidth }

func paneName(p panes.Pane) string {
	if p < 1 || int(p) > len(paneDefs) {
		return ""
	}
	return paneDefs[p-1].Name
}

// releaseLayout lays out an area of w by h cells: the notes over the files,
// or the focused pane alone.
func releaseLayout(w, h int, focus panes.Pane, zoom bool, d *ui.Dividers) panes.Layout {
	spec := panes.Spec{Width: w, Height: h, Panes: paneDefs, Focus: focus, Zoom: showsOne(w, zoom)}
	if spec.Zoom {
		return panes.Compute(spec)
	}
	w, h = max(0, w), max(0, h)
	if focus < 1 || int(focus) > paneCount {
		focus = paneNotes
	}
	cut := stackCut(d, h)
	l := panes.Layout{Width: w, Height: h, Focus: focus, Panes: paneDefs, Boxes: []panes.Box{
		{Pane: paneNotes, Rect: panes.Rect{X: 0, Y: 0, W: w, H: cut}},
		{Pane: paneFiles, Rect: panes.Rect{X: 0, Y: cut, W: w, H: h - cut}},
	}}
	for i := range l.Boxes {
		r := &l.Boxes[i].Rect
		r.X, r.Y = max(0, min(r.X, w)), max(0, min(r.Y, h))
		r.W, r.H = max(0, min(r.W, w-r.X)), max(0, min(r.H, h-r.Y))
	}
	return l
}

// stackCut is the row where the Files start in the stack: the notes take the
// larger share. The divider Info[0] moves it.
func stackCut(d *ui.Dividers, h int) int {
	if h < 2*panes.MinHeight {
		return h / 2
	}
	def := h * 3 / 5
	if d == nil {
		return def
	}
	return d.Info[0].Of(def, panes.MinHeight, h-panes.MinHeight)
}

// fillPanes sets the pane keys, taken from the global keys.
func (k *keyMap) fillPanes(keys config.Keymap) {
	modal := ui.In(keys, ctxModal)
	k.Next = modal.Binding("global.next_pane", "next pane")
	k.Prev = modal.Binding("global.prev_pane", "previous pane")
	k.Zoom = modal.Binding("global.zoom", "zoom")
	k.DividerReset = modal.Binding(config.ActionDividerReset, "reset dividers")
	k.DividerUp = modal.Binding(config.ActionDividerUp, "grow pane")
	k.DividerDown = modal.Binding(config.ActionDividerDown, "shrink pane")
	k.Pane1 = modal.Binding("global.pane_1", paneName(paneNotes))
	k.Pane2 = modal.Binding("global.pane_2", paneName(paneFiles))
	k.jump = ui.Jump(k.Pane1, k.Pane2)
}

func (k keyMap) paneBindings() []key.Binding {
	return []key.Binding{k.Next, k.Prev, k.Zoom, k.DividerUp, k.DividerDown, k.DividerReset, k.Pane1, k.Pane2}
}

func (k keyMap) digit(msg tea.KeyPressMsg) (panes.Pane, bool) {
	for i, b := range [paneCount]key.Binding{k.Pane1, k.Pane2} {
		if keymap.Matches(msg, b) {
			return panes.Pane(i + 1), true
		}
	}
	return 0, false
}

func (m *Modal) paneLayout() panes.Layout {
	return releaseLayout(m.width, m.areaHeight(), m.nav.Focus(), m.zoom.Zoomed(), m.dividers)
}

// areaHeight is the height of the panes: the modal's, less the header.
func (m *Modal) areaHeight() int {
	if m.loaded && !m.failed() {
		return max(m.height-m.headerRows(), 0)
	}
	return m.height
}

// pressPane handles the keys that move between panes, zoom and move the
// dividers, and reports whether msg was one.
func (m *Modal) pressPane(msg tea.KeyPressMsg) bool {
	k := m.keys
	l := m.paneLayout()
	d := m.dividers
	switch {
	case keymap.Matches(msg, k.DividerUp) && m.canStack():
		m.growPane(1)
	case keymap.Matches(msg, k.DividerDown) && m.canStack():
		m.growPane(-1)
	case keymap.Matches(msg, k.DividerReset) && d != nil && d.Moved():
		d.Reset()
	case keymap.Matches(msg, k.Next):
		m.nav.Step(l, 1)
	case keymap.Matches(msg, k.Prev):
		m.nav.Step(l, -1)
	case keymap.Matches(msg, k.Zoom):
		m.zoom.Toggle()
		m.nav.Reset()
	default:
		p, ok := k.digit(msg)
		if !ok {
			return false
		}
		m.nav.SetFocus(p)
	}
	m.resize()
	return true
}

// canStack reports whether the stack shows, with room to move a divider.
func (m *Modal) canStack() bool {
	return m.dividers != nil && !m.paneLayout().Zoomed && m.areaHeight() >= 2*panes.MinHeight
}

// growPane grows the focused pane by dir presses, or shrinks it.
func (m *Modal) growPane(dir int) {
	h := m.areaHeight()
	_, sign := panes.Grow(int(m.nav.Focus())-1, 2)
	m.dividers.Move(&m.dividers.Info[0], dir*sign, h*3/5, panes.MinHeight, h-panes.MinHeight)
}

// drawPanes draws the boxes of l with their titles, counts and bodies.
func (m *Modal) drawPanes(l panes.Layout) string {
	return m.frames.Draw(l, func(b panes.Box) panes.Content {
		c := panes.Content{Focus: b.Pane == l.Focus}
		title := "[" + strconv.Itoa(int(b.Pane)) + "] " + paneName(b.Pane)
		if l.Zoomed {
			title = paneName(b.Pane)
			c.BottomLeft = m.frames.StripForms(paneDefs, b.Pane)
		}
		c.TopLeft = titleForms(title, b, m.icons.Ellipsis)
		if b.Pane == paneFiles && m.loaded {
			c.TopRight = ui.CountForms(ui.CountSpec{Total: len(m.rel.Assets), Loaded: len(m.rel.Assets)})
		}
		c.Body = m.paneBody(b)
		return c
	})
}

func titleForms(title string, b panes.Box, ellipsis string) []string {
	room := b.Rect.W - 5
	if room <= 0 || ansi.StringWidth(title) <= room {
		return []string{title}
	}
	return []string{title, ansi.Truncate(title, room, ellipsis)}
}

func (m *Modal) paneBody(b panes.Box) string {
	in := b.Rect.Inner()
	if in.W <= 0 || in.H <= 0 {
		return ""
	}
	if !m.loaded {
		if b.Pane != paneNotes {
			return ""
		}
		return ansi.Truncate("Loading"+m.icons.Ellipsis, in.W, m.icons.Ellipsis)
	}
	if b.Pane == paneNotes {
		m.notes.SetSize(in.W, in.H)
		m.buildNotes(in.W)
		if m.nav.Focus() == paneNotes {
			m.notes.Focus()
		} else {
			m.notes.Blur()
		}
		return m.notes.View()
	}
	m.files.SetSize(in.W, in.H)
	if m.nav.Focus() == paneFiles {
		m.files.Focus()
	} else {
		m.files.Blur()
	}
	return m.withReadError(m.files.View(), in.W)
}

// withReadError puts why the release couldn't be read again over the last
// rows of the files, which keep showing what was read before. It names no
// key to retry: the release is read again by itself.
func (m *Modal) withReadError(view string, width int) string {
	if m.err == nil {
		return view
	}
	v := m.voice
	v.Retry = key.NewBinding()
	text, hint := ui.ErrorText("load the release", "", v)(m.err)
	if errors.Is(m.err, core.ErrForbidden) {
		text, hint = ui.ErrorText("load the release", m.repo.String(), v)(m.err)
	}
	errs := ui.ErrorLine(m.errs, text, hint, max(width, 1))
	rows := strings.Split(view, "\n")
	if len(errs) > len(rows) {
		return view
	}
	copy(rows[len(rows)-len(errs):], errs)
	return strings.Join(rows, "\n")
}
