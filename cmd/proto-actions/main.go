// proto-actions: throwaway prototype of a redesigned Actions modal over
// hardcoded data, drawn with pkg/bubbles/panes and the app's own theme.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/filepicker"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/logview"
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
	ov        int      // 0 none, 1 group, 2 filter, 3 sort, 4 download directory
	modal     ui.Modal // the group, filter or sort overlay
	fp        filepicker.Model

	openRun map[int]bool
	openGrp map[string]bool
	preview bool // the annotations pane previews the selected job's log

	asel   int
	selKey string

	view    int // 0 runs, 1 a job's log, 2 the workflow file
	logRun  int
	logJob  int
	pg      pager.Model // the workflow file
	lv      logview.Model
	llines  []logview.Line // the job's log lines, without step markers
	lraw    []logview.Line // the same with a marker line per step
	lsecs   []logview.Section
	raw     bool // the flat timestamped log, no folds
	mk      int  // the step [ ] is on
	curStep int

	dsel         int          // the downloads popup's cursor
	dpick        map[int]bool // artifacts marked in the popup
	sortRev      bool
	dwhat        string
	pend         []tea.Cmd
	confirm      string
	confirmCount int
	fltOff       bool // the filter's criteria are kept but not applied

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
	case pager.CloseMsg, logview.CloseMsg:
		return m, tea.Quit
	}
	if m.modal != nil {
		return m, m.modalUpdate(msg)
	}
	if m.ov == 4 {
		var cmd tea.Cmd
		m.fp, cmd = m.fp.Update(msg)
		return m, cmd
	}
	switch m.view {
	case 1:
		var cmd tea.Cmd
		m.lv, cmd = m.lv.Update(msg)
		return m, cmd
	case 2:
		var cmd tea.Cmd
		m.pg, cmd = m.pg.Update(msg)
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
	case m.ov == 4:
		return m, m.pickerKey(msg)
	case m.ov == 5:
		m.dlKey(k)
		return m, nil
	case m.modal != nil:
		return m, m.modalUpdate(msg)
	case m.view != 0:
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
	case "d":
		m.openDownloads()
	case "z":
		m.zoom.Toggle()
	case "v":
		if ri, _, ok := m.ctx(); ok && m.focus == 1 {
			m.openWorkflow(ri)
			break
		}
		m.preview = !m.preview
	case "1", "2":
		m.focus = panes.Pane(k[0] - '0')
	case "tab", "shift+tab":
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

// rightKey: enter on an annotation opens its job's log.
func (m *model) rightKey(k string) {
	if k != "enter" {
		return
	}
	ri, _, ok := m.ctx()
	if !ok {
		return
	}
	as := m.annos()
	if len(as) > 0 {
		a := as[min(m.asel, len(as)-1)]
		m.openLog(ri, a.job, a.step)
	}
}

func (m *model) move(d int) {
	if m.focus == 1 {
		m.sel = max(0, min(len(m.nodes())-1, m.sel+d))
		return
	}
	m.asel = max(0, min(len(m.annos())-1, m.asel+d))
}

// logKey is the job's log: the real log view with step folds, plus the ':'
// line, the raw toggle and the step jumps.
func (m *model) logKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.String()
	if m.view == 2 {
		return m.workflowKey(msg)
	}
	if !m.lv.Capturing() {
		switch k {
		case "backspace":
			m.view = 0
			return m, nil
		case ":":
			m.cmdOn, m.cmd = true, ""
			return m, nil
		case "x":
			m.toggleRaw()
			return m, nil
		case "]", "[":
			if len(m.lsecs) == 0 || m.raw {
				return m, nil
			}
			d := 1
			if k == "[" {
				d = -1
			}
			m.mk = max(0, min(len(m.lsecs)-1, m.mk+d))
			m.curStep = m.mk
			m.lv.FocusSection(m.mk)
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.lv, cmd = m.lv.Update(msg)
	return m, cmd
}

// workflowKey is the workflow file in the pager.
func (m *model) workflowKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.pg.Capturing() {
		var cmd tea.Cmd
		m.pg, cmd = m.pg.Update(msg)
		return m, cmd
	}
	switch msg.String() {
	case "backspace":
		m.view = 0
		return m, nil
	case ":":
		m.cmdOn, m.cmd = true, ""
		return m, nil
	case "esc":
		cmd, ok := m.pg.ClearTransient()
		if !ok {
			m.view = 0
			return m, nil
		}
		return m, cmd
	}
	var cmd tea.Cmd
	m.pg, cmd = m.pg.Update(msg)
	return m, cmd
}

// toggleRaw flips between the steps as folds and the flat timestamped log.
func (m *model) toggleRaw() {
	m.raw = !m.raw
	if m.raw {
		m.lv.SetLines(m.lraw, nil)
		m.lv.SetTimeMode(logview.TimeAbsolute)
		return
	}
	m.lv.SetLines(m.llines, m.lsecs)
	m.lv.SetTimeMode(logview.TimeHidden)
	m.focusStep()
}

// focusStep opens the current step alone, or the failing one first.
func (m *model) focusStep() {
	r := allRuns[m.logRun]
	if r.jobs[m.logJob].status == "failure" && m.curStep == m.failStep() {
		m.lv.FocusFailed()
		return
	}
	m.lv.FocusSection(m.curStep)
}

func (m *model) failStep() int {
	_, s := allRuns[m.logRun].failing()
	return s
}

func (m *model) openLog(ri, ji, step int) {
	m.view, m.logRun, m.logJob, m.raw = 1, ri, ji, false
	r := allRuns[ri]
	if step < 0 {
		_, step = r.failing()
		if r.jobs[ji].status != "failure" {
			step = 0
		}
	}
	m.curStep, m.mk = step, step
	m.llines, m.lraw, m.lsecs = nil, nil, nil
	a := m.attempt(ri)
	m.lraw = append(m.lraw, logview.Line{Text: fmt.Sprintf("attempt %d/%d  %s", a, attempts(r), r.jobs[ji].name)})
	for _, row := range r.logRows(r.jobs[ji]) {
		if row.marker {
			m.lsecs = append(m.lsecs, logview.Section{Title: row.text, Start: len(m.llines),
				Failed: row.status == "failure"})
			m.lraw = append(m.lraw, logview.Line{Text: row.text, Kind: logview.Group})
			continue
		}
		ln := logview.Line{Text: row.text}
		if t, err := time.Parse("15:04:05.000Z", row.ts); err == nil {
			ln.Time = t
		}
		if after, ok := strings.CutPrefix(row.text, "##[error]"); ok {
			ln.Text, ln.Kind = after, logview.Error
		}
		m.llines = append(m.llines, ln)
		m.lraw = append(m.lraw, ln)
	}
	for i := range m.lsecs {
		end := len(m.llines)
		if i+1 < len(m.lsecs) {
			end = m.lsecs[i+1].Start
		}
		m.lsecs[i].End = end
	}
	m.lv = logview.New(logview.WithKeyMap(m.logKeys()), logview.WithLineNumbers(true),
		logview.WithTimeMode(logview.TimeHidden))
	m.lv.SetLines(m.llines, m.lsecs)
	m.lv.Focus()
	m.focusStep()
}

// logKeys is the real log pane's keys, minus the ones that close the view.
func (m *model) logKeys() logview.KeyMap {
	k := logview.NewKeyMap(ui.In(m.cfg.Keys, "actions_log"))
	k.Quit.SetEnabled(false)
	k.Dismiss.SetEnabled(false)
	return k
}

// openWorkflow shows the workflow file of run ri in the pager, read-only.
func (m *model) openWorkflow(ri int) {
	m.view, m.logRun = 2, ri
	m.pg = pager.New(pager.WithKeyMap(pager.NewKeyMap(ui.In(m.cfg.Keys, "text"))), pager.WithEditor(m.cfg.Editor))
	m.pg.SetLineNumbers(true)
	m.pg.Focus()
	m.pend = append(m.pend, m.pg.SetContent(strings.ToLower(allRuns[ri].workflow)+".yml", allRuns[ri].workflowYAML()))
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
	if m.view != 0 {
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
	l := panes.Layout{Width: m.w, Height: h, Focus: m.focus, Panes: defs}
	w1 := m.runsWidth()
	if m.zoom.Zoomed() {
		return panes.Compute(panes.Spec{Width: m.w, Height: h, Panes: defs, Focus: m.focus, Zoom: true}), strip
	}
	l.Boxes = []panes.Box{
		{Pane: 1, Rect: panes.Rect{W: w1, H: h}},
		{Pane: 2, Rect: panes.Rect{X: w1, W: m.w - w1, H: h}},
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
		return sealed(hint("j k", "move")+"  "+hint("enter l", "open")+"  "+hint("h backspace", "up")+"  "+hint("c", "choose this directory")+"  "+hint("esc", "cancel"), m.w)
	}
	if m.ov == 5 {
		return sealed(hint("j k", "move")+"  "+hint("space", "mark")+"  "+hint("a", "all")+"  "+hint("enter", "choose directory")+"  "+hint("esc", "close"), m.w)
	}
	if m.ov != 0 {
		var hs []string
		for _, b := range m.overlayHints() {
			if b.Enabled() && b.Help().Key != "" {
				hs = append(hs, hint(b.Help().Key, b.Help().Desc))
			}
		}
		return sealed(strings.Join(hs, m.th.Subtle.Render(m.ic.Separator)), m.w)
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
	case m.view == 2:
		parts = []string{hint("j k", "scroll"), hint("/ n N", "search"), hint("backspace", "back"), hint("esc", "clear/close")}
	case m.view == 1:
		parts = []string{hint("enter", "fold step"), hint("*", "fold all"), hint("x", "raw/steps"), hint("] [", "step"), hint("/", "search"), hint(":n :p", "attempt"), hint("- S N T", "options"), hint("backspace", "back")}
	case m.graph && m.focus == 2:
		parts = []string{hint("arrows", "scroll"), hint("l", "LR/TD"), hint("G", "close"), hint("tab", "pane")}
	default:
		parts = []string{hint("tab", "pane"), hint("enter", "open"), hint("d", "downloads"), hint("z", "zoom"), hint("g", "group"), hint("alt+g", "ungroup"), hint("f", "filter"), hint("alt+f", "filter on/off"), hint("s", "sort"), hint("alt+s", "reverse"), hint("G", "graph"), hint(":n :p", "attempt")}
		if m.focus == 1 {
			parts = append(parts, hint("v", "workflow file"))
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
