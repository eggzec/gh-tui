// Command prproto is a throwaway prototype of a PR modal made of panes.
// Everything is fake; nothing talks to GitHub.
package main

import (
	"fmt"
	"os"
	"regexp"
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

// Selection colours, all in one place: truecolor with a 256 colour fallback.
var (
	selBG     = bgc(0x2a, 0x2f, 0x3a, 236) // the selected row of a focused pane
	selMark   = fgc(0x61, 0xaf, 0xef, 75)  // the ▌ marker of a focused pane
	selDim    = fgc(0x5c, 0x63, 0x70, 242) // the ▌ marker of an unfocused pane
	addCurBG  = bgc(0x0a, 0x3d, 0x0a, 28)  // the cursor on an added line
	delCurBG  = bgc(0x4f, 0x0a, 0x0a, 88)  // the cursor on a removed line
	ctxCurBG  = selBG                      // the cursor on a context line
	selMarker = "▌"
)

func fgc(r, g, b, c int) string {
	if truecolor {
		return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)
	}
	return fmt.Sprintf("\x1b[38;5;%dm", c)
}

// tint colours text with a truecolor foreground, 256 colour fallback.
func tint(r, g, b, c int) func(string) string {
	return func(s string) string { return fgc(r, g, b, c) + s + "\x1b[39m" }
}

// The status icons, in one table: GitHub's own Octicons from Nerd Fonts,
// coloured like GitHub. PRPROTO_ICONS=unicode keeps plain unicode glyphs.
var (
	plainIcons = os.Getenv("PRPROTO_ICONS") == "unicode"
	icGreen    = tint(0x3f, 0xb9, 0x50, 71)
	icRed      = tint(0xf8, 0x51, 0x49, 167)
	icAmber    = tint(0xd2, 0x99, 0x22, 178)
	icPurple   = tint(0xa3, 0x71, 0xf7, 141)
	resolvedC  = tint(0x5c, 0x7a, 0x5c, 65) // dim green
)

type glyph struct {
	nerd, uni string
	col       func(string) string
}

// String is the coloured glyph.
func (g glyph) String() string {
	if plainIcons {
		return g.col(g.uni)
	}
	return g.col(g.nerd)
}

var ic = struct {
	approved, changes, commented, requested, rerequested, dismissed glyph
	unresolved, resolved, outdated                                  glyph
	checkOK, checkFail, checkPending                                glyph
	merged, prOpen, prDraft, prClosed, issueOpen, issueClosed       glyph
	blocked                                                         glyph
}{
	approved:     glyph{"", "✓", icGreen},
	changes:      glyph{"", "✗", icRed},
	commented:    glyph{"", "○", dim},
	requested:    glyph{"", "○", icAmber},
	rerequested:  glyph{"", "↻", icAmber},
	dismissed:    glyph{"", "⊘", dim},
	unresolved:   glyph{"", "●", icAmber},
	resolved:     glyph{"", "✓", resolvedC},
	outdated:     glyph{"", "◌", dim},
	checkOK:      glyph{"", "✓", icGreen},
	checkFail:    glyph{"", "✗", icRed},
	checkPending: glyph{"", "●", icAmber},
	merged:       glyph{"", "◆", icPurple},
	prOpen:       glyph{"", "●", icGreen},
	prDraft:      glyph{"", "○", dim},
	prClosed:     glyph{"", "✗", icRed},
	issueOpen:    glyph{"", "●", icGreen},
	issueClosed:  glyph{"", "✓", icPurple},
	blocked:      glyph{"", "⛔", icRed},
}

// outdatedIcon is the plain outdated glyph; callers colour it dim.
var outdatedIcon = func() string {
	if plainIcons {
		return ic.outdated.uni
	}
	return ic.outdated.nerd
}()

// threadIcon is the state icon of a thread.
func threadIcon(resolved bool) string {
	if resolved {
		return ic.resolved.String()
	}
	return ic.unresolved.String()
}

// rightAlign puts right at the right edge of a w wide row. A long row loses
// the end of left first, then the right part.
func rightAlign(left, right string, w int) string {
	if right == "" {
		return left
	}
	rw := ansi.StringWidth(right)
	if ansi.StringWidth(left)+rw+1 > w {
		left = ansi.Truncate(left, max(1, w-rw-1), "…")
	}
	lw := ansi.StringWidth(left)
	if lw+rw+1 > w {
		return ansi.Truncate(left, w, "…")
	}
	return left + strings.Repeat(" ", w-lw-rw) + right
}

// selRow marks the selected row l, w columns wide: the first column becomes
// the marker, and a focused pane adds a dim background across the row. The
// row keeps its own foreground colours.
func selRow(l string, w int, focus bool) string {
	rest := ansi.TruncateLeft(l, 1, "")
	if !focus {
		return selDim + selMarker + "\x1b[39m" + rest
	}
	rest = strings.NewReplacer("\x1b[0m", "\x1b[0m"+selBG, "\x1b[m", "\x1b[m"+selBG).Replace(rest)
	pad := strings.Repeat(" ", max(0, w-ansi.StringWidth(l)))
	return selBG + "\x1b[1m" + selMark + selMarker + "\x1b[22;39m" + rest + pad + "\x1b[0m"
}

const rightW = 36

const (
	pDesc = iota
	pMain
	pMerge
	pReviewers
	pDev
	pFiles
	pMeta
	nPanes
)

var paneNames = []string{"Description", "Conversation", "Merge", "Reviewers", "Development", "Files", "Meta"}

// users maps a login to its display name; a login missing here has none and
// shows as itself.
var users = map[string]string{
	"alice":   "Alice Chen",
	"bob":     "Bob Okafor",
	"carol":   "Carol Diaz",
	"dave":    "Dave Kim",
	"erin":    "Erin Novak",
	"Copilot": "Copilot",

	"dependabot": "Dependabot",
}

// disp is how a person shows: the full name, else the login; a team
// (org/slug) shows as "slug team".
func disp(login string) string {
	if _, slug, ok := strings.Cut(login, "/"); ok {
		return slug + " team"
	}
	if n := users[login]; n != "" {
		return n
	}
	return login
}

// dispS is disp with the team tag dim.
func dispS(login string) string {
	if _, slug, ok := strings.Cut(login, "/"); ok {
		return slug + dim(" team")
	}
	return disp(login)
}

// personItem is a picker item for a person: the name and the dim-able login,
// so typing either matches.
func personItem(login string) string {
	if n := disp(login); n != login {
		return n + "  " + login
	}
	return login
}

// loginOf is the login of a picker item made by personItem.
func loginOf(item string) string {
	if _, l, ok := strings.Cut(item, "  "); ok {
		return l
	}
	return item
}

var inlineCode = regexp.MustCompile("`([^`]+)`")

// descOpen is the open details blocks of the description, by their path;
// descHdr the line of each block's summary in the last render.
var (
	descOpen = map[string]bool{}
	descHdr  []descHead
)

type descHead struct {
	key  string
	line int
}

// mdLines renders the description: light markdown with details blocks closed
// until enter opens them.
func mdLines(body string, w int) []string {
	descHdr = nil
	return mdSegs(parseBody(body), w, "d", 0)
}

func mdSegs(segs []seg, w int, key string, off int) []string {
	var out []string
	for i, s := range segs {
		if !s.det {
			out = append(out, mdPlain(s.text, w)...)
			continue
		}
		k := fmt.Sprintf("%s.%d", key, i)
		descHdr = append(descHdr, descHead{k, off + len(out)})
		if !descOpen[k] {
			out = append(out, s.summary+dim(" ⋯"))
			continue
		}
		out = append(out, s.summary)
		var in []string
		if s.text != "" {
			in = mdPlain(s.text, w-2)
		}
		in = append(in, mdSegs(s.rest, w-2, k, off+len(out)+len(in))...)
		for _, l := range in {
			out = append(out, dim("▏ ")+l)
		}
	}
	return out
}

// toggleDesc opens or closes the first details block at or below the top of
// the description pane, else the last one above it.
func (m *model) toggleDesc() {
	w, _ := m.paneDims(pDesc)
	mdLines(m.cv.items[0].body, w-4)
	for _, h := range descHdr {
		if h.line >= m.dscroll {
			descOpen[h.key] = !descOpen[h.key]
			return
		}
	}
	if n := len(descHdr); n > 0 {
		descOpen[descHdr[n-1].key] = !descOpen[descHdr[n-1].key]
	}
}

// mdPlain renders light markdown: bold headings, bullets, dim-background inline
// code and fenced blocks behind a dim left rule.
func mdPlain(body string, w int) []string {
	w = max(w, 12)
	codeBG := func(s string) string { return "\x1b[48;5;238m" + s + "\x1b[49m" }
	inline := func(s string) string {
		return inlineCode.ReplaceAllStringFunc(s, func(x string) string { return codeBG(strings.Trim(x, "`")) })
	}
	var out []string
	fence := false
	for _, l := range strings.Split(body, "\n") {
		t := strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(t, "```"):
			fence = !fence
		case fence:
			out = append(out, dim("▏ ")+ansi.Truncate(l, w-2, "…"))
		case strings.HasPrefix(t, "#"):
			out = append(out, bold(strings.TrimSpace(strings.TrimLeft(t, "#"))))
		case strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* "):
			for i, wl := range wrap(t[2:], w-2) {
				if i == 0 {
					out = append(out, dim("• ")+inline(wl))
				} else {
					out = append(out, "  "+inline(wl))
				}
			}
		default:
			for _, wl := range wrap(l, w) {
				out = append(out, inline(wl))
			}
		}
	}
	return out
}

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

var ctxNames = []string{"hunks only", "full file"}

var sel = lipgloss.NewStyle().Background(lipgloss.Color("238")).Render

// ---------- the pager-like diff modal ----------

// dl is one line of the full file with its changes inline; src is the line
// number in the new file (for a deleted line, the next line that follows).
type dl struct {
	text    string
	kind    byte // '@' hunk header, '+', '-', 'h' context, 'f' filler, 'H' file header
	src     int
	fi, loc int // file index and index within that file's lines (-1 for 'H')
	old, nw int // gutter line numbers, 0 for blank
	es, ee  int // rune range of the body to emphasise (the changed words)
}

type draft struct{ label, text string }

type vrow struct {
	text  string
	li    int
	draft bool
	dk    [3]int // for a draft row: file, end doc line, start doc line
}

func fileDoc(f file, fi int) []dl {
	isNew := strings.Contains(f.diff, "@@ -0,0")
	delta := 0 // added minus removed so far: old number = new number - delta
	filler := func(n int) dl {
		t := strings.ReplaceAll(fillerLine(n), "\t", "    ")
		if isNew {
			return dl{text: "+" + t, kind: '+', src: n, nw: n}
		}
		return dl{text: " " + t, kind: 'f', src: n, old: n - delta, nw: n}
	}
	var out []dl
	n := 1
	for _, h := range parseHunks(f.diff) {
		for ; n < h.start; n++ {
			out = append(out, filler(n))
		}
		out = append(out, dl{text: h.hdr, kind: '@', src: h.start})
		for _, l := range h.lines {
			l = strings.ReplaceAll(l, "\t", "    ")
			if l == "" {
				l = " "
			}
			x := dl{text: l, kind: 'h', src: n}
			switch {
			case strings.HasPrefix(l, "+"):
				x.kind, x.nw = '+', n
				n++
				delta++
			case strings.HasPrefix(l, "-"):
				x.kind, x.old = '-', n-delta
				delta--
			default:
				x.old, x.nw = n-delta, n
				n++
			}
			out = append(out, x)
		}
	}
	for ; n <= f.total(); n++ {
		out = append(out, filler(n))
	}
	for i := range out {
		out[i].fi, out[i].loc = fi, i
	}
	// emphasise the changed words of a removed line and the added line paired with it
	for i := 0; i < len(out); {
		if out[i].kind != '-' {
			i++
			continue
		}
		j := i
		for j < len(out) && out[j].kind == '-' {
			j++
		}
		k := j
		for k < len(out) && out[k].kind == '+' {
			k++
		}
		if j-i == k-j {
			for p := 0; p < j-i; p++ {
				emphasise(&out[i+p], &out[j+p])
			}
		}
		i = k
	}
	return out
}

// afterMark drops the leading +, - or space of a diff line (a blank line may
// have lost its space).
func afterMark(s string) string {
	if s == "" {
		return ""
	}
	return s[1:]
}

func emphasise(a, b *dl) {
	ra, rb := []rune(afterMark(a.text)), []rune(afterMark(b.text))
	pre := 0
	for pre < len(ra) && pre < len(rb) && ra[pre] == rb[pre] {
		pre++
	}
	suf := 0
	for suf < len(ra)-pre && suf < len(rb)-pre && ra[len(ra)-1-suf] == rb[len(rb)-1-suf] {
		suf++
	}
	if pre+suf == 0 || (pre == len(ra) && pre == len(rb)) {
		return
	}
	a.es, a.ee = pre, len(ra)-suf
	b.es, b.ee = pre, len(rb)-suf
}

// docFor is the lines of the pager: one file, or the whole patch with a git
// style header before each file.
func (m *model) docFor(o *overlay) []dl {
	if !o.all {
		return fileDoc(files[o.arg], o.arg)
	}
	var out []dl
	for i, f := range files {
		p := f.path()
		out = append(out, dl{text: "diff --git a/" + p + " b/" + p, kind: 'H', fi: i, loc: -1})
		out = append(out, fileDoc(f, i)...)
	}
	return out
}

// Delta style colours: a background per line kind, a stronger one on the
// changed words, truecolor with a 256 colour fallback.
var truecolor = strings.Contains(os.Getenv("COLORTERM"), "truecolor") || strings.Contains(os.Getenv("COLORTERM"), "24bit")

func bgc(r, g, b, c int) string {
	if truecolor {
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
	}
	return fmt.Sprintf("\x1b[48;5;%dm", c)
}

// lineBG: st 0 plain, 1 cursor, 2 highlighted (range or selected draft),
// 3 cursor of an unfocused pager (the marker only, no cursor background).
func lineBG(kind byte, st int) string {
	switch kind {
	case '+':
		return [4]string{bgc(0, 40, 0, 22), addCurBG, bgc(0, 56, 48, 29), bgc(0, 40, 0, 22)}[st]
	case '-':
		return [4]string{bgc(63, 0, 1, 52), delCurBG, bgc(80, 0, 48, 89), bgc(63, 0, 1, 52)}[st]
	}
	return [4]string{"", ctxCurBG, bgc(32, 32, 80, 17), ""}[st]
}

// curMark is the ▌ column of the pager's cursor row; st 3 is the dim one.
func curMark(st int) string {
	switch st {
	case 1:
		return "\x1b[1m" + selMark + selMarker + "\x1b[22m"
	case 3:
		return selDim + selMarker
	case 4: // a line a review thread comments on
		return selMark + selMarker
	}
	return ""
}

func renderDL(x dl, w, st int) string {
	const rst = "\x1b[0m"
	tw := max(0, w-4)
	switch x.kind {
	case 'H':
		switch st {
		case 1, 3:
			return selRow(bold(x.text), tw, st == 1)
		case 2:
			return lineBG(' ', st) + x.text + rst
		}
		return bold(x.text)
	case '@':
		fn := ""
		if _, after, ok := strings.Cut(strings.TrimPrefix(x.text, "@@"), "@@"); ok {
			fn = strings.TrimSpace(after)
		}
		s := "── " + fn + " "
		s += strings.Repeat("─", max(0, tw-lipgloss.Width(s)))
		switch st {
		case 1, 3:
			return selRow(dim(s), tw, st == 1)
		case 2:
			return lineBG(' ', st) + "\x1b[90m" + s + rst
		}
		return dim(s)
	}
	num := func(n int) string {
		if n == 0 {
			return "    "
		}
		return fmt.Sprintf("%4d", n)
	}
	gut := num(x.old) + " " + num(x.nw) + " │ "
	l := lineBG(x.kind, st&3)
	body := []rune(afterMark(x.text))
	out := l + "\x1b[90m" + gut + "\x1b[39m"
	if mk := curMark(st); mk != "" {
		out = l + mk + "\x1b[90m" + gut[1:] + "\x1b[39m"
	}
	if x.kind == 'f' {
		out += "\x1b[90m"
	}
	if x.ee > x.es {
		eb := bgc(0, 96, 0, 34)
		if x.kind == '-' {
			eb = bgc(144, 16, 17, 124)
		}
		out += string(body[:x.es]) + eb + string(body[x.es:x.ee]) + l + string(body[x.ee:])
	} else {
		out += string(body)
	}
	pad := max(0, tw-lipgloss.Width(gut)-len(body))
	return out + strings.Repeat(" ", pad) + rst
}

// dview returns the file, the doc indexes shown in the current mode and the
// screen rows (a draft adds a row under its line).
func (m *model) dview(o *overlay) ([]dl, []int, []vrow) {
	doc := m.docFor(o)
	var vis []int
	var rows []vrow
	for i, x := range doc {
		if (o.ctx == 0 && x.kind == 'f') || (o.ctx == 1 && x.kind == '@') {
			continue
		}
		vis = append(vis, i)
		rows = append(rows, vrow{li: len(vis) - 1})
		if x.loc >= 0 {
			rows = append(rows, m.threadRows(x.fi, x.loc, len(vis)-1)...)
		}
		var ds [][3]int
		for k := range m.drafts {
			if k[0] == x.fi && k[1] == x.loc {
				ds = append(ds, k)
			}
		}
		slices.SortFunc(ds, func(a, b [3]int) int { return a[2] - b[2] })
		for _, k := range ds {
			d := m.drafts[k]
			rows = append(rows, vrow{text: yellow("   ✎ ") + rev(" draft ") + yellow(" "+d.label+": ") + d.text, li: len(vis) - 1, draft: true, dk: k})
		}
	}
	return doc, vis, rows
}

func cursorRow(rows []vrow, li int) int {
	for i, r := range rows {
		if r.li == li && !r.draft {
			return i
		}
	}
	return 0
}

// curRow is the screen row of the cursor, which may sit on a draft.
func curRow(rows []vrow, o *overlay) int {
	if o.ondraft {
		for i, r := range rows {
			if r.draft && r.dk == o.dkey {
				return i
			}
		}
		o.ondraft = false
	}
	return cursorRow(rows, o.vcur)
}

// selectRow puts the cursor on screen row ri.
func selectRow(o *overlay, rows []vrow, ri int) {
	ri = max(0, min(ri, len(rows)-1))
	o.vcur, o.ondraft, o.dkey = rows[ri].li, rows[ri].draft, rows[ri].dk
}

// fixDiff keeps the cursor on screen.
func (m *model) fixDiff(o *overlay) {
	_, _, h := m.ovSize(o)
	bh := max(1, h-3)
	_, vis, rows := m.dview(o)
	o.vcur = max(0, min(o.vcur, len(vis)-1))
	r := curRow(rows, o)
	if r < o.scroll {
		o.scroll = r
	}
	if r >= o.scroll+bh {
		o.scroll = r - bh + 1
	}
	o.scroll = max(0, min(o.scroll, len(rows)-bh))
}

func (m *model) ovSize(o *overlay) (int, int, int) {
	x, _, w, h := m.ovRect(o)
	return x, w, h
}

// hunkTargets are the doc indexes `}` and `{` jump to in the current mode.
func hunkTargets(doc []dl, mode int) []int {
	var t []int
	for i, x := range doc {
		if x.kind == '@' {
			t = append(t, i+mode)
		}
	}
	return t
}

func (m *model) diffStatus(o *overlay) string {
	if o.cmdOn {
		return ":" + o.cmd + "▏" + dim("   n next file · p previous file")
	}
	_, vis, _ := m.dview(o)
	s := fmt.Sprintf("%d drafts", len(m.drafts))
	if o.ranging {
		s += " · RANGE j/k extend · c comment · esc cancel"
	} else {
		s += " · V range · c comment · ) ( draft · e edit · D delete draft · on a thread c reply T resolve · } { hunk · x mode · :n :p"
	}
	return dim(fmt.Sprintf("L%d/%d · ", o.vcur+1, len(vis)) + s)
}

func (m *model) drawDiff(o *overlay, w, h int) string {
	m.fixDiff(o)
	doc, vis, rows := m.dview(o)
	bh := max(1, h-3)
	cd := vis[o.vcur]
	lo, hi := min(o.anchor, cd), max(o.anchor, cd)
	foc := !o.embed || m.pfocus == 1
	var lines []string
	for i := o.scroll; i < min(len(rows), o.scroll+bh); i++ {
		r := rows[i]
		t := r.text
		if r.draft {
			if o.ondraft && r.dk == o.dkey {
				t = selRow(t, w-4, foc)
			}
		} else {
			di := vis[r.li]
			x := doc[di]
			st := 0
			switch {
			case r.li == o.vcur && !o.ondraft:
				st = 1
				if !foc {
					st = 3
				}
			case o.ranging && di >= lo && di <= hi:
				st = 2
			case o.ondraft && x.fi == o.dkey[0] && x.loc >= o.dkey[2] && x.loc <= o.dkey[1]:
				st = 2
			case m.inThreadRange(x):
				st = 4
			}
			t = renderDL(x, w, st)
		}
		lines = append(lines, t)
	}
	for len(lines) < bh {
		lines = append(lines, "")
	}
	lines = append(lines, m.diffStatus(o))
	cf := doc[cd].fi
	var title string
	if o.all {
		title = fmt.Sprintf("Whole patch · %s (%d/%d) · %s [x] · %d%%", files[cf].path(), cf+1, len(files), ctxNames[o.ctx], (o.vcur+1)*100/len(vis))
	} else {
		title = fmt.Sprintf("Diff · %s (%d/%d) · %s [x] · %d%%", files[cf].path(), cf+1, len(files), ctxNames[o.ctx], (o.vcur+1)*100/len(vis))
	}
	return box(title, lines, w, h, foc)
}

// toggleMode switches hunks only / full file keeping the cursor line on the
// same screen row; a line outside every hunk lands on the nearest hunk line.
func (m *model) toggleMode(o *overlay) {
	_, vis, rows := m.dview(o)
	di, screen := vis[o.vcur], curRow(rows, o)-o.scroll
	o.ondraft = false
	o.ctx = 1 - o.ctx
	_, vis, rows = m.dview(o)
	best := 0
	for i, d := range vis {
		if abs(d-di) <= abs(vis[best]-di) {
			best = i
		}
	}
	o.vcur = best
	o.scroll = max(0, cursorRow(rows, best)-screen)
	m.fixDiff(o)
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

func (m *model) moveCursor(o *overlay, d int) {
	_, _, rows := m.dview(o)
	selectRow(o, rows, curRow(rows, o)+d)
	m.fixDiff(o)
}

// jumpDraft selects the next (dir 1) or previous draft row.
func (m *model) jumpDraft(o *overlay, dir int) {
	_, _, rows := m.dview(o)
	cur := curRow(rows, o)
	for i := cur + dir; i >= 0 && i < len(rows); i += dir {
		if rows[i].draft {
			selectRow(o, rows, i)
			m.fixDiff(o)
			return
		}
	}
}

// goDoc puts the cursor on doc line di if it is shown.
func (m *model) goDoc(o *overlay, di int) {
	_, vis, _ := m.dview(o)
	o.ondraft = false
	for i, d := range vis {
		if d == di {
			o.vcur = i
		}
	}
	m.fixDiff(o)
}

// jumpFile moves between the file headers of the whole patch.
func (m *model) jumpFile(o *overlay, dir int) {
	doc, vis, _ := m.dview(o)
	cd := vis[o.vcur]
	to := -1
	for i, x := range doc {
		if x.kind != 'H' {
			continue
		}
		if dir > 0 && i > cd {
			to = i
			break
		}
		if dir < 0 && i < cd {
			to = i
		}
	}
	if to >= 0 {
		m.goDoc(o, to)
	}
}

// toggleAll switches between this file and the whole patch, keeping the
// cursor on the same source line.
func (m *model) toggleAll(o *overlay) {
	doc, vis, _ := m.dview(o)
	x := doc[vis[o.vcur]]
	o.all = !o.all
	o.arg, o.ondraft = x.fi, false
	doc, vis, _ = m.dview(o)
	best := 0
	for i, d := range vis {
		if doc[d].fi == x.fi && abs(doc[d].loc-x.loc) <= abs(doc[vis[best]].loc-x.loc) || doc[vis[best]].fi != x.fi && doc[d].fi == x.fi {
			best = i
		}
	}
	o.vcur, o.scroll = best, max(0, best-5)
	m.fixDiff(o)
}

func (m *model) jumpHunk(o *overlay, dir int) {
	doc, vis, _ := m.dview(o)
	cd := vis[o.vcur]
	ts := hunkTargets(doc, o.ctx)
	to := -1
	for _, t := range ts {
		if dir > 0 && t > cd {
			to = t
			break
		}
		if dir < 0 && t < cd {
			to = t
		}
	}
	if to < 0 {
		return
	}
	m.goDoc(o, to)
}

// diffKey handles a key in the pager-like diff modal.
func (m *model) diffKey(o *overlay, msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	if o.compose {
		switch k {
		case "esc":
			o.compose, o.editing = false, false
		case "enter":
			if strings.TrimSpace(o.ctext) != "" {
				m.drafts[[3]int{o.cfile, o.cend, o.clo}] = draft{o.clabel, o.ctext}
			}
			o.compose, o.ranging, o.editing = false, false, false
		case "backspace":
			if r := []rune(o.ctext); len(r) > 0 {
				o.ctext = string(r[:len(r)-1])
			}
		default:
			o.ctext += msg.Text
		}
		return nil
	}
	if o.cmdOn {
		switch k {
		case "esc":
			o.cmdOn = false
		case "enter":
			c := o.cmd
			o.cmdOn = false
			switch c {
			case "n", "next":
				m.nextFile(o, 1)
			case "p", "prev", "N":
				m.nextFile(o, -1)
			default:
				return m.say("(prototype) unknown command :" + c)
			}
		case "backspace":
			if o.cmd == "" {
				o.cmdOn = false
			} else {
				o.cmd = o.cmd[:len(o.cmd)-1]
			}
		default:
			o.cmd += msg.Text
		}
		return nil
	}
	doc, vis, _ := m.dview(o)
	cd := vis[o.vcur]
	if o.ondraft && o.dkey[0] < 0 {
		if t, ok := m.tgtOfKey(o.dkey); ok {
			switch k {
			case "c", "T", "e", "X", "D", "enter":
				return m.act(k, t)
			}
		}
	}
	switch k {
	case ":":
		o.cmdOn, o.cmd = true, ""
	case "esc":
		if o.ranging {
			o.ranging = false
		} else {
			m.leave(o)
		}
	case "backspace", "q":
		m.leave(o)
	case "Z":
		if !o.embed {
			o.max = !o.max
			m.fixDiff(o)
		}
	case "x":
		m.toggleMode(o)
	case "a":
		m.toggleAll(o)
	case ")":
		m.jumpDraft(o, 1)
	case "(":
		m.jumpDraft(o, -1)
	case "e":
		if !o.ondraft {
			return m.say("move to a draft to edit it")
		}
		d := m.drafts[o.dkey]
		o.compose, o.editing, o.ctext, o.clabel = true, true, d.text, d.label
		o.cfile, o.cend, o.clo = o.dkey[0], o.dkey[1], o.dkey[2]
	case "J", "]":
		m.nextFile(o, 1)
	case "K", "[":
		m.nextFile(o, -1)
	case "}":
		m.jumpHunk(o, 1)
	case "{":
		m.jumpHunk(o, -1)
	case "V":
		o.ranging, o.anchor = !o.ranging, cd
	case "D":
		if !o.ondraft {
			return m.say("move to a draft to delete it")
		}
		key := o.dkey
		m.confirmDo("Delete the draft "+m.drafts[key].label+"?", "draft deleted", func(m *model) tea.Cmd {
			delete(m.drafts, key)
			if o := m.diffTop(); o != nil {
				o.ondraft = false
				m.fixDiff(o)
			}
			return nil
		})
	case "c", "enter":
		lo, hi := cd, cd
		if o.ranging {
			lo, hi = min(o.anchor, cd), max(o.anchor, cd)
		}
		o.compose, o.editing, o.ctext, o.cend = true, false, "", doc[hi].loc
		o.cfile, o.clo = doc[hi].fi, doc[lo].loc
		if doc[lo].fi != doc[hi].fi {
			o.clo = 0
		}
		if o.cend < 0 {
			o.compose = false
			return nil
		}
		a, b := doc[lo].src, doc[hi].src
		o.clabel = fmt.Sprintf("L%d", a)
		if a != b {
			o.clabel = fmt.Sprintf("L%d–%d", a, b)
		}
	default:
		if d := moveDelta(k); d != 0 {
			m.moveCursor(o, d)
		}
	}
	return nil
}

func (m *model) drawComposer(o *overlay) (string, int, int, int) {
	x, y, w, h := m.ovRect(o)
	cw := min(60, w-4)
	s := box("New draft comment · "+o.clabel, []string{o.ctext + "▏", dim("enter save draft · esc cancel")}, cw, 4, true)
	return s, x + (w-cw)/2, y + (h-4)/2, cw
}

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

const description = "## Summary\n" +
	"\n" +
	"Adds a small `calc` package with `Add`, `Div` and `Mul` helpers, and uses it from the app command. Division by zero still panics for now; the review thread covers what to do about it.\n" +
	"\n" +
	"## Why\n" +
	"\n" +
	"The app command had three copies of the same arithmetic, and each one handled edge cases a little differently. One package means one place to fix them, and one place to test.\n" +
	"\n" +
	"## Changes\n" +
	"\n" +
	"- add `calc.Add`, `calc.Div` and `calc.Mul` in `calc/calc.go`\n" +
	"- replace the inline arithmetic in `cmd/app/main.go` with calls to the package\n" +
	"- document the package in `calc/doc.go`\n" +
	"- return an error from `Div` instead of panicking on a zero divisor\n" +
	"- keep the old behaviour behind `--legacy-math` until the next release\n" +
	"\n" +
	"<details>\n<summary>Benchmarks</summary>\n\n- `Add`: 0.3 ns/op\n- `Div`: 0.4 ns/op\n</details>\n" +
	"\n" +
	"A call now looks like this:\n" +
	"\n" +
	"```go\n" +
	"q, err := calc.Div(total, count)\n" +
	"if err != nil {\n" +
	"\treturn fmt.Errorf(\"average: %w\", err)\n" +
	"}\n" +
	"```\n" +
	"\n" +
	"## Testing\n" +
	"\n" +
	"- `go test ./calc/...` covers every helper, including division by zero\n" +
	"- ran the app command by hand with empty and non-empty input\n" +
	"- the `e2e` job is flaky on this branch; a fix is in a separate pull request"

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

	// pager-like diff: cursor (index into the shown lines), range anchor (doc
	// index), comment composer, and the Z maximize flag of any overlay
	vcur    int
	ranging bool
	anchor  int
	compose bool
	ctext   string
	clabel  string
	cend    int
	cfile   int
	clo     int
	editing bool
	all     bool // the whole patch instead of one file
	max     bool
	ondraft bool   // the cursor is on a draft row
	dkey    [3]int // which draft
	embed   bool   // the pager shown beside the zoomed files list

	title  string
	items  []string
	sel    map[string]bool
	multi  bool
	filter string
	fon    bool // the filter line is open and takes the typing
	pcur   int
	people bool // items are personItems: the login shows dim after the name
	done   func(*model, []string) tea.Cmd

	text  string
	toast string

	// the reply composer and the PR editor
	save   func(*model, string, string) tea.Cmd
	etitle string
	efield int
}

type model struct {
	w, h      int
	focus     int
	zoom      bool
	drafts    map[[3]int]draft
	pv        overlay // the embedded pager of the zoomed files panel
	pfocus    int     // 0 list, 1 diff
	pvOn      bool
	scroll    int
	dscroll   int // the description pane's scroll
	cur       [nPanes]int
	stack     []overlay
	toast     string
	toastID   int
	reviewers []reviewer
	dev       []devItem
	meta      [4]metaRow
	rows      []row
	cv        convState
}

type clearToastMsg struct{ id int }

func newModel() *model {
	m := &model{
		focus:     pMain,
		drafts:    map[[3]int]draft{},
		cv:        newConv(),
		pv:        overlay{kind: "file", embed: true},
		reviewers: []reviewer{{"bob", "approved"}, {"carol", "changes"}, {"org/core", "requested"}},
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
	// root files come first as plain names, then one folder row per directory
	for i, f := range files {
		if f.dir == "" {
			m.rows = append(m.rows, row{fi: i})
		}
	}
	last := "?"
	for i, f := range files {
		if f.dir == "" {
			continue
		}
		if f.dir != last {
			m.rows = append(m.rows, row{dir: true, label: f.dir})
			last = f.dir
		}
		m.rows = append(m.rows, row{fi: i})
	}
	m.cur[pFiles] = 0
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

// descH is the description pane's height beside the conversation: a third of
// the body, at least 5 rows.
func (m *model) descH() int {
	H := m.bodyH()
	return min(max(5, H/3), max(3, H-4))
}

func (m *model) panelHeights() [nPanes]int {
	H := m.bodyH()
	nat := [nPanes]int{pMerge: 7, pReviewers: max(3, len(m.reviewers)+2), pDev: max(3, len(m.dev)+2), pMeta: 6}
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
	nat[pMain] = H - m.descH()
	nat[pDesc] = m.descH()
	return nat
}

func (m *model) paneDims(i int) (int, int) {
	if m.split() && i == pFiles {
		return listW, m.bodyH()
	}
	if m.zoomed() {
		return m.w, m.bodyH()
	}
	if i == pMain || i == pDesc {
		return m.w - rightW, m.panelHeights()[i]
	}
	return rightW, m.panelHeights()[i]
}

// ---------- content ----------

func wrap(s string, w int) []string {
	return strings.Split(ansi.Wrap(s, max(w, 10), ""), "\n")
}

func (m *model) oldConvLines(cw int) []string {
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
	if m.cur[pane] == i {
		w, _ := m.paneDims(pane)
		return selRow("  "+text, w-4, m.focus == pane)
	}
	return "  " + text
}

func (m *model) paneLines(i, cw int) ([]string, int) {
	switch i {
	case pDesc:
		return mdLines(m.cv.items[0].body, cw), 0
	case pMain:
		return m.convLines(cw), 0
	case pMerge:
		return []string{
			yellow("Blocked: changes requested"),
			m.mrow(i, 0, ic.checkFail.String()+" 1 of 12 checks failing"+dim("  C")),
			m.mrow(i, 1, "Merge (squash)"+dim("  M")),
			m.mrow(i, 2, "Auto-merge"+dim("  A")),
			m.mrow(i, 3, "Update branch"+dim("  U")),
		}, m.cur[i] + 1
	case pReviewers:
		var out []string
		for k, r := range m.reviewers {
			icon := ic.requested.String()
			switch r.state {
			case "approved":
				icon = ic.approved.String()
			case "changes":
				icon = ic.changes.String()
			case "re-requested":
				icon = ic.rerequested.String()
			}
			out = append(out, m.mrow(i, k, icon+" "+dispS(r.name)+dim(" "+r.state)))
		}
		if len(out) == 0 {
			out = append(out, dim("  no reviewers (a to request)"))
		}
		return out, m.cur[i]
	case pDev:
		var out []string
		for k, d := range m.dev {
			st, g := green("open"), ic.issueOpen
			if d.kind == "pr" {
				g = ic.prOpen
			}
			if d.state == "closed" {
				st, g = red("closed"), ic.issueClosed
				if d.kind == "pr" {
					g = ic.prClosed
				}
			}
			t := g.String() + fmt.Sprintf(" closes #%d ", d.num)
			if d.kind == "pr" {
				t = g.String() + fmt.Sprintf(" PR #%d ", d.num)
			}
			t += st
			if d.blocked != "" {
				t += " " + ic.blocked.String() + " " + yellow(d.blocked)
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
				t += " " + ic.unresolved.String()
			}
			if m.cur[pFiles] == k {
				w, _ := m.paneDims(pFiles)
				out = append(out, selRow("  "+t, w-4, m.focus == pFiles))
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
			vals := r.vals
			if r.label == "Assignees" {
				vals = nil
				for _, l := range r.vals {
					vals = append(vals, disp(l))
				}
			}
			v := strings.Join(vals, ", ")
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
	if i == pMain {
		n, tot := m.cv.counts()
		t += fmt.Sprintf(" · %d of %d shown · filter: %s", n, tot, filterNames[m.cv.filter])
		if m.cv.hideSys {
			t += " · events hidden"
		}
	}
	if i == pDesc && m.cv.items[0].edited {
		t += " · edited"
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
	if i == pMain || i == pDesc {
		off = min(m.scroll, max(0, len(lines)-ih))
		if i == pDesc {
			off = min(m.dscroll, max(0, len(lines)-ih))
		}
		if len(lines) > ih {
			title += fmt.Sprintf(" · %d%%", (off+ih)*100/len(lines))
		}
	} else {
		off = max(0, cl-ih+1)
	}
	end := min(len(lines), off+ih)
	return box(title, lines[off:end], w, h, m.focus == i && !(m.split() && m.pfocus == 1))
}

// ---------- overlays ----------

func (m *model) ovRect(o *overlay) (int, int, int, int) {
	W, H := m.w, m.h-1
	if o.embed {
		return listW, m.bodyTop(), W - listW, m.bodyH()
	}
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
	if m.narrow() || o.max {
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
		return "", nil
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
			g := ic.checkOK
			if c.state == "fail" {
				g = ic.checkFail
			}
			out = append(out, fmt.Sprintf("%s %-18s %s", g.String(), c.name, dim(c.dur)))
		}
		return "Checks · 12 runs", out
	case "issue":
		d := m.dev[o.arg]
		kind := "Issue"
		if d.kind == "pr" {
			kind = "Pull request"
		}
		st := ic.issueOpen.String() + " " + green("open")
		if d.kind == "pr" {
			st = ic.prOpen.String() + " " + green("open")
		}
		if d.state == "closed" {
			st = ic.issueClosed.String() + " " + icPurple("closed")
			if d.kind == "pr" {
				st = ic.prClosed.String() + " " + red("closed")
			}
		}
		out := []string{bold(d.title), st}
		if d.blocked != "" {
			out = append(out, ic.blocked.String()+" "+yellow("blocked by "+d.blocked+" (open)"))
		}
		out = append(out, "", "Opened by Dave Kim 2 weeks ago. Labels: bug.", "",
			"Fake summary of the issue body. The real one would be the markdown of the description, with the latest comments below it.")
		return fmt.Sprintf("%s #%d", kind, d.num), out
	case "help":
		return "Keys", []string{
			"tab / shift+tab   next / previous pane      1-7  jump to a pane",
			"j k ↑ ↓ pgup pgdn scroll or move            z    zoom the focused pane (main screen)",
			"esc / backspace   close the top overlay     q    quit",
			"",
			"Description 1: the PR body as light markdown · j/k pgup/pgdn scroll · e edit title and description",
			"            (tab/ctrl+s/esc in the editor) · below 120 cols the panes are a strip: description first",
			"People      shown by full name (login when there is none, a team as \"core team\"); pickers match",
			"            the name or the login, which shows dim after the name",
			"Conversation a tree: j/k move · l/→ open (or into) · h/← close (or to parent) · enter toggle (not on a thread) · * open/close all",
			"            enter/o on a thread opens the diff on its line (x full file, esc back to this row); fold a thread with l/h",
			"            f filter all/unresolved/changes requested/mine · s show/hide system events (pushes, CI, labels, review requests)",
			"            ) ( next/prev unresolved thread; enter on a push opens its commits, aligned under the text",
			"            <details> blocks in bodies are one row \"Title ⋯\": l/→ open · h/← close · * all (blocks too); open ones nest under a dim rule",
			"            Description pane: enter opens/closes the first details block at or below the top line",
			"            c reply on a thread (flat, at its end) or quote reply on a comment/review (new comment, GitHub has no reply there) · T resolve/reopen a thread (y/n) · e edit your comment/reply (the PR is edited in Description)",
			"            X delete your comment/reply (y/n) · editor: tab field · ctrl+s save · esc cancel",
			"            threads show in the diff under their line: c reply · T resolve · enter fold · ) ( reach them",
			"Icons       Octicons (PRPROTO_ICONS=unicode for plain glyphs). Threads: " + ic.unresolved.String() + " unresolved · " + ic.resolved.String() + " resolved (row dimmed) · " + ic.outdated.String() + " outdated",
			"            reviews: " + ic.approved.String() + " approved · " + ic.changes.String() + " changes requested · " + ic.commented.String() + " commented · " + ic.requested.String() + " requested · " + ic.rerequested.String() + " re-requested · " + ic.dismissed.String() + " dismissed",
			"            checks: " + ic.checkOK.String() + " pass · " + ic.checkFail.String() + " fail · " + ic.checkPending.String() + " pending   PRs: " + ic.prOpen.String() + " open · " + ic.prDraft.String() + " draft · " + ic.merged.String() + " merged · " + ic.prClosed.String() + " closed",
			"            issues: " + ic.issueOpen.String() + " open · " + ic.issueClosed.String() + " closed · " + ic.blocked.String() + " blocked",
			"            right edge of a row: ⋯ 3 folded replies,",
			"            2 threads ⋯ folded review, ⋯ 3 comments folded run, then edited and age like 20h, 2d)",
			"Merge       enter/C checks · M merge · A auto-merge · U update branch",
			"Reviewers   a request a reviewer · R re-request review (asks y/n) · x remove",
			"Files zoom  z on Files at 120+ cols splits list | diff: tab/1/2 focus · enter loads the file",
			"Development enter issue summary · a link issue · x unlink",
			"Files       enter file diff · D combined diff · l focus conversation",
			"Meta        enter edit the field",
			"",
			"Overlays    Z maximizes the overlay to the whole area (z zooms a pane on the main screen)",
			"In a diff   a pager: j/k line cursor · } { next/prev hunk · :n :p or J K next/prev file",
			"            x toggles hunks only / full file (the cursor line stays on its row)",
			"            V range (j/k extend) · c or enter comment (enter saves a draft, esc cancels)",
			"            c alone comments the cursor line · ) ( next/prev draft (j/k also land on drafts)",
			"            e edits the draft under the cursor · D deletes it (y/n) · a this file / whole patch",
			"In a picker j/k g/G move · / filters (enter or esc leaves the line) · space toggles (multi) · enter picks · esc closes",
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
		var lines []string
		fl := o.filtered()
		slots := max(1, h-3)
		off := max(0, o.pcur-slots+1)
		for i := off; i < min(len(fl), off+slots); i++ {
			p := "  "
			mk := ""
			label := fl[i]
			if o.people {
				if n, l, ok := strings.Cut(label, "  "); ok {
					label = n + dim("  "+l)
				}
			}
			if o.multi {
				mk = "[ ] "
				if o.sel[fl[i]] {
					mk = "[x] "
				}
			}
			if i == o.pcur {
				lines = append(lines, selRow(p+mk+fl[i], w-4, true))
				continue
			}
			lines = append(lines, p+mk+label)
		}
		for len(lines) < slots {
			lines = append(lines, "")
		}
		fline := dim(strings.Repeat("─", w-4))
		if o.fon {
			fline = accent("/") + o.filter + "▏"
		}
		lines = append(lines, fline)
		title := o.title
		if o.filter != "" {
			title += " · /" + o.filter
		}
		if o.multi {
			n := 0
			for _, v := range o.sel {
				if v {
					n++
				}
			}
			title += fmt.Sprintf(" · %d of %d", n, len(o.items))
		} else {
			title += fmt.Sprintf(" · %d of %d", len(fl), len(o.items))
		}
		s = box(title, lines, w, h, true)
	case "file":
		s = m.drawDiff(o, w, h)
	case "reply", "editpr":
		return m.drawCompose(o)
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
	o.scroll, o.vcur, o.ranging, o.ondraft = 0, 0, false, false
	if o.embed {
		m.cur[pFiles] = o.cur
	}
}

// nextFile moves to the next or previous file, or file header in the whole patch.
func (m *model) nextFile(o *overlay, d int) {
	if o.all {
		m.jumpFile(o, d)
		return
	}
	m.showFile(o, o.arg+d)
}

// leave closes the pager, or hands the focus back to the files list.
func (m *model) leave(o *overlay) {
	if o.embed {
		m.pfocus = 0
		return
	}
	m.pop()
}

// diffTop is the pager that has the keys now.
func (m *model) diffTop() *overlay {
	if n := len(m.stack); n > 0 && m.stack[n-1].kind == "file" {
		return &m.stack[n-1]
	}
	if m.split() {
		return &m.pv
	}
	return nil
}

func (m *model) confirmDo(text, toast string, fn func(*model) tea.Cmd) {
	m.push(overlay{kind: "confirm", text: text, toast: toast, done: func(m *model, _ []string) tea.Cmd { return fn(m) }})
}

// split: the zoomed files panel shows the list and the pager side by side.
func (m *model) split() bool { return m.zoom && !m.narrow() && m.focus == pFiles }

const listW = 32

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
	hdr := bold("#42 "+m.cv.title) + dim(" · ") + ic.prOpen.String() + " " + green("open") + " · " + disp("alice") + " " + dim("feat/calc → main") +
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
	if m.split() {
		w, h := m.paneDims(pFiles)
		_ = w
		_ = h
		_, _, dw, dh := m.ovRect(&m.pv)
		parts = append(parts, lipgloss.JoinHorizontal(lipgloss.Top, m.pane(pFiles), m.drawDiff(&m.pv, dw, dh)))
	} else if m.zoomed() {
		parts = append(parts, m.pane(m.focus))
	} else {
		var col []string
		for i := pMerge; i <= pMeta; i++ {
			col = append(col, m.pane(i))
		}
		left := lipgloss.JoinVertical(lipgloss.Left, m.pane(pDesc), m.pane(pMain))
		parts = append(parts, lipgloss.JoinHorizontal(lipgloss.Top, left, lipgloss.JoinVertical(lipgloss.Left, col...)))
	}
	lines := strings.Split(strings.Join(parts, "\n"), "\n")
	for len(lines) < m.h-1 {
		lines = append(lines, "")
	}
	lines = lines[:m.h-1]
	if m.split() && m.pv.compose {
		s, x, y, w := m.drawComposer(&m.pv)
		place(lines, s, x, y, w)
	}
	for i := range m.stack {
		for j := 1; j < len(lines); j++ {
			l := lines[j]
			lines[j] = dim(ansi.Strip(l))
		}
		s, x, y, w := m.drawOverlay(&m.stack[i])
		place(lines, s, x, y, w)
		if o := &m.stack[i]; o.kind == "file" && o.compose {
			s, x, y, w := m.drawComposer(o)
			place(lines, s, x, y, w)
		}
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
			if o.fon {
				return dim("type to filter · enter/esc leave the filter line")
			}
			if o.multi {
				return dim("j/k move · / filter · space toggle · enter apply · esc cancel")
			}
			return dim("j/k move · / filter · enter pick · esc cancel")
		case "reply":
			return dim("type the reply · enter save · esc cancel")
		case "editpr":
			return dim("tab title/body · enter newline · ctrl+s save · esc cancel")
		case "file":
			if o.compose {
				return dim("type the comment · enter save draft · esc cancel")
			}
			return dim(fmt.Sprintf("%d drafts · j/k · } { hunk · V range · c comment · x mode · :n :p · Z max · esc close", len(m.drafts)))
		default:
			return dim("j/k scroll · esc/backspace close")
		}
	}
	h := map[int]string{
		pDesc:      "j/k scroll · pgup/pgdn page · e edit title and description",
		pMain:      "j/k move · h/l fold · * all · f filter · ) ( unresolved · c " + m.replyVerb() + " · T resolve · e edit · X delete",
		pMerge:     "enter/C checks · M merge · A auto-merge · U update",
		pReviewers: "a request · R re-request · x remove",
		pDev:       "enter open · a link · x unlink",
		pFiles:     "enter diff · D combined · l conversation",
		pMeta:      "enter edit",
	}[m.focus]
	return dim(h + " · tab/1-7 focus · z zoom · ? help · q quit")
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
	defer m.fixConv()
	if len(m.stack) > 0 {
		return m.overlayKey(msg)
	}
	defer m.syncPV()
	if m.split() {
		p := &m.pv
		if m.pfocus == 1 && (p.compose || p.cmdOn) {
			return m.diffKey(p, msg)
		}
		switch k {
		case "tab", "shift+tab":
			m.pfocus = 1 - m.pfocus
			return nil
		case "1":
			m.pfocus = 0
			return nil
		case "2":
			m.pfocus = 1
			return nil
		}
		if m.pfocus == 1 && k != "z" && k != "?" {
			return m.diffKey(p, msg)
		}
	}
	switch k {
	case "q":
		return tea.Quit
	case "tab":
		m.focus = (m.focus + 1) % nPanes
	case "shift+tab":
		m.focus = (m.focus + nPanes - 1) % nPanes
	case "1", "2", "3", "4", "5", "6", "7":
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

// syncPV points the embedded pager at the file under the list cursor when the
// split appears.
func (m *model) syncPV() {
	if !m.split() {
		m.pvOn = false
		return
	}
	if m.pvOn {
		return
	}
	m.pvOn, m.pfocus = true, 0
	if r := m.rows[m.cur[pFiles]]; !r.dir {
		m.showFile(&m.pv, r.fi)
	}
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
	if m.focus == pDesc {
		w, h := m.paneDims(pDesc)
		m.dscroll = max(0, min(m.dscroll+d, len(mdLines(m.cv.items[0].body, w-4))-(h-2)))
		return
	}
	if m.focus == pMain {
		rows := m.crows(false)
		if len(rows) > 0 {
			m.setSel(rows, m.selIdx(rows)+d)
		}
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
	case pDesc:
		if k == "e" {
			m.editPR()
		}
		if k == "enter" {
			m.toggleDesc()
		}
	case pMain:
		return m.convKey(k)
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
					items = append(items, personItem(n))
				}
			}
			m.push(overlay{kind: "picker", title: "Request a reviewer", items: items, people: true, done: func(m *model, p []string) tea.Cmd {
				m.reviewers = append(m.reviewers, reviewer{loginOf(p[0]), "requested"})
				return m.say("(prototype) requested " + disp(loginOf(p[0])))
			}})
		case "R":
			c := m.cur[pReviewers]
			if c >= len(m.reviewers) {
				return nil
			}
			r := m.reviewers[c]
			if r.state == "requested" || r.state == "re-requested" {
				return m.say("review is already requested from " + disp(r.name))
			}
			m.confirmDo("Re-request review from "+disp(r.name)+"? ", "(prototype) re-requested "+disp(r.name), func(m *model) tea.Cmd {
				m.reviewers[c].state = "re-requested"
				return nil
			})
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
				if m.split() {
					m.showFile(&m.pv, r.fi)
				} else {
					m.push(overlay{kind: "file", arg: r.fi, cur: m.cur[pFiles]})
				}
			}
		case "D":
			m.push(overlay{kind: "file", all: true})
		case "l", "right":
			if m.split() {
				m.pfocus = 1
			} else {
				m.focus = pMain
			}
		}
	case pMeta:
		if k == "enter" {
			r := m.meta[m.cur[pMeta]]
			people := r.label == "Assignees"
			conv := func(s string) string {
				if people {
					return personItem(s)
				}
				return s
			}
			sel := map[string]bool{}
			for _, v := range r.vals {
				sel[conv(v)] = true
			}
			var opts []string
			for _, o := range r.opts {
				opts = append(opts, conv(o))
			}
			idx := m.cur[pMeta]
			m.push(overlay{kind: "picker", title: r.label, items: opts, sel: sel, multi: true, people: people, done: func(m *model, p []string) tea.Cmd {
				if people {
					for i := range p {
						p[i] = loginOf(p[i])
					}
				}
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
			t, done := top.toast, top.done
			m.pop()
			if done != nil {
				done(m, nil)
			}
			return m.say(t)
		case "n", "esc", "backspace":
			m.pop()
		}
	case "picker":
		fl := top.filtered()
		if top.fon {
			switch k {
			case "esc", "enter":
				top.fon = false
			case "up", "ctrl+p":
				top.pcur = max(0, top.pcur-1)
			case "down", "ctrl+n":
				top.pcur = max(0, min(top.pcur+1, len(fl)-1))
			case "backspace":
				if top.filter == "" {
					top.fon = false
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
			return nil
		}
		switch k {
		case "esc", "backspace":
			m.pop()
		case "/":
			top.fon = true
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
		case "up", "k", "ctrl+p":
			top.pcur = max(0, top.pcur-1)
		case "down", "j", "ctrl+n":
			top.pcur = max(0, min(top.pcur+1, len(fl)-1))
		case "g":
			top.pcur = 0
		case "G":
			top.pcur = max(0, len(fl)-1)
		case "space", " ":
			if top.multi && top.pcur < len(fl) {
				top.sel[fl[top.pcur]] = !top.sel[fl[top.pcur]]
			}
		}
	case "file":
		return m.diffKey(top, msg)
	case "reply", "editpr":
		return m.composeKey(top, msg)
	default:
		switch k {
		case "esc", "backspace", "q":
			m.pop()
			return nil
		case "Z":
			top.max = !top.max
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
