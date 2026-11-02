package pulls

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// headerChipName is the longest label name the header shows.
const headerChipName = 24

// drawnPanes is the panes area as it was last drawn, and what it was drawn
// from.
type drawnPanes struct {
	key drawKey
	out string
	// dim is out as the panes show behind an overlay, once it was needed.
	dim string
}

// drawKey is everything the drawn panes depend on.
type drawKey struct {
	width, height int
	nav           panes.Nav
	zoom          bool
	gen           int
	files         int
	updated       int64
	hover         hovered
	offset        int
	merge         int
	conv          int
	involved      int
	// dev is the version of the Development pane's tree.
	dev int
	// desc is the version of the description's tree, and body and edited
	// are what it was made from.
	desc   int
	body   string
	edited int64
	// header is the rows above the panes.
	header int
}

// SetTheme implements ui.Modal.
func (m *panesModal) SetTheme(t ui.Theme) {
	m.theme = t
	m.st = newStyles(t, m.icons)
	m.frames = panes.DefaultStyles(t.Dark)
	m.frames.Border = m.icons.Border
	m.frames.Ellipsis = m.icons.Ellipsis
	m.frames.Separator = m.icons.Separator
	m.chips = ui.NewLabelChips(m.st.label, t.Dark, m.icons.Ellipsis, headerChipName)
	m.dim = t.Subtle.Faint(true)
	m.cst = t.Confirm(m.icons)
	m.restyleInvolved()
	m.gen++
	m.merge.restyle(m)
	m.restyleConv()
	m.restyleDev()
	m.restyleFilesDiff()
}

// View implements ui.Modal: the labels, if there are any, the panes and the
// footer.
func (m *panesModal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	l := m.layout()
	lines := make([]string, 0, 3)
	hr := m.headerRows()
	if hr > 0 {
		lines = append(lines, m.header())
	}
	if m.height >= 2+hr {
		out := m.panes(l)
		if m.top() != nil {
			out = m.covered(out)
		}
		lines = append(lines, out)
	}
	if m.height >= 1+hr {
		if m.top() != nil {
			lines = append(lines, m.overlayFooter())
		} else {
			lines = append(lines, m.footer(l))
		}
	}
	return strings.Join(lines, "\n")
}

// panes draws the panes of l, or takes them from the last time if nothing
// they show changed.
func (m *panesModal) panes(l panes.Layout) string {
	k := drawKey{m.width, m.height, m.nav, l.Zoomed, m.gen, m.files.seq, m.detail.UpdatedAt.UnixNano(), m.hover[m.nav.Focus()-1], m.offset(), m.merge.version, m.conv.version, m.inv.ver, m.devVersion(), m.desc.version, m.detail.Body, m.detail.BodyEditedAt.UnixNano(), m.headerRows()}
	if m.drawn.out != "" && m.drawn.key == k {
		return m.drawn.out
	}
	out := m.frames.Draw(l, func(b panes.Box) panes.Content {
		if b.Role == panes.Preview {
			return m.previewContent(b)
		}
		c := m.slots[b.Pane-1].content(m, b)
		c.Focus = b.Pane == l.Focus
		if len(c.TopLeft) == 0 {
			title := "[" + strconv.Itoa(int(b.Pane)) + "] " + paneName(b.Pane)
			if l.Zoomed {
				// The digits are in the strip on the bottom border.
				title = paneName(b.Pane)
			}
			c.TopLeft = m.titleForms(title, b)
		}
		if l.Zoomed {
			c.BottomLeft = m.stripForms(b.Pane)
		}
		return c
	})
	m.drawn = drawnPanes{key: k, out: out}
	return out
}

// titleForms are the forms of a title for box b: the whole title, then the
// title cut to the room the border leaves it, so that one that does not fit
// still shows in part. The bubble never cuts a form itself.
func (m *panesModal) titleForms(title string, b panes.Box) []string {
	if title == "" {
		return nil
	}
	room := b.Rect.W - 5
	if room <= 0 || ansi.StringWidth(title) <= room {
		return []string{title}
	}
	return []string{title, ansi.Truncate(title, room, m.icons.Ellipsis)}
}

// stripForms are the forms of the strip of the panes with p marked, for the
// bottom border of the zoomed box, the longest first.
func (m *panesModal) stripForms(p panes.Pane) []string {
	return m.frames.StripForms(paneDefs, p)
}

// sealed cuts or pads line to width cells, and ends its styles, so that
// the next line starts plain.
func sealed(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	if pad := width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line + "\x1b[m"
}

// headerRows is the rows above the panes: one for the labels of the pull
// request if it has any, else none, so the panes have the row.
func (m *panesModal) headerRows() int {
	if len(m.detail.Labels) > 0 {
		return 1
	}
	return 0
}

// header is the first line: the labels of the pull request as far as they
// fit, and the number of the others. The state and title are in the top edge
// of the frame.
func (m *panesModal) header() string {
	labels := m.detail.Labels
	chips := make([]string, len(labels))
	widths := make([]int, len(labels))
	for i, l := range labels {
		chips[i], widths[i] = m.chips.Get(l)
	}
	for k := len(labels); k >= 0; k-- {
		need := max(k-1, 0)
		for _, w := range widths[:k] {
			need += w
		}
		more := "+" + strconv.Itoa(len(labels)-k)
		if k < len(labels) {
			need += len(more)
			if k > 0 {
				need++
			}
		}
		if need > m.width && k > 0 {
			continue
		}
		line := strings.Join(chips[:k], " ")
		if k < len(labels) {
			if k > 0 {
				line += " "
			}
			line += m.theme.Subtle.Render(more)
		}
		return sealed(line, m.width)
	}
	return sealed("", m.width)
}

// footer is the last line: the keys that work in the modal, as far as they
// are bound.
func (m *panesModal) footer(l panes.Layout) string {
	k := m.keys
	hint := func(keys, what string) string {
		if keys == "" {
			return ""
		}
		return m.theme.Muted.Render(keys) + " " + m.theme.Subtle.Render(what)
	}
	var parts []string
	switch {
	case !l.Zoomed:
		parts = append(parts, hint(hintKeys(k.next, k.jump), "focus"))
	default:
		if m.nav.CanStep(l) {
			what := "preview"
			if m.nav.InPreview(l) {
				what = "list"
			}
			parts = append(parts, hint(hintKeys(k.next), what))
		}
		parts = append(parts, hint(hintKeys(k.jump), "other pane"))
	}
	if m.zoom.CanZoom() {
		what := "zoom"
		if l.Zoomed {
			what = "unzoom"
		}
		parts = append(parts, hint(hintKeys(k.zoom), what))
	}
	if m.nav.Focus() == paneConversation {
		parts = append(parts, hint(hintKeys(k.convComment), "comment"))
	}
	parts = append(parts, hint(hintKeys(k.help), "help"))
	// A key that is unbound leaves its part out.
	parts = slices.DeleteFunc(parts, func(p string) bool { return p == "" })
	return sealed(strings.Join(parts, m.theme.Subtle.Render(m.icons.Separator)), m.width)
}
