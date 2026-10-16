package pulls

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// overlay is a picker, a confirm or a viewer that the modal opens over its
// panes. While one is open, the keys go to it alone, and the panes behind it
// keep their focus, zoom and scroll.
type overlay interface {
	// title is what the frame of the overlay says.
	title() string
	// kind is how the overlay is placed over the panes.
	kind() panes.OverlayKind
	// items is the number of rows of a picker, and text the width of what a
	// confirm asks.
	items() int
	text() int
	// update takes a key or a paste.
	update(tea.Msg) tea.Cmd
	// view draws the inside of the frame, w by h cells.
	view(w, h int) string
	// layers are the keys of the overlay, for the footer and the help.
	layers() []keyhelp.Layer
	// clear drops what is transient, such as a search, and reports whether
	// there was any.
	clear() bool
	// closed reports that the overlay is done and takes no more keys.
	closed() bool
}

// openOverlay is an overlay on the stack, with whether it fills the area.
type openOverlay struct {
	overlay
	maximized bool
}

// top returns the overlay that has the keys, or nil.
func (m *panesModal) top() overlay {
	if len(m.overlays) == 0 {
		return nil
	}
	return m.overlays[len(m.overlays)-1].overlay
}

// open puts o on top of the panes and the overlays already open.
func (m *panesModal) open(o overlay) {
	m.overlays = append(m.overlays, openOverlay{overlay: o})
}

// settle drops the overlays on top that are done, which returns the focus to
// what is below.
func (m *panesModal) settle() {
	for len(m.overlays) > 0 && m.top().closed() {
		m.overlays = m.overlays[:len(m.overlays)-1]
	}
}

// pop closes the overlay on top.
func (m *panesModal) pop() {
	if len(m.overlays) > 0 {
		m.overlays = m.overlays[:len(m.overlays)-1]
	}
}

// pressOverlay gives msg to the overlay on top.
func (m *panesModal) pressOverlay(msg tea.Msg) tea.Cmd {
	cmd := m.top().update(msg)
	m.settle()
	return cmd
}

// ask opens the question c over the panes.
func (m *panesModal) ask(c ui.Confirm) {
	m.open(&confirmOverlay{m: m, c: c})
}

// placed returns where the overlay on top lies in the panes' area.
func (m *panesModal) placed() panes.Rect {
	o := m.overlays[len(m.overlays)-1]
	return panes.OverlayRect(panes.Overlay{Kind: o.kind(), Items: o.items(), Text: o.text(), Maximized: o.maximized}, m.width, m.areaHeight())
}

// areaHeight is the height of the panes' area: the modal without its header
// and footer.
func (m *panesModal) areaHeight() int { return max(m.height-2, 0) }

// dimmed returns the panes as they were drawn, without their styles and
// faint. It is made once for each drawing of the panes.
func (m *panesModal) dimmed(out string) string {
	if m.drawn.dim != "" {
		return m.drawn.dim
	}
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = m.dim.Render(ansi.Strip(l))
	}
	m.drawn.dim = strings.Join(lines, "\n")
	return m.drawn.dim
}

// covered draws the overlay on top over the dimmed panes.
func (m *panesModal) covered(out string) string {
	r := m.placed()
	lines := strings.Split(m.dimmed(out), "\n")
	if r.W <= 0 || r.H <= 0 {
		return strings.Join(lines, "\n")
	}
	o := m.top()
	box := strings.Split(m.frames.Frame(panes.Content{Title: o.title(), Body: o.view(max(r.W-2, 0), max(r.H-2, 0)), Focus: true}, r.W, r.H), "\n")
	for i, row := range box {
		y := r.Y + i
		if y >= len(lines) {
			break
		}
		lines[y] = ansi.Cut(lines[y], 0, r.X) + "\x1b[m" + row + "\x1b[m" + ansi.Cut(lines[y], r.X+r.W, m.width)
	}
	return strings.Join(lines, "\n")
}

// overlayFooter is the last line while an overlay is open: the keys of the
// overlay alone.
func (m *panesModal) overlayFooter() string {
	var parts []string
	for _, l := range m.top().layers() {
		for _, b := range l.Short {
			if b.Enabled() && len(b.Keys()) > 0 {
				parts = append(parts, m.theme.Muted.Render(b.Help().Key)+" "+m.theme.Subtle.Render(b.Help().Desc))
			}
		}
	}
	return sealed(strings.Join(parts, m.theme.Subtle.Render(m.icons.Separator)), m.width)
}

// confirmOverlay asks a question, which y answers with yes and n or esc with
// no. Enter does nothing.
type confirmOverlay struct {
	m    *panesModal
	c    ui.Confirm
	done bool
}

func (o *confirmOverlay) title() string           { return "Confirm" }
func (o *confirmOverlay) kind() panes.OverlayKind { return panes.Confirm }
func (o *confirmOverlay) items() int              { return 0 }
func (o *confirmOverlay) text() int               { return ansi.StringWidth(ui.OneLine(o.c.Question)) }
func (o *confirmOverlay) clear() bool             { return false }
func (o *confirmOverlay) closed() bool            { return o.done }
func (o *confirmOverlay) view(w, _ int) string    { return o.c.Line(o.m.cst, o.m.keys.confirm, w) }

// layers are the keys that answer, and the quit key, which closes the whole
// modal: the question takes every key, so the app never sees that one.
func (o *confirmOverlay) layers() []keyhelp.Layer {
	l := o.m.keys.confirm.Layer()
	l.Bindings = append(l.Bindings, o.m.keys.quit)
	l.Short = append(l.Short, o.m.keys.quit)
	return []keyhelp.Layer{l}
}

func (o *confirmOverlay) update(msg tea.Msg) tea.Cmd {
	press, ok := msg.(tea.KeyPressMsg)
	if !ok || o.done {
		return nil
	}
	if keymap.Matches(press, o.m.keys.quit) {
		o.done = true
		return o.m.close()
	}
	cmd, done := o.m.keys.confirm.Answer(o.c, press)
	o.done = done
	return cmd
}
