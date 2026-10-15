// Command prproto is a throwaway prototype of a PR modal made of panes.
// Everything is fake; nothing talks to GitHub.
package main

import (
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func sty(c string) func(string) string {
	s := lipgloss.NewStyle().Foreground(lipgloss.Color(c))
	return func(t string) string { return s.Render(t) }
}

var (
	green  = sty("2")
	red    = sty("1")
	yellow = sty("3")
	cyan   = sty("6")
	dim    = sty("8")
	accent = sty("12")
	bold   = lipgloss.NewStyle().Bold(true).Render
	rev    = lipgloss.NewStyle().Reverse(true).Render
)

const rightW = 36

const (
	pMain = iota
	pMerge
	pReviewers
	pDev
	pFiles
	pMeta
)

var paneNames = []string{"Conversation", "Merge", "Reviewers", "Development", "Files", "Meta"}

type file struct {
	dir, name string
	add, del  int
	thread    bool
	diff      string
}

func (f file) path() string { return f.dir + f.name }

var files = []file{
	{"pkg/calc/", "calc.go", 40, 2, true, `@@ -1,8 +1,46 @@
 package calc

-func Add(a, b int) int { return a + b }
+// Add returns a+b.
+func Add(a, b int) int { return a + b }
+
+// Div returns a/b and panics when b is zero.
+func Div(a, b int) int {
+	return a / b
+}
+
+// Mul returns a*b.
+func Mul(a, b int) int { return a * b }
@@ -38,3 +40,6 @@
 func helper() {}
+// Sub returns a-b.
+func Sub(a, b int) int { return a - b }
+
 // end`},
	{"pkg/calc/", "calc_test.go", 52, 0, false, `@@ -0,0 +1,52 @@
+package calc
+
+import "testing"
+
+func TestAdd(t *testing.T) {
+	if Add(1, 2) != 3 {
+		t.Fatal("bad add")
+	}
+}
+
+func TestMul(t *testing.T) {
+	if Mul(2, 3) != 6 {
+		t.Fatal("bad mul")
+	}
+}`},
	{"pkg/calc/", "doc.go", 8, 0, false, `@@ -0,0 +1,8 @@
+// Package calc has small integer helpers.
+//
+// It exists to share arithmetic between the commands
+// and keep their tests in one place.
+package calc`},
	{"cmd/app/", "main.go", 10, 4, false, `@@ -3,12 +3,18 @@
 import (
 	"fmt"
-	"os"
+	"os"
+
+	"example.com/app/pkg/calc"
 )

 func main() {
-	fmt.Println(1 + 2)
-	os.Exit(0)
+	fmt.Println(calc.Add(1, 2))
+	fmt.Println(calc.Mul(2, 3))
+	os.Exit(0)
-	// TODO
 }
@@ -28,4 +34,6 @@
 func run() error {
-	return nil
+	if dryRun {
+		return nil
+	}
+	return nil
 }`},
	{"cmd/app/", "flags.go", 6, 2, false, `@@ -1,6 +1,10 @@
 package main

-var verbose = false
-var quiet = false
+var (
+	verbose = false
+	quiet   = false
+	dryRun  = false
+)`},
	{"", "README.md", 4, 0, false, `@@ -10,3 +10,7 @@
 ## Usage

+Run the app with the calc helpers:
+
+    app --dry-run`},
}

func (f file) total() int {
	return map[string]int{"calc.go": 60, "calc_test.go": 52, "doc.go": 8, "main.go": 45, "flags.go": 40, "README.md": 30}[f.name]
}

type hunk struct {
	hdr   string
	start int
	lines []string
}

func parseHunks(d string) []hunk {
	var hs []hunk
	for _, l := range strings.Split(d, "\n") {
		if strings.HasPrefix(l, "@@") {
			var a, b, c, e int
			fmt.Sscanf(l, "@@ -%d,%d +%d,%d @@", &a, &b, &c, &e)
			hs = append(hs, hunk{hdr: l, start: max(c, 1)})
			continue
		}
		if len(hs) > 0 {
			hs[len(hs)-1].lines = append(hs[len(hs)-1].lines, l)
		}
	}
	return hs
}

func fillerLine(n int) string {
	switch n % 4 {
	case 0:
		return fmt.Sprintf("func helper%d(x int) int {", n)
	case 1:
		return fmt.Sprintf("\treturn x + %d", n)
	case 2:
		return "}"
	}
	return ""
}

var ctxNames = []string{"hunks only", "±10 lines", "full file"}

type fl struct {
	text string
	kind byte
}

// fileView fakes the three context modes: the hunks as given, ten lines
// around each change, and the whole file with the changes marked inline.
func fileView(f file, mode int) []string {
	neu := strings.Contains(f.diff, "@@ -0,0")
	filler := func(n int) fl {
		if neu {
			return fl{"+" + fillerLine(n), '+'}
		}
		return fl{" " + fillerLine(n), 'f'}
	}
	var out []fl
	n := 1
	for _, h := range parseHunks(f.diff) {
		for ; n < h.start; n++ {
			out = append(out, filler(n))
		}
		out = append(out, fl{h.hdr, '@'})
		for _, l := range h.lines {
			k := byte('h')
			switch {
			case strings.HasPrefix(l, "+"):
				k = '+'
				n++
			case strings.HasPrefix(l, "-"):
				k = '-'
			default:
				n++
			}
			out = append(out, fl{l, k})
		}
	}
	for ; n <= f.total(); n++ {
		out = append(out, filler(n))
	}
	keep := make([]bool, len(out))
	for i, x := range out {
		if x.kind == '+' || x.kind == '-' {
			for j := max(0, i-10); j <= min(len(out)-1, i+10); j++ {
				keep[j] = true
			}
		}
	}
	var res []string
	gap := false
	for i, x := range out {
		show := false
		switch mode {
		case 0:
			show = x.kind != 'f'
		case 1:
			show = keep[i] && x.kind != '@'
		default:
			show = x.kind != '@'
		}
		if !show {
			gap = gap || (mode == 1 && len(res) > 0)
			continue
		}
		if gap {
			res = append(res, dim("   ⋯"))
			gap = false
		}
		switch x.kind {
		case '@':
			res = append(res, cyan(x.text))
		case '+':
			res = append(res, green(x.text))
		case '-':
			res = append(res, red(x.text))
		case 'f':
			res = append(res, dim(x.text))
		default:
			res = append(res, x.text)
		}
	}
	return res
}

type row struct {
	dir   bool
	fi    int
	label string
}

type entry struct{ who, kind, when, body string }

var timeline = []entry{
	{"bob", "comment", "3d ago", "Nice start. Can we cover division by zero? I did not see a test for it and the helper looks like it would panic."},
	{"alice", "commit", "3d ago", "Add calc helper (a1b2c3d)"},
	{"alice", "comment", "2d ago", "Added a test for it, plus a package doc in doc.go."},
	{"bob", "approve", "2d ago", "LGTM once CI is green."},
	{"carol", "changes", "1d ago", "The Div helper panics on zero. Please return an error instead; see the thread."},
	{"carol", "thread", "1d ago", "calc.go:12\n+	return a / b\ncarol: Return an error instead of panicking?\nalice: Planning to, wanted to confirm the API first."},
	{"ci", "event", "1d ago", "e2e failed on a1b2c3d"},
	{"alice", "comment", "5h ago", "Pushed a fix for the panic, waiting on carol."},
	{"coverage-bot", "comment", "1h ago", "Coverage is 91% (+2%) with this change."},
}

const description = "Adds a small calc package with Add, Div and Mul helpers, and uses it from the app command. Division by zero still panics for now; the review thread covers what to do about it."

type check struct{ name, state, dur string }

var checks = []check{
	{"build (linux)", "ok", "1m12s"}, {"build (macos)", "ok", "2m01s"}, {"build (windows)", "ok", "2m40s"},
	{"test", "ok", "3m22s"}, {"lint", "ok", "0m48s"}, {"vet", "ok", "0m20s"},
	{"bench", "ok", "4m10s"}, {"schema", "ok", "0m09s"}, {"cross-compile", "ok", "1m55s"},
	{"codeql", "ok", "5m03s"}, {"docs", "ok", "0m31s"}, {"e2e", "fail", "3m02s"},
}

type reviewer struct{ name, state string }

type devItem struct {
	kind         string // closes or pr
	num          int
	title, state string
	blocked      string
}

type metaRow struct {
	label string
	vals  []string
	opts  []string
}

type overlay struct {
	kind   string // file, combined, checks, issue, help, picker, confirm
	arg    int
	scroll int

	// the diff overlay: pane focus (0 files, 1 diff), cursor in the files
	// list (an index into model.rows), context mode and the `:` line
	focus int
	cur   int
	ctx   int
	cmdOn bool
	cmd   string

	title  string
	items  []string
	sel    map[string]bool
	multi  bool
	filter string
	pcur   int
	done   func(*model, []string) tea.Cmd

	text  string
	toast string
}

type model struct {
	w, h      int
	focus     int
	zoom      bool
	scroll    int
	cur       [6]int
	stack     []overlay
	toast     string
	toastID   int
	reviewers []reviewer
	dev       []devItem
	meta      [4]metaRow
	rows      []row
}

type clearToastMsg struct{ id int }

func newModel() *model {
	m := &model{
		reviewers: []reviewer{{"bob", "approved"}, {"carol", "changes"}, {"@org/core", "requested"}},
		dev: []devItem{
			{"closes", 12, "Calc is slow", "open", "#9"},
			{"closes", 15, "Typo in README", "closed", ""},
			{"pr", 40, "Refactor arithmetic", "open", ""},
		},
		meta: [4]metaRow{
			{"Assignees", []string{"alice"}, []string{"alice", "bob", "carol", "dave"}},
			{"Labels", []string{"bug", "api"}, []string{"bug", "api", "docs", "enhancement", "good first issue"}},
			{"Milestone", []string{"v1.2"}, []string{"v1.1", "v1.2", "v2.0"}},
			{"Project", []string{"Roadmap · Todo"}, []string{"Roadmap · Todo", "Roadmap · Doing", "Roadmap · Done", "Sprint 7 · Backlog"}},
		},
	}
	last := "?"
	for i, f := range files {
		if f.dir != last {
			l := f.dir
			if l == "" {
				l = "./"
			}
			m.rows = append(m.rows, row{dir: true, label: l})
			last = f.dir
		}
		m.rows = append(m.rows, row{fi: i})
	}
	m.cur[pFiles] = 1
	return m
}

func (m *model) Init() tea.Cmd { return nil }

// ---------- geometry ----------

func (m *model) narrow() bool { return m.w < 120 }
func (m *model) zoomed() bool { return m.zoom || m.narrow() }
func (m *model) bodyTop() int {
	if m.narrow() {
		return 2
	}
	return 1
}
func (m *model) bodyH() int { return m.h - m.bodyTop() - 1 }

func (m *model) panelHeights() [6]int {
	H := m.bodyH()
	nat := [6]int{0, 7, max(3, len(m.reviewers)+2), max(3, len(m.dev)+2), 0, 6}
	sum := func() int {
		s := 0
		for _, v := range nat {
			s += v
		}
		return s
	}
	for sum() > H-5 {
		best := -1
		for _, i := range []int{pMerge, pReviewers, pDev, pMeta} {
			if nat[i] > 3 && (best < 0 || nat[i] > nat[best]) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		nat[best]--
	}
	nat[pFiles] = max(3, H-sum())
	nat[pMain] = H
	return nat
}

func (m *model) paneDims(i int) (int, int) {
	if m.zoomed() {
		return m.w, m.bodyH()
	}
	if i == pMain {
		return m.w - rightW, m.bodyH()
	}
	return rightW, m.panelHeights()[i]
}

// ---------- content ----------

func wrap(s string, w int) []string {
	return strings.Split(ansi.Wrap(s, max(w, 10), ""), "\n")
}

func (m *model) convLines(cw int) []string {
	var out []string
	out = append(out, bold("#42 Add calc helper"), dim("alice opened this 3 days ago · feat/calc → main"), "")
	for _, l := range wrap(description, cw) {
		out = append(out, l)
	}
	out = append(out, "", dim(strings.Repeat("─", cw)), "")
	for _, e := range timeline {
		var head string
		switch e.kind {
		case "comment":
			head = bold(e.who) + dim(" commented · "+e.when)
		case "commit":
			head = dim("◆ ") + bold(e.who) + dim(" pushed a commit · "+e.when)
		case "approve":
			head = green("✓ ") + bold(e.who) + green(" approved") + dim(" · "+e.when)
		case "changes":
			head = red("✗ ") + bold(e.who) + red(" requested changes") + dim(" · "+e.when)
		case "event":
			head = yellow("● ") + bold(e.who) + dim(" · "+e.when)
		case "thread":
			parts := strings.Split(e.body, "\n")
			out = append(out, yellow("┌ ")+bold(parts[0])+yellow(" · unresolved")+dim(" · "+e.when))
			out = append(out, dim("│ ")+green(parts[1]))
			for _, p := range parts[2:] {
				for _, l := range wrap(p, cw-2) {
					out = append(out, dim("│ ")+l)
				}
			}
			out = append(out, yellow("└"), "")
			continue
		}
		out = append(out, head)
		for _, l := range wrap(e.body, cw-2) {
			if e.kind == "commit" {
				l = cyan(l)
			}
			out = append(out, "  "+l)
		}
		out = append(out, "")
	}
	return out
}

func diffLines(d string) []string {
	var out []string
	for _, l := range strings.Split(d, "\n") {
		switch {
		case strings.HasPrefix(l, "@@"):
			out = append(out, cyan(l))
		case strings.HasPrefix(l, "+"):
			out = append(out, green(l))
		case strings.HasPrefix(l, "-"):
			out = append(out, red(l))
		default:
			out = append(out, l)
		}
	}
	return out
}

func (m *model) mrow(pane, i int, text string) string {
	if m.focus == pane && m.cur[pane] == i {
		return accent("› ") + text
	}
	return "  " + text
}

func (m *model) paneLines(i, cw int) ([]string, int) {
	switch i {
	case pMain:
		return m.convLines(cw), 0
	case pMerge:
		return []string{
			yellow("Blocked: changes requested"),
			m.mrow(i, 0, red("✗")+" 1 of 12 checks failing"+dim("  C")),
			m.mrow(i, 1, "Merge (squash)"+dim("  M")),
			m.mrow(i, 2, "Auto-merge"+dim("  A")),
			m.mrow(i, 3, "Update branch"+dim("  U")),
		}, m.cur[i] + 1
	case pReviewers:
		var out []string
		for k, r := range m.reviewers {
			icon := yellow("○")
			switch r.state {
			case "approved":
				icon = green("✓")
			case "changes":
				icon = red("✗")
			}
			out = append(out, m.mrow(i, k, icon+" "+r.name+dim(" "+r.state)))
		}
		if len(out) == 0 {
			out = append(out, dim("  no reviewers (a to request)"))
		}
		return out, m.cur[i]
	case pDev:
		var out []string
		for k, d := range m.dev {
			st := green("open")
			if d.state == "closed" {
				st = red("closed")
			}
			t := fmt.Sprintf("closes #%d ", d.num)
			if d.kind == "pr" {
				t = fmt.Sprintf("PR #%d ", d.num)
			}
			t += st
			if d.blocked != "" {
				t += yellow(" ⛔ " + d.blocked)
			}
			out = append(out, m.mrow(i, k, t))
		}
		if len(out) == 0 {
			out = append(out, dim("  nothing linked (a to link)"))
		}
		return out, m.cur[i]
	case pFiles:
		var out []string
		curLine := 0
		for k, r := range m.rows {
			if r.dir {
				out = append(out, dim("▾ "+r.label))
				continue
			}
			f := files[r.fi]
			t := f.name + " " + green(fmt.Sprintf("+%d", f.add)) + " " + red(fmt.Sprintf("−%d", f.del))
			if f.thread {
				t += yellow(" ●")
			}
			if m.focus == pFiles && m.cur[pFiles] == k {
				out = append(out, accent("› ")+t)
				curLine = k
			} else {
				out = append(out, "  "+t)
			}
		}
		if m.cur[pFiles] > curLine {
			curLine = m.cur[pFiles]
		}
		return out, m.cur[pFiles]
	default:
		var out []string
		for k, r := range m.meta {
			v := strings.Join(r.vals, ", ")
			if v == "" {
				v = dim("none")
			}
			out = append(out, m.mrow(i, k, fmt.Sprintf("%-10s %s", r.label, v)))
		}
		return out, m.cur[i]
	}
}

func (m *model) paneTitle(i int) string {
	t := fmt.Sprintf("%d %s", i+1, paneNames[i])
	if i == pFiles {
		t += fmt.Sprintf(" (%d)", len(files))
	}
	return t
}

func box(title string, lines []string, w, h int, focus bool) string {
	col := dim
	if focus {
		col = accent
	}
	inner := w - 2
	t := "─ " + title + " "
	if lipgloss.Width(t) > inner {
		t = ansi.Truncate(t, inner, "")
	}
	b := []string{col("╭" + t + strings.Repeat("─", max(0, inner-lipgloss.Width(t))) + "╮")}
	for i := 0; i < h-2; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		l = ansi.Truncate(l, w-4, "…")
		pad := max(0, w-4-lipgloss.Width(l))
		b = append(b, col("│")+" "+l+strings.Repeat(" ", pad)+" "+col("│"))
	}
	b = append(b, col("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(b, "\n")
}

func (m *model) maxScroll() int {
	w, h := m.paneDims(pMain)
	return max(0, len(m.convLines(w-4))-(h-2))
}

func (m *model) pane(i int) string {
	w, h := m.paneDims(i)
	lines, cl := m.paneLines(i, w-4)
	ih := max(1, h-2)
	off := 0
	title := m.paneTitle(i)
	if i == pMain {
		off = min(m.scroll, max(0, len(lines)-ih))
		if len(lines) > ih {
			title += fmt.Sprintf(" · %d%%", (off+ih)*100/len(lines))
		}
	} else {
		off = max(0, cl-ih+1)
	}
	end := min(len(lines), off+ih)
	return box(title, lines[off:end], w, h, m.focus == i)
}

// ---------- overlays ----------

func (m *model) ovRect(o *overlay) (int, int, int, int) {
	W, H := m.w, m.h-1
	switch o.kind {
	case "confirm":
		w := min(W-2, max(40, lipgloss.Width(o.text)+8))
		return (W - w) / 2, max(1, (H-3)/2), w, 3
	case "picker":
		if m.narrow() {
			return 0, 1, W, H - 1
		}
		w := min(46, W-4)
		h := min(len(o.items)+4, 14)
		return (W - w) / 2, max(1, (H-h)/2), w, h
	}
	// The header line stays visible above every overlay.
	if m.narrow() {
		return 0, 1, W, H - 1
	}
	return 4, 2, W - 8, H - 4
}

func (o *overlay) filtered() []string {
	var out []string
	for _, it := range o.items {
		if strings.Contains(strings.ToLower(it), strings.ToLower(o.filter)) {
			out = append(out, it)
		}
	}
	return out
}

func (m *model) ovContent(o *overlay) (string, []string) {
	switch o.kind {
	case "file":
		f := files[o.arg]
		return fmt.Sprintf("2 Diff · %s (%d/%d) · %s [x]", f.path(), o.arg+1, len(files), ctxNames[o.ctx]), fileView(f, o.ctx)
	case "combined":
		var out []string
		for _, f := range files {
			out = append(out, bold(accent(fmt.Sprintf("━━ %s  +%d −%d", f.path(), f.add, f.del))))
			out = append(out, diffLines(f.diff)...)
			out = append(out, "")
		}
		return "Combined diff · 6 files · +120 −8", out
	case "checks":
		out := []string{red("1 failing"), green("11 passing"), ""}
		for _, c := range checks {
			ic := green("✓")
			if c.state == "fail" {
				ic = red("✗")
			}
			out = append(out, fmt.Sprintf("%s %-18s %s", ic, c.name, dim(c.dur)))
		}
		return "Checks · 12 runs", out
	case "issue":
		d := m.dev[o.arg]
		kind := "Issue"
		if d.kind == "pr" {
			kind = "Pull request"
		}
		st := green("● open")
		if d.state == "closed" {
			st = red("● closed")
		}
		out := []string{bold(d.title), st}
		if d.blocked != "" {
			out = append(out, yellow("⛔ blocked by "+d.blocked+" (open)"))
		}
		out = append(out, "", "Opened by dave 2 weeks ago. Labels: bug.", "",
			"Fake summary of the issue body. The real one would be the markdown of the description, with the latest comments below it.")
		return fmt.Sprintf("%s #%d", kind, d.num), out
	case "help":
		return "Keys", []string{
			"tab / shift+tab   next / previous pane      1-6  jump to a pane",
			"j k ↑ ↓ pgup pgdn scroll or move            z    zoom the focused pane",
			"esc / backspace   close the top overlay     q    quit",
			"",
			"Merge       enter/C checks · M merge · A auto-merge · U update branch",
			"Reviewers   a request a reviewer · x remove",
			"Development enter issue summary · a link issue · x unlink",
			"Files       enter file diff · D combined diff · l focus conversation",
			"Meta        enter edit the field",
			"",
			"In a diff   tab/1/2 panes (files, diff) · enter in files shows that file",
			"            x cycles context: hunks only / ±10 lines / full file",
			"            :n / :p (typed like in less) or J / K: next / previous file",
			"In a picker type to filter · ↑/↓ · tab toggles (multi) · enter picks",
		}
	}
	return "", nil
}

func (m *model) drawOverlay(o *overlay) (string, int, int, int) {
	x, y, w, h := m.ovRect(o)
	var s string
	switch o.kind {
	case "confirm":
		s = box("Confirm", []string{o.text + "  " + dim("y/n")}, w, h, true)
	case "picker":
		lines := []string{accent("› ") + o.filter + "▏", dim(strings.Repeat("─", w-4))}
		fl := o.filtered()
		slots := max(1, h-4)
		off := max(0, o.pcur-slots+1)
		for i := off; i < min(len(fl), off+slots); i++ {
			p := "  "
			if i == o.pcur {
				p = accent("› ")
			}
			mk := ""
			if o.multi {
				mk = "[ ] "
				if o.sel[fl[i]] {
					mk = "[x] "
				}
			}
			lines = append(lines, p+mk+fl[i])
		}
		s = box(o.title, lines, w, h, true)
	case "file":
		w1 := min(30, w/3)
		if m.narrow() {
			w1 = w
		}
		var parts []string
		if o.focus == 0 || !m.narrow() {
			var lines []string
			for k, r := range m.rows {
				if r.dir {
					lines = append(lines, dim("▾ "+r.label))
					continue
				}
				name := files[r.fi].name
				if r.fi == o.arg {
					name = bold(name) + accent(" ◂")
				}
				p := "  "
				if o.focus == 0 && o.cur == k {
					p = accent("› ")
				}
				lines = append(lines, p+name)
			}
			off := max(0, o.cur-(h-2)+1)
			parts = append(parts, box("1 Files", lines[min(off, len(lines)):], w1, h, o.focus == 0))
		}
		if o.focus == 1 || !m.narrow() {
			w2 := w - w1
			if m.narrow() {
				w2 = w
			}
			parts = append(parts, m.scrollBox(o, w2, h, o.focus == 1))
		}
		s = lipgloss.JoinHorizontal(lipgloss.Top, parts...)
	default:
		s = m.scrollBox(o, w, h, true)
	}
	return s, x, y, w
}

func (m *model) scrollBox(o *overlay, w, h int, focus bool) string {
	title, lines := m.ovContent(o)
	ih := h - 2
	off := min(o.scroll, max(0, len(lines)-ih))
	if len(lines) > ih {
		title += fmt.Sprintf(" · %d%%", (off+ih)*100/len(lines))
	}
	return box(title, lines[off:min(len(lines), off+ih)], w, h, focus)
}

func (m *model) rowOf(fi int) int {
	for k, r := range m.rows {
		if !r.dir && r.fi == fi {
			return k
		}
	}
	return 0
}

func (m *model) showFile(o *overlay, fi int) {
	o.arg = max(0, min(fi, len(files)-1))
	o.cur = m.rowOf(o.arg)
	o.scroll = 0
}

func place(bg []string, ov string, x, y, w int) {
	for i, l := range strings.Split(ov, "\n") {
		r := y + i
		if r < 0 || r >= len(bg) {
			break
		}
		left := ansi.Truncate(bg[r], x, "")
		left += strings.Repeat(" ", max(0, x-lipgloss.Width(left)))
		bg[r] = left + l + ansi.TruncateLeft(bg[r], x+w, "")
	}
}

// ---------- view ----------

func (m *model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m *model) render() string {
	if m.w == 0 {
		return ""
	}
	hdr := bold("#42 Add calc helper") + dim(" · ") + green("open") + " · alice " + dim("feat/calc → main") +
		" · " + green("+120") + " " + red("−8") + " · 6 files"
	parts := []string{hdr}
	if m.narrow() {
		var names []string
		for i := range paneNames {
			n := fmt.Sprintf("%d %s", i+1, paneNames[i])
			if i == m.focus {
				names = append(names, rev(" "+n+" "))
			} else {
				names = append(names, dim(n))
			}
		}
		parts = append(parts, strings.Join(names, " "))
	}
	if m.zoomed() {
		parts = append(parts, m.pane(m.focus))
	} else {
		var col []string
		for i := pMerge; i <= pMeta; i++ {
			col = append(col, m.pane(i))
		}
		parts = append(parts, lipgloss.JoinHorizontal(lipgloss.Top, m.pane(pMain), lipgloss.JoinVertical(lipgloss.Left, col...)))
	}
	lines := strings.Split(strings.Join(parts, "\n"), "\n")
	for len(lines) < m.h-1 {
		lines = append(lines, "")
	}
	lines = lines[:m.h-1]
	for i := range m.stack {
		for j := 1; j < len(lines); j++ {
			l := lines[j]
			lines[j] = dim(ansi.Strip(l))
		}
		s, x, y, w := m.drawOverlay(&m.stack[i])
		place(lines, s, x, y, w)
	}
	lines = append(lines, ansi.Truncate(m.footer(), m.w, "…"))
	return strings.Join(lines, "\n")
}

func (m *model) footer() string {
	if m.toast != "" {
		return green(m.toast)
	}
	if n := len(m.stack); n > 0 {
		switch o := m.stack[n-1]; o.kind {
		case "confirm":
			return dim("y yes · n/esc no")
		case "picker":
			if o.multi {
				return dim("type to filter · ↑/↓ move · tab toggle · enter apply · esc cancel")
			}
			return dim("type to filter · ↑/↓ move · enter pick · esc cancel")
		case "file":
			if o.cmdOn {
				return ":" + o.cmd + "▏" + dim("   n next file · p previous file")
			}
			return dim("tab/1/2 panes · j/k · enter open file · x context · :n :p · J/K files · esc close")
		default:
			return dim("j/k scroll · esc/backspace close")
		}
	}
	h := map[int]string{
		pMain:      "j/k scroll",
		pMerge:     "enter/C checks · M merge · A auto-merge · U update",
		pReviewers: "a request · x remove",
		pDev:       "enter open · a link · x unlink",
		pFiles:     "enter diff · D combined · l conversation",
		pMeta:      "enter edit",
	}[m.focus]
	return dim(h + " · tab/1-6 focus · z zoom · ? help · q quit")
}

// ---------- update ----------

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case clearToastMsg:
		if msg.id == m.toastID {
			m.toast = ""
		}
	case tea.KeyPressMsg:
		return m, m.key(msg)
	}
	return m, nil
}

func (m *model) say(s string) tea.Cmd {
	m.toastID++
	m.toast = s
	id := m.toastID
	return tea.Tick(2500*time.Millisecond, func(time.Time) tea.Msg { return clearToastMsg{id} })
}

func (m *model) push(o overlay) { m.stack = append(m.stack, o) }
func (m *model) pop()           { m.stack = m.stack[:len(m.stack)-1] }

func moveDelta(k string) int {
	switch k {
	case "j", "down":
		return 1
	case "k", "up":
		return -1
	case "pgdown", "space":
		return 10
	case "pgup":
		return -10
	case "g", "home":
		return -10000
	case "G", "end":
		return 10000
	}
	return 0
}

func (m *model) key(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if k == "ctrl+c" {
		return tea.Quit
	}
	if len(m.stack) > 0 {
		return m.overlayKey(msg)
	}
	switch k {
	case "q":
		return tea.Quit
	case "tab":
		m.focus = (m.focus + 1) % 6
	case "shift+tab":
		m.focus = (m.focus + 5) % 6
	case "1", "2", "3", "4", "5", "6":
		m.focus = int(k[0] - '1')
	case "z":
		m.zoom = !m.zoom
	case "?":
		m.push(overlay{kind: "help"})
	default:
		return m.paneKey(k)
	}
	return nil
}

func (m *model) count(p int) int {
	switch p {
	case pMerge:
		return 4
	case pReviewers:
		return len(m.reviewers)
	case pDev:
		return len(m.dev)
	case pFiles:
		return len(m.rows)
	case pMeta:
		return 4
	}
	return 0
}

func (m *model) move(d int) {
	if m.focus == pMain {
		m.scroll = max(0, min(m.scroll+d, m.maxScroll()))
		return
	}
	if m.focus == pFiles {
		m.cur[pFiles] = m.snap(m.cur[pFiles], d)
		return
	}
	m.cur[m.focus] = max(0, min(m.cur[m.focus]+d, m.count(m.focus)-1))
}

// snap moves a cursor in the files rows by d, skipping folder rows.
func (m *model) snap(cur, d int) int {
	n := len(m.rows)
	t := max(0, min(cur+d, n-1))
	if m.rows[t].dir {
		if d >= 0 {
			t++
		} else {
			t--
		}
		if t < 0 || t >= n || m.rows[t].dir {
			t = cur
		}
	}
	return t
}

func (m *model) confirm(text, toast string) {
	m.push(overlay{kind: "confirm", text: text, toast: toast})
}

func (m *model) paneKey(k string) tea.Cmd {
	if d := moveDelta(k); d != 0 {
		m.move(d)
		return nil
	}
	switch m.focus {
	case pMerge:
		i := -1
		switch k {
		case "C":
			i = 0
		case "M":
			i = 1
		case "A":
			i = 2
		case "U":
			i = 3
		case "enter":
			i = m.cur[pMerge]
		}
		switch i {
		case 0:
			m.push(overlay{kind: "checks"})
		case 1:
			m.confirm("Merge #42 with squash?", "(prototype) would merge")
		case 2:
			m.confirm("Enable auto-merge on #42?", "(prototype) would enable auto-merge")
		case 3:
			m.confirm("Update branch with main?", "(prototype) would update the branch")
		}
	case pReviewers:
		switch k {
		case "a":
			var items []string
			for _, n := range []string{"bob", "carol", "dave", "erin", "Copilot"} {
				if !slices.ContainsFunc(m.reviewers, func(r reviewer) bool { return r.name == n }) {
					items = append(items, n)
				}
			}
			m.push(overlay{kind: "picker", title: "Request a reviewer", items: items, done: func(m *model, p []string) tea.Cmd {
				m.reviewers = append(m.reviewers, reviewer{p[0], "requested"})
				return m.say("(prototype) requested " + p[0])
			}})
		case "x":
			if c := m.cur[pReviewers]; c < len(m.reviewers) {
				m.reviewers = slices.Delete(m.reviewers, c, c+1)
				m.cur[pReviewers] = max(0, min(c, len(m.reviewers)-1))
			}
		}
	case pDev:
		switch k {
		case "enter":
			if len(m.dev) > 0 {
				m.push(overlay{kind: "issue", arg: m.cur[pDev]})
			}
		case "a":
			m.push(overlay{kind: "picker", title: "Link an issue", items: []string{"#21 Docs for calc", "#22 Rounding bug", "#23 Add CLI flag"},
				done: func(m *model, p []string) tea.Cmd {
					var n int
					fmt.Sscanf(p[0], "#%d", &n)
					_, title, _ := strings.Cut(p[0], " ")
					m.dev = append(m.dev, devItem{"closes", n, title, "open", ""})
					return m.say("(prototype) linked " + p[0])
				}})
		case "x":
			if c := m.cur[pDev]; c < len(m.dev) {
				m.dev = slices.Delete(m.dev, c, c+1)
				m.cur[pDev] = max(0, min(c, len(m.dev)-1))
			}
		}
	case pFiles:
		switch k {
		case "enter":
			if r := m.rows[m.cur[pFiles]]; !r.dir {
				m.push(overlay{kind: "file", arg: r.fi, cur: m.cur[pFiles], focus: 1})
			}
		case "D":
			m.push(overlay{kind: "combined"})
		case "l", "right":
			m.focus = pMain
		}
	case pMeta:
		if k == "enter" {
			r := m.meta[m.cur[pMeta]]
			sel := map[string]bool{}
			for _, v := range r.vals {
				sel[v] = true
			}
			idx := m.cur[pMeta]
			m.push(overlay{kind: "picker", title: r.label, items: r.opts, sel: sel, multi: true, done: func(m *model, p []string) tea.Cmd {
				m.meta[idx].vals = p
				return m.say("(prototype) updated " + strings.ToLower(r.label))
			}})
		}
	}
	return nil
}

func (m *model) overlayKey(msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	top := &m.stack[len(m.stack)-1]
	switch top.kind {
	case "confirm":
		switch k {
		case "y":
			t := top.toast
			m.pop()
			return m.say(t)
		case "n", "esc", "backspace":
			m.pop()
		}
	case "picker":
		fl := top.filtered()
		switch k {
		case "esc":
			m.pop()
		case "enter":
			var picked []string
			if top.multi {
				for _, it := range top.items {
					if top.sel[it] {
						picked = append(picked, it)
					}
				}
			} else if top.pcur < len(fl) {
				picked = []string{fl[top.pcur]}
			} else {
				return nil
			}
			done := top.done
			m.pop()
			return done(m, picked)
		case "up", "ctrl+p":
			top.pcur = max(0, top.pcur-1)
		case "down", "ctrl+n":
			top.pcur = max(0, min(top.pcur+1, len(fl)-1))
		case "tab":
			if top.multi && top.pcur < len(fl) {
				top.sel[fl[top.pcur]] = !top.sel[fl[top.pcur]]
			}
		case "backspace":
			if top.filter == "" {
				m.pop()
			} else {
				r := []rune(top.filter)
				top.filter = string(r[:len(r)-1])
				top.pcur = 0
			}
		default:
			if msg.Text != "" {
				top.filter += msg.Text
				top.pcur = 0
			}
		}
	case "file":
		if top.cmdOn {
			switch k {
			case "esc":
				top.cmdOn = false
			case "enter":
				c := top.cmd
				top.cmdOn = false
				switch c {
				case "n", "next":
					m.showFile(top, top.arg+1)
				case "p", "prev", "N":
					m.showFile(top, top.arg-1)
				default:
					return m.say("(prototype) unknown command :" + c)
				}
			case "backspace":
				if top.cmd == "" {
					top.cmdOn = false
				} else {
					top.cmd = top.cmd[:len(top.cmd)-1]
				}
			default:
				top.cmd += msg.Text
			}
			return nil
		}
		switch k {
		case ":":
			top.cmdOn, top.cmd = true, ""
		case "esc", "backspace", "q":
			m.pop()
		case "tab", "shift+tab":
			top.focus = 1 - top.focus
		case "1", "h", "left":
			top.focus = 0
		case "2", "l", "right":
			top.focus = 1
		case "x":
			top.ctx = (top.ctx + 1) % len(ctxNames)
			top.scroll = 0
		case "]", "J":
			m.showFile(top, top.arg+1)
		case "[", "K":
			m.showFile(top, top.arg-1)
		case "enter":
			if top.focus == 0 && !m.rows[top.cur].dir {
				m.showFile(top, m.rows[top.cur].fi)
			}
		default:
			d := moveDelta(k)
			if d == 0 {
				return nil
			}
			if top.focus == 0 {
				top.cur = m.snap(top.cur, d)
				break
			}
			_, _, _, h := m.ovRect(top)
			_, lines := m.ovContent(top)
			top.scroll = max(0, min(top.scroll+d, len(lines)-(h-2)))
		}
	default:
		switch k {
		case "esc", "backspace", "q":
			m.pop()
			return nil
		case "?":
			if top.kind == "help" {
				m.pop()
			}
			return nil
		}
		if d := moveDelta(k); d != 0 {
			_, _, _, h := m.ovRect(top)
			_, lines := m.ovContent(top)
			top.scroll = max(0, min(top.scroll+d, len(lines)-(h-2)))
		}
	}
	return nil
}

func main() {
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
