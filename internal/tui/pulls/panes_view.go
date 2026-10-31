package pulls

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

const (
	// headerChipName is the longest label name the header shows.
	headerChipName = 24
	// headerTitleMin is the width the title keeps before a label is dropped
	// to make room for it.
	headerTitleMin = 20
)

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

// View implements ui.Modal: the header, the panes and the footer.
func (m *panesModal) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	l := m.layout()
	lines := make([]string, 0, 3)
	lines = append(lines, m.header())
	if m.height >= 3 {
		out := m.panes(l)
		if m.hasStrip() {
			out += "\n" + m.strip()
		}
		if m.top() != nil {
			// The strip row is part of the area an overlay covers, so a
			// full-area overlay keeps its bottom border.
			out = m.covered(out)
		}
		lines = append(lines, out)
	}
	if m.height >= 2 {
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
	k := drawKey{m.width, m.height, m.nav, l.Zoomed, m.gen, m.files.seq, m.detail.UpdatedAt.UnixNano(), m.hover[m.nav.Focus()-1], m.offset(), m.merge.version, m.conv.version, m.inv.ver, m.devVersion(), m.desc.version, m.detail.Body, m.detail.BodyEditedAt.UnixNano()}
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
			title := l.Title(b.Pane)
			if l.Zoomed {
				// The digits are in the strip under the boxes.
				title = paneName(b.Pane)
			}
			c.TopLeft = m.titleForms(title, b)
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

// strip is the row of pane names under the zoomed boxes, in the longest form
// that fits the width, and blank when none does.
func (m *panesModal) strip() string {
	for _, f := range m.frames.StripForms(paneDefs, m.nav.Focus()) {
		if ansi.StringWidth(f) <= m.width {
			return sealed(f, m.width)
		}
	}
	return sealed("", m.width)
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

// header is the first line: the state, the number and title as a link to the
// pull request, and its labels as far as they fit.
func (m *panesModal) header() string {
	d := m.detail
	var lead string
	if m.loaded {
		lead = m.st.states[ui.PullState(d.State, d.Draft)] + " "
	}
	head := "#" + strconv.Itoa(m.number)
	if t := ui.OneLine(d.Title); t != "" {
		head += " " + t
	}
	rest := m.width - ansi.StringWidth(lead)
	room, tail := rest, ""
	if len(d.Labels) > 0 && rest > 0 {
		tail, room = m.labelTail(d.Labels, rest, ansi.StringWidth(head))
	}
	head = termtext.Truncate(head, room, m.icons.Ellipsis)
	title := m.theme.Text.Bold(true).Render(head)
	return sealed(lead+termtext.Link(d.URL, title)+tail, m.width)
}

// labelTail returns what follows a title that is headWidth cells wide, in
// rest cells: a separator, and as many label chips as fit, with the number of
// the others, and the width left to the title. Chips are dropped before the
// title is cut under headerTitleMin cells; none is shown if even the
// number of them doesn't fit.
func (m *panesModal) labelTail(labels []core.Label, rest, headWidth int) (tail string, titleWidth int) {
	sep := m.icons.Separator
	sepWidth := ansi.StringWidth(sep)
	keep := min(headWidth, headerTitleMin)
	chips := make([]string, len(labels))
	widths := make([]int, len(labels))
	for i, l := range labels {
		chips[i], widths[i] = m.chips.Get(l)
	}
	for k := len(labels); k >= 0; k-- {
		need := sepWidth
		for i := range k {
			need += widths[i]
			if i > 0 {
				need++
			}
		}
		more := "+" + strconv.Itoa(len(labels)-k)
		if k < len(labels) {
			need += len(more)
			if k > 0 {
				need++
			}
		}
		if rest-need < keep {
			continue
		}
		tail := m.theme.Subtle.Render(sep) + strings.Join(chips[:k], " ")
		if k < len(labels) {
			if k > 0 {
				tail += " "
			}
			tail += m.theme.Subtle.Render(more)
		}
		return tail, min(headWidth, rest-need)
	}
	return "", headWidth
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
	parts = append(parts, hint(hintKeys(k.help), "help"))
	// A key that is unbound leaves its part out.
	parts = slices.DeleteFunc(parts, func(p string) bool { return p == "" })
	return sealed(strings.Join(parts, m.theme.Subtle.Render(m.icons.Separator)), m.width)
}
