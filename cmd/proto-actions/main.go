// proto-actions: throwaway prototype of a redesigned Actions modal over
// hardcoded data, drawn with pkg/bubbles/panes and the app's own theme.
// Keys 1, 2 and 3 pick the pane structure (see variants.go).
package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

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
	w, h     int
	dark     bool
	fixed    bool // dark/light forced by PROTO_LIGHT or PROTO_DARK
	focus    panes.Pane
	zoom     panes.Zoom
	tab      int
	sel      int // index in nodes()
	graph    bool // right pane shows the dependency graph
	gsrc     bool // ... as mermaid source
	gdir     string
	gx, gy   int
	gcache   map[string][]string
	openRun  map[int]bool
	openGrp  map[string]bool
	sect     int // 0 log, 1 problems, 2 artifacts
	raw      bool
	lsel     [3]int // selected row per section
	rawSel   int
	jobKey   string
	stepOpen map[string]bool
	dl       string // download in progress or just done
	prog     int

	cfg    config.Config
	th     ui.Theme
	ic     ui.Icons
	rs     ui.RunStyles
	frames panes.Styles
}

func newModel() *model {
	m := &model{w: 120, h: 40, dark: true, focus: 1, gdir: "LR", cfg: config.Default(),
		openRun: map[int]bool{allRuns[0].id: true}, openGrp: map[string]bool{fmt.Sprint(allRuns[0].id, "/test"): true}, stepOpen: map[string]bool{}}
	switch {
	case os.Getenv("PROTO_LIGHT") != "":
		m.dark, m.fixed = false, true
	case os.Getenv("PROTO_DARK") != "":
		m.fixed = true
	}
	m.restyle()
	m.selectFailing()
	m.sync()
	return m
}

type tickMsg struct{}

func tick() tea.Cmd {
	return tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
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

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		if !m.fixed {
			m.dark = msg.IsDark()
			m.restyle()
		}
	case tickMsg:
		if m.dl == "" {
			return m, nil
		}
		m.prog += 20
		if m.prog > 160 {
			m.dl, m.prog = "", 0
			return m, nil
		}
		return m, tick()
	case tea.KeyPressMsg:
		mm, cmd := m.key(msg.String())
		m.sync()
		return mm, cmd
	}
	return m, nil
}

func (m *model) download(what string) tea.Cmd {
	m.dl, m.prog = what, 0
	return tick()
}

func (m *model) key(k string) (tea.Model, tea.Cmd) {
	n := len(m.defs())
	ns := m.nodes()
	if m.graph && m.focus == 2 {
		switch k {
		case "left", "h":
			m.gx -= 4
			return m, nil
		case "right", "l":
			m.gx += 4
			return m, nil
		case "up", "k":
			m.gy--
			return m, nil
		case "down", "j":
			m.gy++
			return m, nil
		case "m":
			m.gsrc, m.gx, m.gy = !m.gsrc, 0, 0
			return m, nil
		case "o":
			m.gdir = map[string]string{"LR": "TD", "TD": "LR"}[m.gdir]
			m.gx, m.gy = 0, 0
			return m, nil
		case "p", "a", "t", "d", "D":
			return m, nil
		}
	}
	switch k {
	case "ctrl+c", "q", "esc":
		return m, tea.Quit
	case "l", "p", "a":
		m.sect = strings.Index("lpa", k)
	case "t":
		m.sect, m.raw = 0, !m.raw
	case "d", "D":
		r, _, ok := m.selJob()
		if !ok || len(r.artifacts()) == 0 {
			break
		}
		m.sect = 2
		if k == "D" {
			return m, m.download(fmt.Sprintf("all %d artifacts", len(r.artifacts())))
		}
		return m, m.download(r.artifacts()[m.lsel[2]].name)
	case "g":
		m.graph, m.gsrc, m.gx, m.gy = !m.graph, false, 0, 0
		if m.graph {
			m.focus = 2
		}
	case "right", "enter", "left":
		if m.focus == 2 {
			m.enterRight()
			break
		}
		if m.sel >= len(ns) {
			break
		}
		nd := ns[m.sel]
		r := allRuns[nd.run]
		key := fmt.Sprint(r.id, "/", nd.group)
		switch {
		case nd.kind == kRun && k == "left":
			delete(m.openRun, r.id)
		case nd.kind == kRun && k == "enter" && m.openRun[r.id]:
			delete(m.openRun, r.id)
		case nd.kind == kRun:
			m.openRun[r.id] = true
		case nd.kind == kGroup && k == "left", nd.kind == kGroup && k == "enter" && m.openGrp[key]:
			delete(m.openGrp, key)
		case nd.kind == kGroup:
			m.openGrp[key] = true
		case nd.kind == kJob && k == "left" && nd.group != "":
			delete(m.openGrp, key)
			for i, x := range m.nodes() {
				if x.kind == kGroup && x.run == nd.run && x.group == nd.group {
					m.sel = i
				}
			}
		case nd.kind == kJob && k == "enter":
			m.focus = 2
		}
	case "tab":
		m.focus = m.focus%panes.Pane(n) + 1
	case "shift+tab":
		m.focus = (m.focus+panes.Pane(n)-2)%panes.Pane(n) + 1
	case "z":
		if m.zoom.CanZoom() {
			m.zoom.Toggle()
		}
	case "]", "[":
		t := len(tabNames)
		d := 1
		if k == "[" {
			d = t - 1
		}
		m.tab, m.sel = (m.tab+d)%t, 0
	case "j", "down", "k", "up":
		d := 1
		if k == "k" || k == "up" {
			d = -1
		}
		m.move(d, len(ns))
	}
	return m, nil
}

// enterRight acts on the selected row of the right pane.
func (m *model) enterRight() {
	r, ji, ok := m.selJob()
	if !ok {
		return
	}
	switch m.sect {
	case 0:
		if !m.raw {
			k := fmt.Sprint(r.id, "/", ji, "/", m.lsel[0])
			m.stepOpen[k] = !m.stepOpened(r, ji, m.lsel[0])
		}
	case 1:
		if as := r.annotations(r.jobs[ji]); len(as) > 0 {
			a := as[m.lsel[1]]
			m.sect, m.raw, m.lsel[0] = 0, false, a.step
			m.stepOpen[fmt.Sprint(r.id, "/", ji, "/", a.step)] = true
		}
	}
}

// move steps the tree, or the selected row of the right pane's section.
func (m *model) move(d, nodes int) {
	if m.focus == 1 {
		m.sel = max(0, min(nodes-1, m.sel+d))
		return
	}
	r, ji, ok := m.selJob()
	if !ok {
		return
	}
	j := r.jobs[ji]
	size := []int{len(j.steps), len(r.annotations(j)), len(r.artifacts())}[m.sect]
	if m.sect == 0 && m.raw {
		m.rawSel = max(0, min(59, m.rawSel+d))
		return
	}
	m.lsel[m.sect] = max(0, min(size-1, m.lsel[m.sect]+d))
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
	w1 := max(panes.DefaultListWidth, m.w*2/5)
	ws := []int{w1, m.w - w1}
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
	for i, t := range tabNames {
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
	if m.graph {
		parts = append(parts, hint("hjkl", "scroll"), hint("m", "mermaid"), hint("o", "LR/TD"), hint("g", "back"))
	} else {
		parts = append(parts, hint("[ ]", "tab"), hint("j k", "move"), hint("l p a", "section"), hint("g", "graph"))
	}
	parts = slices.DeleteFunc(parts, func(p string) bool { return p == "" })
	if m.dl != "" {
		const n = 8
		done := min(n, m.prog*n/100)
		bar := strings.Repeat("#", done) + strings.Repeat("-", n-done)
		msg := fmt.Sprintf("Downloading %s [%s] %d%%", m.dl, bar, min(100, m.prog))
		if m.prog >= 100 {
			msg = "Downloaded " + m.dl
		}
		return sealed(m.th.Accent.Render(msg), m.w)
	}
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
