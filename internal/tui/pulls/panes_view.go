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
	focus         panes.Pane
	zoom          bool
	sub           [panes.Count]panes.Role
	gen           int
	files         int
	updated       int64
	hover         hovered
	offset        int
	desc          int
	body          string
	edited        int64
}

// SetTheme implements ui.Modal.
func (m *panesModal) SetTheme(t ui.Theme) {
	m.theme = t
	m.st = newStyles(t, m.icons)
	m.frames = panes.DefaultStyles(t.Dark)
	m.frames.Border = m.icons.Border
	m.frames.Ellipsis = m.icons.Ellipsis
	m.chips = ui.NewLabelChips(m.st.label, t.Dark, m.icons.Ellipsis, headerChipName)
	m.dim = t.Subtle.Faint(true)
	m.cst = t.Confirm(m.icons)
	m.gen++
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
		if m.top() != nil {
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
	k := drawKey{m.width, m.height, m.focus, l.Zoomed, m.sub, m.gen, m.files.seq, m.detail.UpdatedAt.UnixNano(), m.hover[m.focus-1], m.offset(), m.desc.version, m.detail.Body, m.detail.BodyEditedAt.UnixNano()}
	if m.drawn.out != "" && m.drawn.key == k {
		return m.drawn.out
	}
	out := m.frames.Draw(l, func(b panes.Box) panes.Content {
		if b.Role == panes.Preview {
			return m.previewContent(b)
		}
		return m.slots[b.Pane-1].content(m, b)
	})
	m.drawn = drawnPanes{key: k, out: out}
	return out
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
		if m.canStep(l) {
			what := "preview"
			if m.inPreview(l) {
				what = "list"
			}
			parts = append(parts, hint(hintKeys(k.next), what))
		}
		parts = append(parts, hint(hintKeys(k.jump), "other pane"))
	}
	if m.canZoom() {
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
