package main

// The conversation pane as a tree with a cursor: the PR, comments and reviews
// (level 1), a review's threads (level 2) and their replies (level 3). The
// threads are shared with the diff pager, which shows them under their line.

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const me = "alice"

var nextID = 100

func newID() int { nextID++; return nextID }

type reply struct {
	id              int
	who, when, body string
	edited, pending bool
}

type thread struct {
	id            int
	fi, line, loc int
	snippet       string
	start         int // first commented line when the comment spans several
	outdated      bool
	resolved      bool
	replies       []*reply
}

func (t *thread) label() string { return fmt.Sprintf("%s:%d", files[t.fi].name, t.line) }

// span is the commented line range, "12" or "11-12".
func (t *thread) span() string {
	if t.start > 0 && t.start < t.line {
		return fmt.Sprintf("%d-%d", t.start, t.line)
	}
	return fmt.Sprint(t.line)
}

func fileGlyph(name string) string {
	if strings.HasSuffix(name, ".go") {
		return ""
	}
	return ""
}

// threadBlock draws the code a review thread is on as a block of its own: a
// header with the file and lines, then the last diff lines up to the
// commented one, drawn as the diff pager draws them, all behind a dim rule,
// and a blank line before the first comment.
func threadBlock(t *thread, w int) []string {
	head := fileGlyph(files[t.fi].name) + " " + files[t.fi].path() + ":" + t.span()
	if t.outdated {
		head += " " + outdatedIcon
	}
	out := []string{dim(head)}
	doc := fileDoc(files[t.fi], t.fi)
	var pick []dl
	for i := t.loc; i >= 0 && len(pick) < 4; i-- {
		if k := doc[i].kind; k == '+' || k == '-' || k == 'h' {
			pick = append([]dl{doc[i]}, pick...)
		} else if k == '@' {
			break
		}
	}
	start := t.start
	if start == 0 {
		start = t.line
	}
	for _, x := range pick {
		st := 0
		if x.kind != '-' && x.src >= start && x.src <= t.line {
			st = 3
		}
		out = append(out, renderDL(x, max(24, w)+4, st))
	}
	return append(out, "")
}

type citem struct {
	id                           int
	kind, state, who, when, body string
	edited                       bool
	threads                      []*thread
	commits                      [][2]string // a push: short SHA and subject
	forced                       bool
}

// sysText is the plain text of a system line: a push or another event of no human.
func (it *citem) sysText() string {
	if it.kind != "push" {
		return it.body
	}
	n := len(it.commits)
	s := fmt.Sprintf("%s pushed %d commit", disp(it.who), n)
	if n != 1 {
		s += "s"
	}
	s += " (" + it.commits[0][0]
	if n > 1 {
		s += "…" + it.commits[n-1][0]
	}
	s += ")"
	if it.forced {
		s += " · forced"
	}
	return s
}

// forcedStyle is the dim warning colour of "forced".
var forcedStyle = sty("130")

// sysLines centres a dim system line with a short rule on each side; a line
// too wide for the pane drops the rules and wraps, centred.
func sysLines(text string, cw int) []string {
	paint := func(s string) string {
		a, b, ok := strings.Cut(s, "forced")
		if !ok {
			return dim(s)
		}
		return dim(a) + forcedStyle("forced") + dim(b)
	}
	centre := func(s string, w int) string {
		return " " + strings.Repeat(" ", max(0, (cw-1-w)/2)) + s
	}
	if rule, _ := sysFit(text, cw); rule > 0 {
		r := dim(strings.Repeat("─", rule))
		return []string{centre(r+" "+paint(text)+" "+r, len([]rune(text))+2*(rule+1))}
	}
	var out []string
	for _, l := range wrap(text, max(10, cw-4)) {
		out = append(out, centre(paint(l), len([]rune(l))))
	}
	return out
}

// sysFit gives the rule length a system line fits with (0 when it must wrap)
// and the column where its text starts.
func sysFit(text string, cw int) (rule, start int) {
	for _, rule := range []int{4, 2} {
		w := len([]rune(text)) + 2*(rule+1)
		if w <= cw-1 {
			return rule, 1 + max(0, (cw-1-w)/2) + rule + 1
		}
	}
	return 0, 2
}

// commitLine is a commit row of an open push: it starts where the push line's
// text does, and a subject too long for the room is cut with an ellipsis.
func commitLine(text string, c [2]string, cw int, gm string) string {
	_, start := sysFit(text, cw)
	left := strings.Repeat(" ", max(0, start-1)) + gm + dim(c[0]) + "  " + c[1]
	return rightAlign(left, checkIcon(commitState(c[0])), cw)
}

type convState struct {
	items    []*citem
	open     map[string]bool
	sel      string
	last     int
	filter   int
	hideSys  bool // system events (pushes, CI, labels, review requests) hidden
	title    string
	first    int // first and last line of the selected row in the last render
	lastLine int
}

var filterNames = []string{"all", "unresolved", "changes requested", "mine"}

func itKey(it *citem) string { return fmt.Sprintf("it:%d", it.id) }
func thKey(t *thread) string { return fmt.Sprintf("th:%d", t.id) }
func (c *convState) isOpen(k string, def bool) bool {
	if v, ok := c.open[k]; ok {
		return v
	}
	return def
}

func locFor(fi, line int) int {
	for _, x := range fileDoc(files[fi], fi) {
		if x.src == line && (x.kind == '+' || x.kind == 'h' || x.kind == 'f') {
			return x.loc
		}
	}
	return -1
}

func newConv() convState {
	rp := func(who, when, body string) *reply { return &reply{id: newID(), who: who, when: when, body: body} }
	th := func(line int, snippet string, resolved bool, rs ...*reply) *thread {
		return &thread{id: newID(), fi: 0, line: line, loc: locFor(0, line), snippet: snippet, resolved: resolved, replies: rs}
	}
	span := func(start int, outdated bool, t *thread) *thread {
		t.start, t.outdated = start, outdated
		return t
	}
	it := func(kind, state, who, when, body string, ths ...*thread) *citem {
		return &citem{id: newID(), kind: kind, state: state, who: who, when: when, body: body, threads: ths}
	}
	push := func(who, when string, forced bool, cs ...[2]string) *citem {
		return &citem{id: newID(), kind: "push", who: who, when: when, commits: cs, forced: forced}
	}
	items := []*citem{
		it("pr", "", "alice", "3d ago", description),
		// items[0] is the pull request itself: the description pane shows it.
		it("comment", "", "bob", "3d ago", "Nice start. Can we cover division by zero? I did not see a test for it and the helper looks like it would panic."),
		push("alice", "3d ago", false, [2]string{"a1b2c3d", "Add calc helper"}, [2]string{"b2c3d4e", "Add tests for Div"}, [2]string{"c3d4e5f", "Add package doc"}),
		it("comment", "", "alice", "2d ago", "Added a test for it, plus a package doc in doc.go."),
		it("comment", "", "dave", "2d ago", "The docs read well to me."),
		it("review", "approve", "bob", "2d ago", "LGTM once CI is green."),
		it("review", "changes", "carol", "1d ago", "The Div helper panics on zero. Please return an error instead; see the threads.",
			span(11, false, th(12, "+	return a / b", false,
				rp("carol", "1d ago", "Return an error instead of panicking?"),
				rp("alice", "1d ago", "Planning to, wanted to confirm the API first."))),
			span(0, true, th(30, "+func Mul(a, b int) int { return a * b }", true,
				rp("carol", "1d ago", "nit: Mul could use a doc example."),
				rp("alice", "20h ago", "Done, added one.")))),
		it("review", "comment", "dave", "20h ago", "A couple of questions.",
			th(20, "+func Add(a, b int) int { return a + b }", false,
				rp("dave", "20h ago", "Should Add guard against overflow?"))),
		it("sys", "", "ci", "1d ago", "ci: e2e failed on a1b2c3d"),
		it("sys", "", "alice", "1d ago", "Alice Chen requested a review from Carol Diaz"),
		it("sys", "", "alice", "1d ago", "Alice Chen added the label bug"),
		it("sys", "", "alice", "1d ago", "Alice Chen removed the label wip"),
		push("alice", "5h ago", true, [2]string{"d4e5f6a", "Return an error from Div"}, [2]string{"f9e8d7c", "Fix e2e flake by waiting for the fixture server before the first request"}),
		it("sys", "", "ci", "5h ago", "ci: unit tests failed on f9e8d7c"),
		it("sys", "", "alice", "5h ago", "Alice Chen requested a review from Carol Diaz"),
		it("comment", "", "alice", "5h ago", "Pushed a fix for the panic, waiting on carol."),
		it("comment", "", "ghost-bot", "1h ago", "Coverage is 91% (+2%) with this change."),
		it("comment", "", "dependabot", "18h ago", "Bumps golang.org/x/net from 0.41.0 to 0.43.0.\n"+
			"<details>\n<summary>Release notes</summary>\n\n"+
			"- `http2`: fix a stream leak on early server close\n- `html`: parse `<template>` content like browsers do\n- `proxy`: honour `NO_PROXY` with CIDR ranges\n- dependency updates\n"+
			"</details>\n"+
			"<details>\n<summary>Changelog</summary>\n\nThe tags between the two versions:\n\n"+
			"<details>\n<summary>0.42.0</summary>\n\n- `http2`: add `Transport.StrictMaxConcurrentStreams` docs\n- `idna`: update to Unicode 16\n```go\nclient := &http.Client{Transport: &http2.Transport{}}\n```\n</details>\n"+
			"- `0.43.0`: security fixes only\n"+
			"</details>\n"+
			"<details>\n<summary>Commits</summary>\n\n"+
			"- `3c4a1f2` http2: fix a stream leak on early close\n- `9be07d1` html: parse template content\n- `51aa4e8` proxy: honour NO_PROXY CIDR ranges\n- `e27c9b0` idna: update to Unicode 16\n- `0f6d3aa` all: update dependencies\n"+
			"</details>\n\n"+
			"Dependabot will resolve any conflicts with this PR as long as you do not alter it yourself. The compatibility score is 98% based on CI results of other repositories."),
	}
	open := map[string]bool{}
	for _, x := range items {
		if x.kind == "review" {
			un := slices.ContainsFunc(x.threads, func(t *thread) bool { return !t.resolved })
			open[itKey(x)] = x.state == "changes" || (x.state == "comment" && un)
		}
		for _, t := range x.threads {
			open[thKey(t)] = !t.resolved
		}
	}
	return convState{items: items, open: open, title: "Add calc helper"}
}

type crow struct {
	key, kind, parent string
	depth             int
	fold, open        bool
	it                *citem
	th                *thread
	rp                *reply
	rl                *[]*reply
	run               []*citem
	commit            [2]string // a commit row of an open push: short SHA and subject
	sg                *seg      // a "seg" row: a details block or text that follows one in a body
	lvl               int       // nesting of a seg row inside details blocks
}

// seg is a part of a body: text, or a <details> block with its summary, its
// lead text and the parts that follow the lead.
type seg struct {
	det     bool
	summary string
	text    string
	rest    []seg
}

var (
	sumRe    = regexp.MustCompile(`(?is)<summary>(.*?)</summary>`)
	detOpen  = regexp.MustCompile(`(?i)^\s*<details[^>]*>`)
	bots     = map[string]bool{"dependabot": true, "ghost-bot": true}
	hasBlock = func(s string) bool { return strings.Contains(s, "<details") }
)

// parseBody splits a markdown body into text and details blocks; blocks nest.
func parseBody(body string) []seg {
	var out []seg
	var txt []string
	flush := func() {
		if t := strings.TrimSpace(strings.Join(txt, "\n")); t != "" {
			out = append(out, seg{text: t})
		}
		txt = nil
	}
	lines := strings.Split(expandTabs(body), "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(lines[i])), "<details") {
			txt = append(txt, lines[i])
			continue
		}
		depth, j := 1, i+1
		for ; j < len(lines); j++ {
			lt := strings.ToLower(strings.TrimSpace(lines[j]))
			if strings.HasPrefix(lt, "<details") {
				depth++
			}
			if strings.HasPrefix(lt, "</details") {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		inner := strings.Join(lines[i:min(j, len(lines))], "\n")
		inner = detOpen.ReplaceAllString(inner, "")
		s := seg{det: true, summary: "Details"}
		if mt := sumRe.FindStringSubmatch(inner); mt != nil {
			s.summary = strings.TrimSpace(mt[1])
			inner = strings.Replace(inner, mt[0], "", 1)
		}
		cs := parseBody(inner)
		if len(cs) > 0 && !cs[0].det {
			s.text, cs = cs[0].text, cs[1:]
		}
		s.rest = cs
		flush()
		out = append(out, s)
		i = j
	}
	flush()
	return out
}

// leadText is the part of a body before its first details block.
func leadText(body string) string {
	if !hasBlock(body) {
		return body
	}
	if s := parseBody(body); len(s) > 0 && !s[0].det {
		return s[0].text
	}
	return ""
}

// whoHead is an author's name, with a dim tag for a bot, and a review's icon.
func whoHead(it *citem) string {
	h := bold(disp(it.who))
	if bots[it.who] {
		h += dim(" bot")
	}
	if it.kind == "review" {
		switch it.state {
		case "approve":
			h = ic.approved.String() + " " + h
		case "changes":
			h = ic.changes.String() + " " + h
		default:
			h = ic.commented.String() + " " + h
		}
	}
	return h
}

// segLines renders a seg row at its owner's hanging indent: a details block
// is its summary and a dim fold marker, with its lead text under a dim rule
// when open.
func segLines(r crow, cw int, gb string, kids bool) []string {
	start := lipgloss.Width(gb) + lipgloss.Width(whoHead(r.it)) + 2
	mark := " "
	if kids {
		mark = dim("│")
	}
	pad := gb + mark + strings.Repeat(" ", max(0, start-lipgloss.Width(gb)-1)) + strings.Repeat(dim("▏ "), r.lvl)
	w := max(10, cw-start-2*r.lvl)
	var out []string
	s := r.sg
	if !s.det {
		for _, l := range mdPlain(s.text, w) {
			out = append(out, pad+l)
		}
		return out
	}
	if !r.open {
		return []string{pad + s.summary + dim(" ⋯")}
	}
	out = append(out, pad+s.summary)
	if s.text != "" {
		for _, l := range mdPlain(s.text, w-2) {
			out = append(out, pad+dim("▏ ")+l)
		}
	}
	return out
}

// isEvent reports an entry of no human: a push or another event.
func (it *citem) isEvent() bool { return it.kind == "push" || it.kind == "sys" }

// openThread opens the diff pager on the file of a review thread, with the
// cursor on its line; the thread shows inline there.
func (m *model) openThread(t *thread) tea.Cmd {
	m.push(overlay{kind: "file", arg: t.fi, cur: m.rowOf(t.fi)})
	o := &m.stack[len(m.stack)-1]
	doc, vis, _ := m.dview(o)
	for i, d := range vis {
		if t.loc >= 0 && doc[d].fi == t.fi && doc[d].loc == t.loc {
			o.vcur = i
			break
		}
	}
	m.fixDiff(o)
	return nil
}

// inThreadRange reports a diff line that a review thread comments on.
func (m *model) inThreadRange(x dl) bool {
	if x.kind != '+' && x.kind != 'h' && x.kind != 'f' {
		return false
	}
	for _, it := range m.cv.items {
		for _, t := range it.threads {
			if t.fi != x.fi || t.loc < 0 || t.outdated {
				continue
			}
			lo := t.line
			if t.start > 0 {
				lo = t.start
			}
			if x.src >= lo && x.src <= t.line {
				return true
			}
		}
	}
	return false
}

func (c *convState) thShown(t *thread) bool {
	switch c.filter {
	case 1:
		return !t.resolved
	case 3:
		return slices.ContainsFunc(t.replies, func(r *reply) bool { return r.who == me })
	}
	return true
}

func (c *convState) itShown(it *citem) bool {
	if c.hideSys && it.isEvent() {
		return false
	}
	switch c.filter {
	case 1:
		return slices.ContainsFunc(it.threads, c.thShown)
	case 2:
		return it.state == "changes"
	case 3:
		return it.who == me || slices.ContainsFunc(it.threads, c.thShown)
	}
	return true
}

// counts returns the entries shown by the filter and all of them (not the PR).
func (c *convState) counts() (int, int) {
	n := 0
	for _, it := range c.items[1:] {
		if c.itShown(it) {
			n++
		}
	}
	return n, len(c.items) - 1
}

// crows lists the visible rows; all ignores the folds.
func (m *model) crows(all bool) []crow {
	c := &m.cv
	op := func(k string, def bool) bool { return all || c.isOpen(k, def) }
	var rows []crow
	// skip leaves out the first comment of a thread: its row is the thread's.
	addReplies := func(rl *[]*reply, depth int, parent string, th *thread, skip int) {
		for i, r := range *rl {
			if i < skip {
				continue
			}
			rows = append(rows, crow{key: fmt.Sprintf("rp:%d", r.id), kind: "reply", depth: depth, parent: parent, th: th, rp: r, rl: rl})
		}
	}
	var addSegs func(it *citem, segs []seg, lvl, depth int, parent, kp string)
	addSegs = func(it *citem, segs []seg, lvl, depth int, parent, kp string) {
		for i := range segs {
			s := &segs[i]
			k := fmt.Sprintf("%s.%d", kp, i)
			o := s.det && op(k, false)
			rows = append(rows, crow{key: k, kind: "seg", depth: depth, parent: parent, fold: s.det, open: o, it: it, sg: s, lvl: lvl})
			if o {
				addSegs(it, s.rest, lvl+1, depth, k, k)
			}
		}
	}
	addItem := func(it *citem, depth int, parent string) {
		key := itKey(it)
		fold := it.kind == "review" || it.kind == "push"
		open := fold && op(key, it.kind != "push")
		rows = append(rows, crow{key: key, kind: it.kind, depth: depth, parent: parent, fold: fold, open: open, it: it})
		if hasBlock(it.body) && (it.kind == "comment" || open) {
			segs := parseBody(it.body)
			if len(segs) > 0 && !segs[0].det {
				segs = segs[1:]
			}
			addSegs(it, segs, 0, depth+1, key, fmt.Sprintf("dt:%d", it.id))
		}
		if !open {
			return
		}
		if it.kind == "push" {
			for n, cm := range it.commits {
				rows = append(rows, crow{key: fmt.Sprintf("cm:%d:%d", it.id, n), kind: "commit", depth: depth + 1, parent: key, it: it, commit: cm})
			}
		}
		for _, t := range it.threads {
			if !c.thShown(t) {
				continue
			}
			tk := thKey(t)
			to := op(tk, !t.resolved)
			rows = append(rows, crow{key: tk, kind: "thread", depth: depth + 1, parent: key, fold: true, open: to, it: it, th: t})
			if to {
				addReplies(&t.replies, depth+2, tk, t, 1)
			}
		}
	}
	var vis []*citem
	for _, it := range c.items[1:] {
		if c.itShown(it) {
			vis = append(vis, it)
		}
	}
	for i := 0; i < len(vis); {
		j := i
		for j < len(vis) && vis[j].kind == "comment" {
			j++
		}
		if j-i >= 2 {
			rk := fmt.Sprintf("run:%d", vis[i].id)
			ro := op(rk, false)
			rows = append(rows, crow{key: rk, kind: "run", fold: true, open: ro, run: vis[i:j]})
			if ro {
				for _, it := range vis[i:j] {
					addItem(it, 1, rk)
				}
			}
			i = j
			continue
		}
		addItem(vis[i], 0, "")
		i++
	}
	return rows
}

func (m *model) selIdx(rows []crow) int {
	for i, r := range rows {
		if r.key == m.cv.sel {
			return i
		}
	}
	return max(0, min(m.cv.last, len(rows)-1))
}

func (m *model) setSel(rows []crow, i int) {
	i = max(0, min(i, len(rows)-1))
	m.cv.sel, m.cv.last = rows[i].key, i
}

// guides returns the dim tree guides of row i: first for its own line, then
// for the lines of its body. Column 0 stays free for the selection marker.
func guides(rows []crow, i int) (first, body string, kids bool) {
	d := rows[i].depth
	seg := rows[i].kind == "seg"
	if seg {
		d--
	}
	later := func(k int) bool {
		for _, x := range rows[i+1:] {
			if x.depth < k {
				return false
			}
			if x.depth == k {
				return true
			}
		}
		return false
	}
	pre := ""
	for k := 1; k < d; k++ {
		if later(k) {
			pre += dim("│  ")
		} else {
			pre += "   "
		}
	}
	first, body = " "+pre, " "+pre
	if d > 0 {
		if later(d) {
			first += dim("├─ ")
			body += dim("│  ")
		} else {
			first += dim("╰─ ")
			body += "   "
		}
	}
	// The guide under a body runs on only to real children, not its seg rows.
	j := i + 1
	for j < len(rows) && rows[j].kind == "seg" {
		j++
	}
	kids = j < len(rows) && rows[j].depth > d
	if seg {
		return body, body, kids
	}
	return first, body, kids
}

func (m *model) crowLines(r crow, cw int, gm, gb string, kids bool) []string {
	closed := r.fold && !r.open
	// A folded node's dim summary: "⋯ 3" for folded replies.
	fold := func(n int) string {
		if !closed || n == 0 {
			return ""
		}
		return fmt.Sprintf("⋯ %d", n)
	}
	// meta joins the secondary info of the right edge, dim.
	meta := func(pending, edited bool, parts ...string) string {
		var p []string
		if edited {
			p = append(p, "edited")
		}
		for _, s := range parts {
			if s != "" {
				p = append(p, s)
			}
		}
		s := dim(strings.Join(p, "  "))
		if pending {
			s = yellow("pending") + "  " + s
		}
		return s
	}
	// head is the row up to where the message starts; msg is the text that
	// follows it on the same line and flows under itself, hanging, below.
	var head, right, msg string
	var snip *thread
	switch r.kind {
	case "push", "sys":
		return sysLines(r.it.sysText(), cw)
	case "commit":
		return []string{commitLine(r.it.sysText(), r.commit, cw, gm)}
	case "run":
		var who []string
		for _, it := range r.run {
			if n := disp(it.who); !slices.Contains(who, n) {
				who = append(who, n)
			}
		}
		lead := ""
		if closed {
			lead = dim("⋯ ")
		}
		head = lead + bold(fmt.Sprintf("%d comments", len(r.run))) + dim(" · "+strings.Join(who, ", "))
		right = meta(false, false, r.run[0].when+" – "+r.run[len(r.run)-1].when)
	case "comment":
		head = whoHead(r.it)
		right = meta(false, r.it.edited, r.it.when)
		msg = leadText(r.it.body)
	case "seg":
		return segLines(r, cw, gb, kids)
	case "review":
		head = whoHead(r.it)
		sum := ""
		if closed {
			if n := len(r.it.threads); n > 0 {
				sum = fmt.Sprintf("%d threads ⋯", n)
			} else {
				n := 0
				sum = fold(n)
			}
		}
		right = meta(false, r.it.edited, sum, r.it.when)
		if r.open {
			msg = leadText(r.it.body)
		}
	case "thread":
		// The thread is its first comment's author; the file shows only in the
		// snippet block, and in the summary of a folded thread.
		first := r.th.replies[0]
		who := disp(first.who)
		if closed {
			head = threadIcon(r.th.resolved) + " " + bold(who) + dim(" · "+r.th.label())
			if r.th.resolved {
				head = threadIcon(true) + " " + dim(who+" · "+r.th.label())
			}
		} else {
			// The root row is the author and the first line of the comment;
			// the snippet block follows, then the rest of the body.
			if r.th.resolved {
				head = threadIcon(true) + " " + dim(who)
			} else {
				head = threadIcon(false) + " " + bold(who)
			}
			if r.th.outdated {
				head += " " + dim(outdatedIcon)
			}
			msg, snip = first.body, r.th
		}
		if closed && r.th.outdated {
			head += " " + dim(outdatedIcon)
		}
		right = meta(false, first.edited, fold(len(r.th.replies)-1), first.when)
	case "reply":
		head = bold(disp(r.rp.who))
		right = meta(r.rp.pending, r.rp.edited, r.rp.when)
		msg = r.rp.body
	}
	// The text starts one gap after the head; later lines align under it.
	var first string
	var rest []string
	var block []string
	start := lipgloss.Width(gm+head) + 2
	if msg != "" {
		w1 := max(10, cw-start-lipgloss.Width(right)-2)
		first = wrap(msg, w1)[0]
		head += "  " + first
		tail := strings.TrimPrefix(msg, first)
		if strings.HasPrefix(tail, "\n") {
			tail = tail[1:]
		} else {
			tail = strings.TrimLeft(tail, " ")
		}
		if tail != "" {
			rest = wrap(tail, max(10, cw-start))
		}
	} else {
		snip = nil
	}
	if snip != nil {
		for _, l := range threadBlock(snip, cw-start-2) {
			if l != "" {
				l = dim("▏ ") + l
			}
			block = append(block, l)
		}
		if len(rest) == 0 && len(block) > 0 && block[len(block)-1] == "" {
			block = block[:len(block)-1]
		}
	}
	out := []string{rightAlign(gm+head, right, cw)}
	guide := gb + " "
	if kids {
		guide = gb + dim("│")
	}
	guide += strings.Repeat(" ", max(0, start-lipgloss.Width(guide)))
	for _, l := range append(block, rest...) {
		out = append(out, guide+l)
	}
	return out
}

func (m *model) convLines(cw int) []string {
	c := &m.cv
	rows := m.crows(false)
	if len(rows) == 0 {
		c.first, c.lastLine = 0, 0
		return []string{dim("  nothing matches this filter (f changes it)")}
	}
	si := m.selIdx(rows)
	c.sel, c.last = rows[si].key, si
	var out []string
	for i, r := range rows {
		gm, gb, kids := guides(rows, i)
		ls := m.crowLines(r, cw, gm, gb, kids)
		if i == si {
			c.first, c.lastLine = len(out), len(out)+len(ls)-1
			for j, l := range ls {
				ls[j] = selRow(l, cw, m.focus == pMain)
			}
		}
		out = append(out, ls...)
		if r.depth == 0 && i+1 < len(rows) && rows[i+1].depth == 0 {
			out = append(out, "")
		}
	}
	return out
}

// fixConv keeps the selected row on screen.
func (m *model) fixConv() {
	w, h := m.paneDims(pMain)
	m.convLines(w - 4)
	ih := max(1, h-2)
	c := &m.cv
	if c.first < m.scroll {
		m.scroll = c.first
	}
	if c.lastLine >= m.scroll+ih {
		m.scroll = min(c.first, c.lastLine-ih+1)
	}
	m.scroll = max(0, min(m.scroll, m.maxScroll()))
}

// ensureSel moves the selection to its nearest visible ancestor.
func (m *model) ensureSel() {
	all := m.crows(true)
	parent := map[string]string{}
	for _, r := range all {
		parent[r.key] = r.parent
	}
	vis := map[string]bool{}
	for _, r := range m.crows(false) {
		vis[r.key] = true
	}
	k := m.cv.sel
	for k != "" && !vis[k] {
		k = parent[k]
	}
	if k != "" {
		m.cv.sel = k
	}
}

func (m *model) nextUnresolved(dir int) tea.Cmd {
	all := m.crows(true)
	cur := m.selIdx(all)
	for j := cur + dir; j >= 0 && j < len(all); j += dir {
		r := all[j]
		if r.kind != "thread" || r.th.resolved {
			continue
		}
		parent := map[string]string{}
		for _, x := range all {
			parent[x.key] = x.parent
		}
		for k := r.parent; k != ""; k = parent[k] {
			m.cv.open[k] = true
		}
		m.cv.sel = r.key
		return nil
	}
	return m.say("no more unresolved threads")
}

func rowTgt(r crow) tgt {
	t := tgt{kind: r.kind, it: r.it, th: r.th, rp: r.rp, rl: r.rl, openKey: r.key}
	if r.kind == "seg" {
		t.kind, t.openKey = r.it.kind, itKey(r.it)
	}
	if r.kind == "reply" {
		t.openKey = r.parent
	}
	return t
}

// replyVerb names what c does on the row under the cursor.
func (m *model) replyVerb() string {
	rows := m.crows(false)
	if len(rows) == 0 {
		return "reply"
	}
	switch rows[m.selIdx(rows)].kind {
	case "reply", "thread":
		return "reply"
	case "pr", "comment", "review", "seg":
		return "quote reply"
	}
	return "reply"
}

func (m *model) convKey(k string) tea.Cmd {
	c := &m.cv
	rows := m.crows(false)
	switch k {
	case "f":
		c.filter = (c.filter + 1) % len(filterNames)
		m.ensureSel()
		return nil
	case "s":
		c.hideSys = !c.hideSys
		m.ensureSel()
		return nil
	}
	if len(rows) == 0 {
		return nil
	}
	i := m.selIdx(rows)
	r := rows[i]
	switch k {
	case "l", "right":
		if r.fold && !r.open {
			c.open[r.key] = true
		} else if r.fold && i+1 < len(rows) && (rows[i+1].depth > r.depth || rows[i+1].parent == r.key) {
			m.setSel(rows, i+1)
		}
	case "h", "left":
		if r.fold && r.open {
			c.open[r.key] = false
		} else if r.parent != "" {
			for j := i - 1; j >= 0; j-- {
				if rows[j].key == r.parent {
					m.setSel(rows, j)
					break
				}
			}
		}
	case "enter", "o":
		if r.kind == "thread" {
			return m.openThread(r.th)
		}
		if r.kind == "commit" {
			m.push(overlay{kind: "commit", title: r.commit[0], text: r.commit[1]})
			return nil
		}
		if k == "enter" && r.fold {
			c.open[r.key] = !r.open
		}
	case "*":
		anyClosed := slices.ContainsFunc(rows, func(x crow) bool { return x.fold && !x.open })
		for _, x := range m.crows(true) {
			if x.fold {
				c.open[x.key] = anyClosed
			}
		}
	case ")":
		return m.nextUnresolved(1)
	case "(":
		return m.nextUnresolved(-1)
	case "c", "T", "e", "X":
		return m.act(k, rowTgt(r))
	}
	m.ensureSel()
	return nil
}

// ---------- actions shared by the tree and the diff pager ----------

// quote opens the composer for a new top-level comment quoting it: GitHub
// has no reply to an issue comment or a review body.
func (m *model) quote(it *citem) tea.Cmd {
	var q strings.Builder
	for _, l := range strings.Split(strings.TrimSpace(it.body), "\n") {
		q.WriteString("> " + l + "\n")
	}
	q.WriteString("\n")
	m.push(overlay{kind: "reply", title: "Quote reply to " + disp(it.who), ctext: q.String(), save: func(m *model, _, txt string) tea.Cmd {
		m.cv.items = append(m.cv.items, &citem{id: newID(), kind: "comment", who: me, when: "now", body: txt})
		return m.say("comment added (pending)")
	}})
	return nil
}

type tgt struct {
	kind, openKey string
	it            *citem
	th            *thread
	rp            *reply
	rl            *[]*reply
}

func (m *model) act(k string, t tgt) tea.Cmd {
	own := (t.kind == "comment" && t.it.who == me) || (t.kind == "reply" && t.rp.who == me)
	switch k {
	case "enter":
		if t.kind == "thread" && t.th != nil {
			key := thKey(t.th)
			m.cv.open[key] = !m.cv.isOpen(key, !t.th.resolved)
		}
	case "c":
		var rl *[]*reply
		who := ""
		switch t.kind {
		case "run":
			return m.say("open the run and pick a comment to reply to")
		case "push", "sys":
			return m.say("events take no reply")
		case "reply":
			rl, who = &t.th.replies, disp(t.rp.who)+" in thread "+t.th.label()
		case "thread":
			rl, who = &t.th.replies, "thread "+t.th.label()
		default:
			return m.quote(t.it)
		}
		ok := thKey(t.th)
		m.push(overlay{kind: "reply", title: "Reply to " + who, save: func(m *model, _, txt string) tea.Cmd {
			*rl = append(*rl, &reply{id: newID(), who: me, when: "now", body: txt, pending: true})
			m.cv.open[ok] = true
			return m.say("reply added (pending)")
		}})
	case "T":
		th := t.th
		if th == nil {
			return m.say("select a thread (or a reply in it) to resolve")
		}
		verb, done := "Resolve", "thread resolved"
		if th.resolved {
			verb, done = "Reopen", "thread reopened"
		}
		m.confirmDo(fmt.Sprintf("%s thread %s?", verb, th.label()), done, func(m *model) tea.Cmd {
			th.resolved = !th.resolved
			m.cv.open[thKey(th)] = !th.resolved
			if o := m.diffTop(); o != nil && o.ondraft && o.dkey[0] == -th.id {
				o.dkey[1] = 0
			}
			return nil
		})
	case "e":
		switch {
		case own && t.kind == "comment":
			it := t.it
			m.push(overlay{kind: "reply", title: "Edit your comment", ctext: it.body,
				save: func(m *model, _, txt string) tea.Cmd {
					it.body, it.edited = txt, true
					return m.say("comment edited")
				}})
		case own:
			rp := t.rp
			m.push(overlay{kind: "reply", title: "Edit your reply", ctext: rp.body,
				save: func(m *model, _, txt string) tea.Cmd {
					rp.body, rp.edited = txt, true
					return m.say("reply edited")
				}})
		default:
			return m.say("you can only edit your own comments")
		}
	case "X", "D":
		switch {
		case !own:
			return m.say("you can only edit your own comments")
		case t.kind == "comment":
			it := t.it
			m.confirmDo("Delete your comment?", "comment deleted", func(m *model) tea.Cmd {
				m.cv.items = slices.DeleteFunc(m.cv.items, func(x *citem) bool { return x == it })
				return nil
			})
		default:
			rp, rl := t.rp, t.rl
			m.confirmDo("Delete your reply?", "reply deleted", func(m *model) tea.Cmd {
				*rl = slices.DeleteFunc(*rl, func(x *reply) bool { return x == rp })
				if o := m.diffTop(); o != nil {
					o.ondraft = false
				}
				return nil
			})
		}
	}
	return nil
}

// editPR opens the title and description editor of the pull request.
func (m *model) editPR() {
	it := m.cv.items[0]
	m.push(overlay{kind: "editpr", title: "Edit pull request #42", etitle: m.cv.title, ctext: it.body,
		save: func(m *model, title, body string) tea.Cmd {
			m.cv.title, it.body, it.edited = title, body, true
			return m.say("pull request updated")
		}})
}

// tgtOfKey is the thread or reply a pager draft key stands for.
func (m *model) tgtOfKey(dk [3]int) (tgt, bool) {
	if dk[0] >= 0 {
		return tgt{}, false
	}
	for _, it := range m.cv.items {
		for _, t := range it.threads {
			if t.id != -dk[0] {
				continue
			}
			g := tgt{kind: "thread", th: t, openKey: thKey(t)}
			if dk[1] > 0 && dk[1] <= len(t.replies) {
				g.kind, g.rp, g.rl = "reply", t.replies[dk[1]-1], &t.replies
			}
			return g, true
		}
	}
	return tgt{}, false
}

// threadRows are the rows of the review threads anchored under a pager line.
func (m *model) threadRows(fi, loc, li int) []vrow {
	var out []vrow
	for _, it := range m.cv.items {
		for _, t := range it.threads {
			if t.fi != fi || t.loc != loc {
				continue
			}
			first := t.replies[0]
			isOpen := m.cv.isOpen(thKey(t), !t.resolved)
			var text string
			switch {
			case !isOpen:
				text = bold(disp(first.who)) + dim(" · "+t.label())
				if n := len(t.replies) - 1; n > 0 {
					text += dim(fmt.Sprintf(" ⋯ %d", n))
				}
			case t.resolved:
				text = dim(disp(first.who))
			default:
				text = bold(disp(first.who))
			}
			if t.outdated {
				text += " " + dim(outdatedIcon)
			}
			out = append(out, vrow{li: li, draft: true, dk: [3]int{-t.id, 0, 0},
				text: yellow("   ┌ ") + rev(" thread ") + " " + threadIcon(t.resolved) + " " + text})
			if !isOpen {
				continue
			}
			// The header carries no excerpt, so the first comment's body
			// is drawn once, on its own row.
			body := first.body
			if t.resolved {
				body = dim(body)
			}
			out = append(out, vrow{li: li, draft: true, dk: [3]int{-t.id, 1, 0},
				text: dim("   │ ") + body})
			for i, r := range t.replies {
				if i == 0 {
					continue
				}
				gl := "├─"
				if i == len(t.replies)-1 {
					gl = "╰─"
				}
				tg := ""
				if r.pending {
					tg = yellow(" (pending)")
				}
				if r.edited {
					tg += dim(" (edited)")
				}
				out = append(out, vrow{li: li, draft: true, dk: [3]int{-t.id, i + 1, 0},
					text: dim("   "+gl+" ") + bold(disp(r.who)) + dim(": ") + r.body + tg})
			}
		}
	}
	return out
}

// ---------- composer and editor overlays ----------

func (m *model) composeKey(o *overlay, msg tea.KeyPressMsg) tea.Cmd {
	k := msg.String()
	multi := o.kind == "editpr"
	field := &o.ctext
	if multi && o.efield == 0 {
		field = &o.etitle
	}
	switch {
	case k == "esc":
		m.pop()
	case (multi && k == "ctrl+s") || (!multi && k == "enter"):
		if strings.TrimSpace(o.ctext) == "" || (multi && strings.TrimSpace(o.etitle) == "") {
			return m.say("nothing to save: the text is empty")
		}
		save, t, b := o.save, o.etitle, o.ctext
		m.pop()
		return save(m, t, b)
	case multi && k == "tab":
		o.efield = 1 - o.efield
	case k == "enter":
		if o.efield == 0 {
			o.efield = 1
		} else {
			o.ctext += "\n"
		}
	case k == "backspace":
		if r := []rune(*field); len(r) > 0 {
			*field = string(r[:len(r)-1])
		}
	default:
		*field += msg.Text
	}
	return nil
}

func (m *model) drawCompose(o *overlay) (string, int, int, int) {
	x, y, w, h := m.ovRect(o)
	cw := min(76, w-4)
	if o.kind == "reply" {
		ch := 9
		ls := wrap(o.ctext+"▏", cw-4)
		if slots := ch - 4; len(ls) > slots {
			ls = ls[len(ls)-slots:]
		}
		for len(ls) < ch-4 {
			ls = append(ls, "")
		}
		ls = append(ls, dim("enter save · esc cancel"))
		return box(o.title, ls, cw, ch, true), x + (w-cw)/2, y + (h-ch)/2, cw
	}
	ch := min(h, 16)
	tl := dim("title ") + o.etitle
	if o.efield == 0 {
		tl += "▏"
	}
	txt := o.ctext
	if o.efield == 1 {
		txt += "▏"
	}
	var bl []string
	for _, l := range strings.Split(txt, "\n") {
		bl = append(bl, wrap(l, cw-4)...)
	}
	slots := ch - 5
	if len(bl) > slots {
		if o.efield == 1 {
			bl = bl[len(bl)-slots:]
		} else {
			bl = bl[:slots]
		}
	}
	for len(bl) < slots {
		bl = append(bl, "")
	}
	ls := append([]string{tl, dim(strings.Repeat("─", cw-4))}, bl...)
	ls = append(ls, dim("tab title/body · enter newline (body) · ctrl+s save · esc cancel"))
	return box(o.title, ls, cw, ch, true), x + (w-cw)/2, y + (h-ch)/2, cw
}
