// proto-actions: throwaway prototype of a redesigned Actions modal over
// hardcoded data, drawn with pkg/bubbles/panes and the app's own theme.
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
	w, h  int
	dark  bool
	fixed bool // dark/light forced by PROTO_LIGHT or PROTO_DARK
	focus panes.Pane
	zoom  panes.Zoom
	tab   int
	sel   int // index in nodes()

	groupBy   string
	groupOff  bool
	collapsed map[string]bool
	flt       map[string]string
	sortBy    string
	ov        int // 0 none, 1 group, 2 filter, 3 sort
	ovSel     int

	openRun map[int]bool
	openGrp map[string]bool
	preview bool // wide: the right pane previews the selected job's log

	rsec       int // right pane list: 0 annotations, 1 downloads
	asel, dsel int
	selKey     string

	view         int // 0 runs, 1 a job's log
	logRun       int
	logJob       int
	rows         []logRow
	logSel       int
	confirm      string
	confirmCount int

	graph  bool // right pane shows the dependency graph
	gsrc   bool // ... as mermaid source
	gdir   string
	gx, gy int
	gcache map[string][]string

	dl   string // download in progress or just done
	prog int

	cfg    config.Config
	th     ui.Theme
	ic     ui.Icons
	rs     ui.RunStyles
	frames panes.Styles
}

func newModel() *model {
	m := &model{w: 120, h: 40, dark: true, focus: 1, gdir: "LR", cfg: config.Default(),
		groupBy: "workflow", sortBy: "newest", collapsed: map[string]bool{}, flt: map[string]string{},
		openRun: map[int]bool{allRuns[0].id: true}, openGrp: map[string]bool{fmt.Sprint(allRuns[0].id, "/test"): true}}
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
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	switch {
	case m.confirm != "":
		what := m.confirm
		m.confirm = ""
		if k == "y" || k == "enter" {
			return m, m.download(what)
		}
		return m, nil
	case m.ov != 0:
		m.overlayKey(k)
		return m, nil
	case m.view == 1:
		return m.logKey(k)
	}
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
		}
	}
	switch k {
	case "q":
		return m, tea.Quit
	case "esc":
		if len(m.flt) > 0 {
			m.flt, m.sel = map[string]string{}, 0
			break
		}
		return m, tea.Quit
	case "g":
		m.ov, m.ovSel = 1, max(0, slices.Index(groups, m.groupBy))
	case "alt+g":
		m.groupOff, m.sel = !m.groupOff, 0
	case "f":
		m.ov, m.ovSel = 2, 0
	case "s":
		m.ov, m.ovSel = 3, max(0, slices.Index(sorts, m.sortBy))
	case "F":
		m.flt, m.sel = map[string]string{}, 0
	case "*":
		m.toggleAll()
	case "m":
		m.graph, m.gsrc, m.gx, m.gy = !m.graph, false, 0, 0
		if m.graph {
			m.focus = 2
		}
	case "M":
		m.graph, m.gsrc, m.gx, m.gy = true, !(m.graph && m.gsrc), 0, 0
		m.focus = 2
	case "o":
		if m.graph {
			m.gdir = map[string]string{"LR": "TD", "TD": "LR"}[m.gdir]
			m.gx, m.gy = 0, 0
		}
	case "d", "D":
		ri, _, ok := m.ctx()
		if !ok {
			break
		}
		ar := allRuns[ri].artifacts()
		if len(ar) == 0 {
			break
		}
		m.rsec = 1
		m.confirmCount = len(ar)
		if k == "D" {
			m.confirm = fmt.Sprintf("all %d downloads", len(ar))
		} else {
			m.confirm = ar[min(m.dsel, len(ar)-1)].name
			m.confirmCount = 1
		}
	case "z":
		if m.w >= wideMin {
			m.preview = !m.preview
		}
	case "tab":
		m.focus = m.focus%2 + 1
	case "shift+tab":
		m.focus = m.focus%2 + 1
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
		m.move(d)
	case "left", "right", "enter":
		if m.focus == 2 {
			if m.graph {
				break
			}
			m.rightKey(k)
			break
		}
		m.treeKey(k)
	}
	return m, nil
}

// treeKey folds and opens rows of the left list.
func (m *model) treeKey(k string) {
	ns := m.nodes()
	if m.sel >= len(ns) {
		return
	}
	nd := ns[m.sel]
	r := allRuns[nd.run]
	key := fmt.Sprint(r.id, "/", nd.group)
	g := m.grouped()
	hk := g + "/" + nd.group
	switch {
	case nd.kind == kHead && k == "left", nd.kind == kHead && k == "enter" && !m.collapsed[hk]:
		m.collapsed[hk] = true
	case nd.kind == kHead:
		delete(m.collapsed, hk)
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
		m.openLog(nd.run, nd.job, -1)
	}
}

// rightKey: left/right switch lists, enter acts on the selected row.
func (m *model) rightKey(k string) {
	switch k {
	case "left":
		m.rsec = 0
	case "right":
		m.rsec = 1
	case "enter":
		ri, _, ok := m.ctx()
		if !ok {
			return
		}
		if m.rsec == 0 {
			as := m.annos()
			if len(as) > 0 {
				a := as[min(m.asel, len(as)-1)]
				m.openLog(ri, a.job, a.step)
			}
			return
		}
		if ar := allRuns[ri].artifacts(); len(ar) > 0 {
			m.confirm = ar[min(m.dsel, len(ar)-1)].name
			m.confirmCount = 1
		}
	}
}

func (m *model) move(d int) {
	if m.focus == 1 {
		m.sel = max(0, min(len(m.nodes())-1, m.sel+d))
		return
	}
	if m.rsec == 0 {
		m.asel = max(0, min(len(m.annos())-1, m.asel+d))
		return
	}
	if ri, _, ok := m.ctx(); ok {
		m.dsel = max(0, min(len(allRuns[ri].artifacts())-1, m.dsel+d))
	}
}

func (m *model) overlayKey(k string) {
	n := []int{0, len(groups), len(facets), len(sorts)}[m.ov]
	switch k {
	case "esc", "q", "backspace":
		m.ov = 0
	case "j", "down":
		m.ovSel = min(n-1, m.ovSel+1)
	case "k", "up":
		m.ovSel = max(0, m.ovSel-1)
	case "right", "l":
		if m.ov == 2 {
			m.cycle(1)
		}
	case "left", "h":
		if m.ov == 2 {
			m.cycle(-1)
		}
	case "enter":
		switch m.ov {
		case 1:
			m.groupBy, m.groupOff, m.ov, m.sel = groups[m.ovSel], false, 0, 0
		case 2:
			m.cycle(1)
		case 3:
			m.sortBy, m.ov, m.sel = sorts[m.ovSel], 0, 0
		}
	}
}

func (m *model) cycle(d int) {
	f := facets[m.ovSel]
	vals := facetValues(f)
	i := max(0, slices.Index(vals, m.flt[f]))
	v := vals[(i+d+len(vals))%len(vals)]
	if v == "any" {
		delete(m.flt, f)
	} else {
		m.flt[f] = v
	}
	m.sel = 0
}

// logKey handles the job's log step of the modal.
func (m *model) logKey(k string) (tea.Model, tea.Cmd) {
	last := len(m.rows) - 1
	switch k {
	case "q", "esc":
		return m, tea.Quit
	case "backspace":
		m.view = 0
	case "j", "down":
		m.logSel = min(last, m.logSel+1)
	case "k", "up":
		m.logSel = max(0, m.logSel-1)
	case "pgdown", "ctrl+d":
		m.logSel = min(last, m.logSel+10)
	case "pgup", "ctrl+u":
		m.logSel = max(0, m.logSel-10)
	case "g":
		m.logSel = 0
	case "G":
		m.logSel = last
	case "]", "[":
		d := 1
		if k == "[" {
			d = -1
		}
		for i := m.logSel + d; i >= 0 && i <= last; i += d {
			if m.rows[i].marker {
				m.logSel = i
				break
			}
		}
	case "e":
		for o := 1; o <= len(m.rows); o++ {
			i := (m.logSel + o) % len(m.rows)
			if m.rows[i].err {
				m.logSel = i
				break
			}
		}
	}
	return m, nil
}

func (m *model) openLog(ri, ji, step int) {
	m.view, m.logRun, m.logJob = 1, ri, ji
	r := allRuns[ri]
	m.rows = r.logRows(r.jobs[ji])
	if step < 0 {
		_, step = r.failing()
		if r.jobs[ji].status != "failure" {
			step = 0
		}
	}
	m.logSel = markerRow(m.rows, step)
}

func markerRow(rows []logRow, step int) int {
	for i, r := range rows {
		if r.marker && r.step == step {
			return i
		}
	}
	return 0
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
	s := strings.Join(lines, "\n")
	if m.ov != 0 {
		s = m.overlay(s)
	}
	return s
}

func (m *model) layout() (panes.Layout, bool) {
	h := max(m.h-2, 0)
	if m.view == 1 {
		return panes.Layout{Width: m.w, Height: h, Focus: 1, Panes: []panes.Def{{Name: "Log", Short: "Log"}},
			Boxes: []panes.Box{{Pane: 1, Rect: panes.Rect{W: m.w, H: h}}}}, false
	}
	defs := m.defs()
	if m.focus < 1 || int(m.focus) > len(defs) {
		m.focus = 1
	}
	strip := m.zoom.Zoomed() && m.h >= 4
	if strip {
		h--
	}
	if m.zoom.Zoomed() {
		return panes.Compute(panes.Spec{Width: m.w, Height: h, Panes: defs, Focus: m.focus, Zoom: true}), strip
	}
	l := panes.Layout{Width: m.w, Height: h, Focus: m.focus, Panes: defs}
	w1 := max(panes.DefaultListWidth, m.w*2/5)
	if m.preview {
		w1 = max(panes.DefaultListWidth, m.w*9/20)
	}
	ws := []int{w1, m.w - w1}
	x := 0
	for i, w := range ws {
		l.Boxes = append(l.Boxes, panes.Box{Pane: panes.Pane(i + 1), Rect: panes.Rect{X: x, W: w, H: h}})
		x += w
	}
	return l, false
}

// overlay draws the group, filter or sort picker over the screen.
func (m *model) overlay(base string) string {
	var title string
	var rows []string
	hint := "j k move  enter pick  esc close"
	switch m.ov {
	case 1:
		title = "Group by"
		for i, g := range groups {
			mark := " "
			if g == m.groupBy {
				mark = m.ic.Cursor
			}
			rows = append(rows, m.cursor(i == m.ovSel, true)+m.th.Accent.Render(mark)+" "+m.th.Text.Render(g))
		}
	case 2:
		title = "Filter"
		hint = "j k row  h l value  esc close"
		for i, f := range facets {
			v := m.flt[f]
			vs := m.th.Subtle.Render("any")
			if v != "" {
				vs = m.th.Accent.Render(v)
			}
			rows = append(rows, m.cursor(i == m.ovSel, true)+m.th.Text.Render(fmt.Sprintf("%-9s", f))+vs)
		}
	default:
		title = "Sort"
		for i, s := range sorts {
			mark := " "
			if s == m.sortBy {
				mark = m.ic.Cursor
			}
			rows = append(rows, m.cursor(i == m.ovSel, true)+m.th.Accent.Render(mark)+" "+m.th.Text.Render(s))
		}
	}
	ow := min(m.w, 38)
	oh := len(rows) + 2
	l := panes.Layout{Width: ow, Height: oh, Focus: 1, Panes: []panes.Def{{Name: title, Short: title}},
		Boxes: []panes.Box{{Pane: 1, Rect: panes.Rect{W: ow, H: oh}}}}
	box := m.frames.Draw(l, func(b panes.Box) panes.Content {
		return panes.Content{TopLeft: []string{title}, BottomLeft: []string{hint, "esc close", ""}, Focus: true, Body: strings.Join(rows, "\n")}
	})
	bl := strings.Split(box, "\n")
	lines := strings.Split(base, "\n")
	x := (m.w - ow) / 2
	y := max(0, (len(lines)-len(bl))/2)
	for i, b := range bl {
		if y+i >= len(lines) {
			break
		}
		old := lines[y+i]
		lines[y+i] = sealed(ansi.Truncate(old, x, ""), x) + b + ansi.Cut(old, x+ow, m.w)
	}
	return strings.Join(lines, "\n")
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
	if m.confirm != "" {
		return sealed(m.th.Warning.Render("Download "+m.confirm+"?")+" "+hint("y", "yes")+"  "+hint("any other key", "cancel"), m.w)
	}
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
	var parts []string
	switch {
	case m.view == 1:
		parts = []string{hint("j k", "move"), hint("] [", "step"), hint("e", "error"), hint("backspace", "back"), hint("esc", "close")}
	case m.graph && m.focus == 2:
		parts = []string{hint("hjkl", "scroll"), hint("M", "mermaid"), hint("o", "LR/TD"), hint("m", "back"), hint("tab", "pane")}
	default:
		parts = []string{hint("tab", "pane"), hint("enter", "open"), hint("g", "group"), hint("f", "filter"), hint("s", "sort"), hint("m", "graph")}
		if m.w >= wideMin {
			parts = append(parts, hint("z", "preview"))
		}
		if m.focus == 2 {
			parts = append(parts, hint("< >", "list"), hint("d D", "download"))
		}
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
