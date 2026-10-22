// proto-actions: throwaway prototype of a redesigned Actions modal over
// hardcoded data, drawn with pkg/bubbles/panes and the app's own theme.
// Keys 1, 2 and 3 pick the pane structure (see variants.go).
package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// wideMin is the width from which the panes show side by side; under it one
// pane shows, zoomed, with the pane strip under it (as the pull request modal).
const wideMin = 120

type model struct {
	w, h    int
	dark    bool
	fixed   bool // dark/light forced by PROTO_LIGHT or PROTO_DARK
	variant int  // 1..3
	focus   panes.Pane
	zoom    panes.Zoom
	tab     int
	sel     int // index in vis()
	job     int // job index of the selected run
	logOff  int // lines from the bottom of the log
	prevOff int // lines from the top of the preview

	cfg    config.Config
	th     ui.Theme
	ic     ui.Icons
	rs     ui.RunStyles
	frames panes.Styles
}

func newModel() *model {
	m := &model{w: 120, h: 40, dark: true, variant: 1, focus: 1, cfg: config.Default()}
	switch {
	case os.Getenv("PROTO_LIGHT") != "":
		m.dark, m.fixed = false, true
	case os.Getenv("PROTO_DARK") != "":
		m.fixed = true
	}
	m.restyle()
	m.reset()
	return m
}

func (m *model) restyle() {
	p, err := m.cfg.Palette(m.dark)
	if err != nil {
		p, _ = config.Default().Palette(m.dark)
	}
	m.th = ui.NewTheme(p, m.dark)
	set := m.cfg.UI.Icons
	if s := os.Getenv("PROTO_ICONS"); s != "" {
		set = s
	}
	m.ic = ui.NewIcons(set)
	m.rs = ui.NewRunStyles(m.th, m.ic)
	m.frames = panes.DefaultStyles(m.dark)
	m.frames.Border = m.ic.Border
	m.frames.Ellipsis = m.ic.Ellipsis
}

func (m *model) Init() tea.Cmd { return tea.RequestBackgroundColor }

// reset points the job and the scrolls at the selected run's failing job.
func (m *model) reset() {
	m.logOff, m.prevOff = 0, 0
	m.job = 0
	if r, ok := m.cur(); ok {
		if ji, _ := r.failing(); ji >= 0 {
			m.job = ji
		}
	}
}

func (m *model) cur() (run, bool) {
	v := m.vis()
	if m.sel < 0 || m.sel >= len(v) {
		return run{}, false
	}
	return v[m.sel], true
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		if !m.fixed {
			m.dark = msg.IsDark()
			m.restyle()
		}
	case tea.KeyPressMsg:
		return m.key(msg.String())
	}
	return m, nil
}

func (m *model) key(k string) (tea.Model, tea.Cmd) {
	n := len(m.defs())
	switch k {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "1", "2", "3":
		m.variant = int(k[0] - '0')
		m.focus, m.tab, m.sel = 1, 0, 0
		m.reset()
	case "tab":
		m.focus = m.focus%panes.Pane(n) + 1
	case "shift+tab":
		m.focus = (m.focus+panes.Pane(n)-2)%panes.Pane(n) + 1
	case "z":
		if m.zoom.CanZoom() {
			m.zoom.Toggle()
		}
	case "]", "[":
		t := len(m.tabs())
		d := 1
		if k == "[" {
			d = t - 1
		}
		m.tab, m.sel = (m.tab+d)%t, 0
		m.reset()
	case "j", "down", "k", "up":
		d := 1
		if k == "k" || k == "up" {
			d = -1
		}
		m.move(d)
	case "g":
		m.move(-1000)
	case "G":
		m.move(1000)
	}
	return m, nil
}

// move steps the list or scrolls the pane that has the focus.
func (m *model) move(d int) {
	r, ok := m.cur()
	if !ok {
		return
	}
	switch {
	case m.focus == 1:
		m.sel = max(0, min(len(m.vis())-1, m.sel+d))
		m.reset()
	case m.variant == 1 && m.focus == 2:
		m.job = max(0, min(len(r.jobs)-1, m.job+d))
		m.logOff = 0
	case m.variant == 1:
		m.logOff = max(0, m.logOff-d)
	default:
		m.prevOff = max(0, m.prevOff+d)
	}
}

func (m *model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// render is the modal: a header, the panes and a footer, as the pull request
// modal composes its frame.
func (m *model) render() string {
	m.zoom.SetNarrow(m.w < wideMin)
	l, strip := m.layout()
	lines := []string{m.header()}
	out := m.frames.Draw(l, func(b panes.Box) panes.Content { return m.content(b, l) })
	if strip {
		out += "\n" + m.strip()
	}
	lines = append(lines, out, m.footer(l))
	return strings.Join(lines, "\n")
}

func (m *model) layout() (panes.Layout, bool) {
	defs := m.defs()
	if m.focus < 1 || int(m.focus) > len(defs) {
		m.focus = 1
	}
	h := max(m.h-2, 0)
	strip := m.zoom.Zoomed() && m.h >= 4
	if strip {
		h--
	}
	if m.zoom.Zoomed() {
		return panes.Compute(panes.Spec{Width: m.w, Height: h, Panes: defs, Focus: m.focus, Zoom: true}), strip
	}
	l := panes.Layout{Width: m.w, Height: h, Focus: m.focus, Panes: defs}
	var ws []int
	if m.variant == 1 {
		w1, w2 := m.w/3, m.w/4
		ws = []int{w1, w2, m.w - w1 - w2}
	} else {
		ws = []int{panes.DefaultListWidth, m.w - panes.DefaultListWidth}
	}
	x := 0
	for i, w := range ws {
		l.Boxes = append(l.Boxes, panes.Box{Pane: panes.Pane(i + 1), Rect: panes.Rect{X: x, W: w, H: h}})
		x += w
	}
	return l, false
}

func sealed(line string, width int) string {
	line = ansi.Truncate(line, width, "")
	if pad := width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line + "\x1b[m"
}

func (m *model) header() string {
	s := m.th.Text.Bold(true).Render("Actions") + m.th.Subtle.Render(m.ic.Separator) + m.th.Muted.Render("eggzec/gh-tui") + "  "
	counts := m.tabCounts()
	for i, t := range m.tabs() {
		label := fmt.Sprintf("%s %d", t, counts[i])
		if i == m.tab {
			s += m.th.Accent.Bold(true).Underline(true).Render(label)
		} else {
			s += m.th.Subtle.Render(label)
		}
		s += "  "
	}
	return sealed(s, m.w)
}

func (m *model) strip() string {
	for _, f := range m.frames.StripForms(m.defs(), m.focus) {
		if ansi.StringWidth(f) <= m.w {
			return sealed(f, m.w)
		}
	}
	return sealed("", m.w)
}

func (m *model) footer(l panes.Layout) string {
	hint := func(keys, what string) string {
		return m.th.Muted.Render(keys) + " " + m.th.Subtle.Render(what)
	}
	var parts []string
	if l.Zoomed {
		parts = append(parts, hint("tab", "other pane"))
	} else {
		parts = append(parts, hint("tab", "focus"))
	}
	if m.zoom.CanZoom() {
		what := "zoom"
		if l.Zoomed {
			what = "unzoom"
		}
		parts = append(parts, hint("z", what))
	}
	parts = append(parts, hint("[ ]", "tab"), hint("j k", "move"))
	parts = append(parts, hint("1 2 3", fmt.Sprintf("variant %d: %s", m.variant, variantNames[m.variant-1])))
	parts = slices.DeleteFunc(parts, func(p string) bool { return p == "" })
	return sealed(strings.Join(parts, m.th.Subtle.Render(m.ic.Separator)), m.w)
}

// titleForms is the title, then the title cut to the room the border leaves.
func (m *model) titleForms(title string, b panes.Box) []string {
	room := b.Rect.W - 5
	if room <= 0 || ansi.StringWidth(title) <= room {
		return []string{title}
	}
	return []string{title, ansi.Truncate(title, room, m.ic.Ellipsis)}
}

// row is left and right-aligned right in w cells.
func (m *model) row(left, right string, w int) string {
	rw := ansi.StringWidth(right)
	if rw > 0 {
		rw++
	}
	left = ansi.Truncate(left, max(0, w-rw), m.ic.Ellipsis)
	gap := w - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// cursor is the row's gutter: accent when the pane has the focus.
func (m *model) cursor(selected, focus bool) string {
	switch {
	case selected && focus:
		return m.th.Accent.Render(m.ic.Cursor) + " "
	case selected:
		return m.th.Subtle.Render(m.ic.Cursor) + " "
	}
	return "  "
}

// window shows h of lines with line sel in view.
func window(lines []string, sel, h int) []string {
	if len(lines) <= h {
		return lines
	}
	start := max(0, min(len(lines)-h, sel-h/2))
	return lines[start : start+h]
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
