// proto-actions: throwaway prototype of a redesigned Actions modal over
// hardcoded data. Variants live in v1.go, v2.go and v3.go; keys 1, 2, 3 switch.
package main

import (
	"fmt"
	"image/color"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type theme struct {
	green, red, yellow, blue, dim, accent, selBg, selFg color.Color
}

func newTheme(dark bool) theme {
	ld := lipgloss.LightDark(dark)
	c := lipgloss.Color
	return theme{
		green:  ld(c("#1a7f37"), c("#3fb950")),
		red:    ld(c("#cf222e"), c("#ff7b72")),
		yellow: ld(c("#9a6700"), c("#e3b341")),
		blue:   ld(c("#0969da"), c("#79c0ff")),
		dim:    ld(c("#6e7781"), c("#8b949e")),
		accent: ld(c("#8250df"), c("#d2a8ff")),
		selBg:  ld(c("#ddf4ff"), c("#1f3a5f")),
		selFg:  ld(c("#0a3069"), c("#ffffff")),
	}
}

func (t theme) col(c color.Color, s string) string {
	return lipgloss.NewStyle().Foreground(c).Render(s)
}

func (t theme) statusCol(s string) color.Color {
	switch s {
	case "success":
		return t.green
	case "failure":
		return t.red
	case "running":
		return t.yellow
	case "queued":
		return t.blue
	}
	return t.dim
}

func glyph(s string) string {
	switch s {
	case "success":
		return "✓"
	case "failure":
		return "✗"
	case "running":
		return "●"
	case "queued":
		return "○"
	case "cancelled":
		return "⊘"
	}
	return "–"
}

// paint colours s unless the row is selected (selected rows are one plain style).
func (m *model) paint(sel bool, c color.Color, s string) string {
	if sel {
		return s
	}
	return m.th.col(c, s)
}

func (m *model) dim(s string) string { return m.th.col(m.th.dim, s) }

func (m *model) glyph(sel bool, status string) string {
	return m.paint(sel, m.th.statusCol(status), glyph(status))
}

func (m *model) bar(r run, sel bool) string {
	if r.status != "running" {
		return ""
	}
	n := len(r.jobs)
	d := r.doneJobs()
	return m.paint(sel, m.th.yellow, strings.Repeat("▰", d)+strings.Repeat("▱", n-d))
}

func (m *model) selStyle(s string, w int) string {
	return lipgloss.NewStyle().Bold(true).Foreground(m.th.selFg).Background(m.th.selBg).Render(fit(s, w))
}

func trunc(s string, w int) string {
	if w <= 0 {
		return ""
	}
	return lipgloss.NewStyle().MaxWidth(w).Render(s)
}

func fit(s string, w int) string {
	s = trunc(s, w)
	if n := w - lipgloss.Width(s); n > 0 {
		s += strings.Repeat(" ", n)
	}
	return s
}

func block(lines []string, w, h int) string {
	out := make([]string, h)
	for i := range out {
		if i < len(lines) {
			out[i] = fit(lines[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return strings.Join(out, "\n")
}

func rightAlign(l, r string, w int) string {
	wr := lipgloss.Width(r)
	if lipgloss.Width(l)+wr+1 > w {
		l = trunc(l, w-wr-1)
	}
	gap := max(1, w-lipgloss.Width(l)-wr)
	return l + strings.Repeat(" ", gap) + r
}

func window(sel, n, h int) int {
	if h <= 0 {
		return 0
	}
	return max(0, min(sel-h/2, n-h))
}

func (m *model) title(name, count string, focus bool) string {
	s := name + " " + count
	if focus {
		return lipgloss.NewStyle().Bold(true).Foreground(m.th.accent).Render(s)
	}
	return m.dim(s)
}

type clearToast int

type model struct {
	w, h      int
	dark      bool
	th        theme
	variant   int
	help      bool
	toast     string
	toastSeq  int
	filtering bool
	filter    string
	sel       int
	logJob    int
	logScroll int
	v1focus   int
	v1job     int
	exp       map[int]bool
	chip      int
}

var chips = []string{"all", "failure", "running", "queued", "success"}

func rank(s string) int {
	for i, x := range []string{"failure", "running", "queued", "cancelled", "success", "skipped"} {
		if x == s {
			return i
		}
	}
	return 9
}

func wfIndex(w string) int {
	for i, x := range workflows {
		if x == w {
			return i
		}
	}
	return 9
}

func (m *model) vis() []run {
	var out []run
	q := strings.ToLower(m.filter)
	for _, r := range allRuns {
		if q != "" && !strings.Contains(strings.ToLower(r.title+" "+r.branch+" "+r.workflow), q) {
			continue
		}
		if m.variant == 3 && m.chip > 0 && r.status != chips[m.chip] {
			continue
		}
		out = append(out, r)
	}
	switch m.variant {
	case 2:
		sort.SliceStable(out, func(a, b int) bool { return wfIndex(out[a].workflow) < wfIndex(out[b].workflow) })
	case 3:
		sort.SliceStable(out, func(a, b int) bool { return rank(out[a].status) < rank(out[b].status) })
	}
	return out
}

func (m *model) selRun() (run, bool) {
	v := m.vis()
	if len(v) == 0 {
		return run{}, false
	}
	m.sel = max(0, min(m.sel, len(v)-1))
	return v[m.sel], true
}

func (m *model) move(d int) {
	n := len(m.vis())
	m.sel = max(0, min(m.sel+d, n-1))
	m.v1job = 0
}

func (m *model) openLog(ji int) {
	m.logJob = ji
	m.logScroll = 1 << 30
}

func (m *model) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tea.BackgroundColorMsg:
		m.dark = msg.IsDark()
		m.th = newTheme(m.dark)
	case clearToast:
		if int(msg) == m.toastSeq {
			m.toast = ""
		}
	case tea.KeyPressMsg:
		return m.key(msg.String())
	}
	return m, nil
}

func (m *model) key(k string) (tea.Model, tea.Cmd) {
	if k == "ctrl+c" {
		return m, tea.Quit
	}
	if m.filtering {
		switch k {
		case "enter":
			m.filtering = false
		case "esc":
			m.filtering, m.filter = false, ""
		case "backspace":
			if m.filter == "" {
				m.filtering = false
			} else {
				r := []rune(m.filter)
				m.filter = string(r[:len(r)-1])
			}
		case "space":
			m.filter += " "
		default:
			if len([]rune(k)) == 1 {
				m.filter += k
			}
		}
		m.sel = 0
		return m, nil
	}
	switch k {
	case "q":
		return m, tea.Quit
	case "1", "2", "3":
		m.variant = int(k[0] - '0')
		m.logJob = -1
		m.v1focus = 0
		return m, nil
	case "?":
		m.help = !m.help
		return m, nil
	case "/":
		m.filtering = true
		return m, nil
	case "r":
		if r, ok := m.selRun(); ok {
			m.toastSeq++
			m.toast = fmt.Sprintf("↻ rerun of #%d queued (fake)", r.id)
			seq := m.toastSeq
			return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return clearToast(seq) })
		}
		return m, nil
	}
	if m.logJob >= 0 {
		switch k {
		case "j", "down":
			m.logScroll++
		case "k", "up":
			m.logScroll = max(0, m.logScroll-1)
		case "pgdown", "space":
			m.logScroll += 10
		case "pgup", "b":
			m.logScroll = max(0, m.logScroll-10)
		case "g", "home":
			m.logScroll = 0
		case "G", "end":
			m.logScroll = 1 << 30
		case "backspace", "esc", "h", "left":
			m.logJob = -1
		}
		return m, nil
	}
	switch m.variant {
	case 1:
		m.v1key(k)
	case 2:
		m.v2key(k)
	case 3:
		m.v3key(k)
	}
	return m, nil
}

func (m *model) logView(r run, j job, w, h int) string {
	lines := logFor(r, j)
	body := h - 1
	m.logScroll = max(0, min(m.logScroll, len(lines)-body))
	out := []string{lipgloss.NewStyle().Bold(true).Foreground(m.th.accent).Render("◂ "+r.title+" › "+j.name) +
		m.dim(fmt.Sprintf("  %d-%d/%d  j/k scroll, g/G ends, backspace back", m.logScroll+1, min(len(lines), m.logScroll+body), len(lines)))}
	for i := m.logScroll; i < len(lines) && i < m.logScroll+body; i++ {
		l := lines[i]
		ts, rest, _ := strings.Cut(l, " ")
		n := m.dim(fmt.Sprintf("%2d %s ", i+1, ts))
		switch {
		case strings.Contains(rest, "##[error]"):
			rest = lipgloss.NewStyle().Bold(true).Foreground(m.th.red).Render(rest)
		case strings.Contains(rest, "##[warning]"):
			rest = m.th.col(m.th.yellow, rest)
		case strings.Contains(rest, "##[group]"):
			rest = m.th.col(m.th.accent, rest)
		}
		out = append(out, n+rest)
	}
	return block(out, w, h)
}

func (m *model) View() tea.View {
	if m.w == 0 {
		return tea.NewView("loading")
	}
	h := m.h - 2
	var body string
	switch m.variant {
	case 1:
		body = m.v1View(m.w, h)
	case 2:
		body = m.v2View(m.w, h)
	default:
		body = m.v3View(m.w, h)
	}
	hint := ""
	switch {
	case m.filtering:
		hint = "/" + m.filter + "▏"
	case m.toast != "":
		hint = m.th.col(m.th.green, m.toast)
	case m.help:
		hint = m.dim([]string{"", "j/k move  tab/enter into jobs  enter on job: log  backspace back",
			"j/k move  enter: log of failing job  g/G first/last", "j/k move  space expand  tab status chip  l log of failing job"}[m.variant])
	case m.filter != "":
		hint = m.dim("filter: " + m.filter + "  (/ edit, esc in filter clears)")
	}
	labels := []string{"1 master-detail", "2 grouped", "3 triage"}
	var f []string
	for i, l := range labels {
		if i+1 == m.variant {
			l = lipgloss.NewStyle().Bold(true).Reverse(true).Render(" " + l + " ")
		} else {
			l = " " + l + " "
		}
		f = append(f, l)
	}
	foot := strings.Join(f, "") + m.dim(" │ / filter  r rerun  ? help  q quit")
	v := tea.NewView(body + "\n" + fit(hint, m.w) + "\n" + fit(foot, m.w))
	v.AltScreen = true
	return v
}

func main() {
	m := &model{dark: true, variant: 1, logJob: -1, exp: map[int]bool{}, th: newTheme(true)}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
