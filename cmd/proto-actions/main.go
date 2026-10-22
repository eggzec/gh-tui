// proto-actions: throwaway prototype of a redesigned Actions modal over
// hardcoded data, drawn with pkg/bubbles/panes and the app's own theme.
package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/filepicker"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
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
	flt       map[string][]string // facet -> the values it may have
	sortBy    string
	ov        int // 0 none, 1 group, 2 filter, 3 sort, 4 download directory
	op        [2]opane
	ofocus    int  // overlay pane with the focus
	osearch   bool // typing a search in the overlay pane
	fp        filepicker.Model

	openRun map[int]bool
	openGrp map[string]bool
	preview bool // the annotations pane previews the selected job's log

	asel, dsel int
	selKey     string

	view         int // 0 runs, 1 a job's log in the pager
	logRun       int
	logJob       int
	pg           pager.Model
	marks        []int // line of each step marker in the pager
	mk           int
	needJump     bool
	curStep      int
	sortRev      bool
	dwhat        string
	pend         []tea.Cmd
	confirm      string
	confirmCount int

	fltOff bool // the filter's criteria are kept but not applied

	att map[int]int // run id -> the re-run attempt shown, 1..attempts

	cmdOn bool // the ':' prompt in the foot is open
	cmd   string
	toast string

	graph  bool // right pane shows the dependency graph
	gdir   string
	gx, gy int
	gcache map[string][]string

	dl    string // download in progress or just done
	dldir string
	prog  int

	cfg    config.Config
	th     ui.Theme
	ic     ui.Icons
	rs     ui.RunStyles
	frames panes.Styles
}

func newModel() *model {
	m := &model{w: 120, h: 40, dark: true, focus: 1, gdir: "LR", cfg: config.Default(), att: map[int]int{},
		groupBy: "workflow", sortBy: "newest", collapsed: map[string]bool{}, flt: map[string][]string{},
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
		mm, cmd := m.key(msg)
		m.sync()
		if len(m.pend) > 0 {
			cmd = tea.Batch(append(m.pend, cmd)...)
			m.pend = nil
		}
		return mm, cmd
	case pager.CloseMsg:
		return m, tea.Quit
	}
	if m.ov == 4 {
		var cmd tea.Cmd
		m.fp, cmd = m.fp.Update(msg)
		return m, cmd
	}
	if m.view == 1 {
		var cmd tea.Cmd
		m.pg, cmd = m.pg.Update(msg)
		if m.needJump && m.pg.Lines() > 0 && len(m.marks) > 0 {
			m.needJump = false
			m.jump(m.marks[m.mk])
		}
		return m, cmd
	}
	return m, nil
}

func (m *model) download(what string) tea.Cmd {
	m.dl, m.prog = what, 0
	return tick()
}

// attempts is how many times the run was run: a failing run was re-run twice.
func attempts(r run) int {
	if r.status == "failure" {
		return 3
	}
	return 1
}

// attempt is the re-run shown for the run.
func (m *model) attempt(ri int) int {
	n := attempts(allRuns[ri])
	if a, ok := m.att[allRuns[ri].id]; ok {
		return min(a, n)
	}
	return n
}

// runCmd runs a ':' line.
func (m *model) runCmd(c string) {
	d := 0
	switch strings.TrimSpace(c) {
	case "n":
		d = 1
	case "p":
		d = -1
	case "":
		return
	default:
		m.toast = "not a command: " + c
		return
	}
	ri := -1
	if m.view == 1 {
		ri = m.logRun
	} else if r, _, ok := m.ctx(); ok {
		ri = r
	}
	if ri < 0 {
		return
	}
	n, a := attempts(allRuns[ri]), m.attempt(ri)
	switch {
	case a+d > n:
		m.toast = fmt.Sprintf("no next attempt (%d/%d)", a, n)
	case a+d < 1:
		m.toast = fmt.Sprintf("no previous attempt (%d/%d)", a, n)
	default:
		m.att[allRuns[ri].id] = a + d
		if m.view == 1 {
			m.openLog(m.logRun, m.logJob, m.curStep)
		}
	}
}

func (m *model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	m.toast = ""
	switch {
	case m.cmdOn:
		switch k {
		case "esc":
			m.cmdOn = false
		case "enter":
			m.cmdOn = false
			m.runCmd(m.cmd)
		case "backspace":
			if m.cmd == "" {
				m.cmdOn = false
			} else {
				m.cmd = m.cmd[:len(m.cmd)-1]
			}
		default:
			if len(k) == 1 {
				m.cmd += k
			}
		}
		return m, nil
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
		return m.logKey(msg)
	}
	if k == ":" && m.focus == 1 {
		m.cmdOn, m.cmd = true, ""
		return m, nil
	}
	if m.graph && m.focus == 2 {
		switch k {
		case "left":
			m.gx -= 4
			return m, nil
		case "right":
			m.gx += 4
			return m, nil
		case "up":
			m.gy--
			return m, nil
		case "down":
			m.gy++
			return m, nil
		case "l":
			m.gdir = map[string]string{"LR": "TD", "TD": "LR"}[m.gdir]
			m.gx, m.gy = 0, 0
			return m, nil
		case "h", "j", "k":
			return m, nil
		}
	}
	switch k {
	case "q":
		return m, tea.Quit
	case "esc":
		if len(m.flt) > 0 || m.fltOff {
			m.flt, m.fltOff, m.sel = map[string][]string{}, false, 0
			break
		}
		return m, tea.Quit
	case "alt+f":
		m.fltOff, m.sel = !m.fltOff, 0
	case "alt+s":
		m.sortRev, m.sel = !m.sortRev, 0
	case "g":
		m.openOv(1)
	case "alt+g":
		m.groupOff, m.sel = !m.groupOff, 0
	case "f":
		m.openOv(2)
	case "s":
		m.openOv(3)
	case "F":
		m.flt, m.fltOff, m.sel = map[string][]string{}, false, 0
	case "*":
		m.toggleAll()
	case "G":
		m.graph, m.gx, m.gy = !m.graph, 0, 0
		if m.graph {
			m.focus = 2
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
		m.focus = 3
		m.confirmCount = len(ar)
		if k == "D" {
			m.dwhat = fmt.Sprintf("all %d downloads", len(ar))
		} else {
			m.dwhat = ar[min(m.dsel, len(ar)-1)].name
			m.confirmCount = 1
		}
		m.openPicker()
	case "z":
		m.zoom.Toggle()
	case "v":
		m.preview = !m.preview
	case "1", "2", "3":
		m.focus = panes.Pane(k[0] - '0')
	case "tab":
		m.focus = m.focus%3 + 1
	case "shift+tab":
		m.focus = (m.focus+1)%3 + 1
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
		if m.focus != 1 {
			if m.graph && m.focus == 2 {
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

// rightKey: enter acts on the selected row of the annotations or downloads.
func (m *model) rightKey(k string) {
	if k != "enter" {
		return
	}
	ri, _, ok := m.ctx()
	if !ok {
		return
	}
	if m.focus == 2 {
		as := m.annos()
		if len(as) > 0 {
			a := as[min(m.asel, len(as)-1)]
			m.openLog(ri, a.job, a.step)
		}
		return
	}
	if ar := allRuns[ri].artifacts(); len(ar) > 0 {
		m.dwhat = ar[min(m.dsel, len(ar)-1)].name
		m.confirmCount = 1
		m.openPicker()
	}
}

func (m *model) move(d int) {
	if m.focus == 1 {
		m.sel = max(0, min(len(m.nodes())-1, m.sel+d))
		return
	}
	if m.focus == 2 {
		m.asel = max(0, min(len(m.annos())-1, m.asel+d))
		return
	}
	if ri, _, ok := m.ctx(); ok {
		m.dsel = max(0, min(len(allRuns[ri].artifacts())-1, m.dsel+d))
	}
}

// logKey is the job's log: the pager, plus step markers and the ':' line.
func (m *model) logKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if m.pg.Capturing() {
		var cmd tea.Cmd
		m.pg, cmd = m.pg.Update(msg)
		return m, cmd
	}
	switch k {
	case "backspace":
		m.view = 0
		return m, nil
	case ":":
		m.cmdOn, m.cmd = true, ""
		return m, nil
	case "esc":
		cmd, ok := m.pg.ClearTransient()
		if !ok {
			return m, tea.Quit
		}
		return m, cmd
	case "]", "[":
		if len(m.marks) == 0 {
			return m, nil
		}
		d := 1
		if k == "[" {
			d = -1
		}
		m.mk = max(0, min(len(m.marks)-1, m.mk+d))
		m.jump(m.marks[m.mk])
		return m, nil
	}
	var cmd tea.Cmd
	m.pg, cmd = m.pg.Update(msg)
	return m, cmd
}

// jump scrolls the pager to a line by keys: top, then down.
func (m *model) jump(line int) {
	m.pg, _ = m.pg.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	for range line {
		m.pg, _ = m.pg.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
}

func (m *model) openLog(ri, ji, step int) {
	m.view, m.logRun, m.logJob = 1, ri, ji
	r := allRuns[ri]
	if step < 0 {
		_, step = r.failing()
		if r.jobs[ji].status != "failure" {
			step = 0
		}
	}
	m.curStep = step
	a := m.attempt(ri)
	lines := []string{fmt.Sprintf("##[attempt %d/%d] %s", a, attempts(r), r.jobs[ji].name)}
	m.marks = nil
	m.mk = 0
	for _, row := range r.logRows(r.jobs[ji]) {
		if row.marker {
			if row.step == step {
				m.mk = len(m.marks)
			}
			m.marks = append(m.marks, len(lines))
			lines = append(lines, "##[group]"+row.text)
			continue
		}
		lines = append(lines, row.ts+" "+row.text)
	}
	m.pg = pager.New(pager.WithKeyMap(pager.NewKeyMap(ui.In(m.cfg.Keys, "text"))), pager.WithEditor(m.cfg.Editor))
	m.pg.SetLineNumbers(true)
	m.pg.Focus()
	m.pend = append(m.pend, m.pg.SetContent("job.log", strings.Join(lines, "\n")))
	m.needJump = true
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
	case 4:
		title = "Download " + m.dwhat + " to"
		hint = "j k move  enter open  backspace up  c choose  esc cancel"
		rows = append(rows, m.th.Accent.Render(m.dpath))
		for i, d := range subdirs(m.dpath) {
			rows = append(rows, m.cursor(i == m.ovSel, true)+m.th.Text.Render(d+"/"))
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
	if m.ov == 4 {
		ow = min(m.w, 64)
	}
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
		return sealed(m.th.Warning.Render("Download "+m.confirm+" to "+m.dldir+"?")+" "+hint("y", "yes")+"  "+hint("any other key", "cancel"), m.w)
	}
	if m.cmdOn {
		return sealed(m.th.Accent.Render(":"+m.cmd+"_")+"  "+hint("enter", "run")+"  "+hint("esc", "cancel"), m.w)
	}
	if m.toast != "" {
		return sealed(m.th.Warning.Render(m.toast), m.w)
	}
	if m.ov == 4 {
		return sealed(hint("j k", "move")+"  "+hint("enter", "open dir")+"  "+hint("backspace", "up")+"  "+hint("c", "choose this directory")+"  "+hint("esc", "cancel"), m.w)
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
		parts = []string{hint("j k", "scroll"), hint("/ n N", "search"), hint("] [", "step"), hint(": n :p", "attempt"), hint("backspace", "back"), hint("esc", "clear/close")}
	case m.graph && m.focus == 2:
		parts = []string{hint("arrows", "scroll"), hint("l", "LR/TD"), hint("G", "close"), hint("tab", "pane")}
	default:
		parts = []string{hint("tab", "pane"), hint("enter", "open"), hint("g", "group"), hint("alt+g", "ungroup"), hint("f", "filter"), hint("alt+f", "filter on/off"), hint("s", "sort"), hint("alt+s", "reverse"), hint("G", "graph"), hint(":n :p", "attempt")}
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
