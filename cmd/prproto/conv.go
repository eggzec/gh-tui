package main

// The conversation pane as a tree with a cursor: the PR, comments and reviews
// (level 1), a review's threads (level 2) and their replies (level 3). The
// threads are shared with the diff pager, which shows them under their line.

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
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
	replies                      []*reply
}

type convState struct {
	items    []*citem
	open     map[string]bool
	sel      string
	last     int
	filter   int
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
	items := []*citem{
		it("pr", "", "alice", "3d ago", description),
		it("comment", "", "bob", "3d ago", "Nice start. Can we cover division by zero? I did not see a test for it and the helper looks like it would panic."),
		it("commit", "", "alice", "3d ago", "Add calc helper (a1b2c3d)"),
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
		it("event", "", "ci", "1d ago", "e2e failed on a1b2c3d"),
		it("comment", "", "alice", "5h ago", "Pushed a fix for the panic, waiting on carol."),
		it("comment", "", "coverage-bot", "1h ago", "Coverage is 91% (+2%) with this change."),
	}
	open := map[string]bool{"pr": true}
	for _, x := range items {
		if x.kind == "review" {
			un := slices.ContainsFunc(x.threads, func(t *thread) bool { return !t.resolved })
			open[itKey(x)] = x.state == "changes" || (x.state == "comment" && un)
		}
		for _, t := range x.threads {
			open[thKey(t)] = !t.resolved
		}
	}
	return convState{items: items, open: open, title: "Add calc helper", sel: "pr"}
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
	switch c.filter {
	case 1:
		return slices.ContainsFunc(it.threads, c.thShown)
	case 2:
		return it.state == "changes"
	case 3:
		return it.who == me || slices.ContainsFunc(it.replies, func(r *reply) bool { return r.who == me }) ||
			slices.ContainsFunc(it.threads, c.thShown)
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
	addItem := func(it *citem, depth int, parent string) {
		key := itKey(it)
		if it.kind == "pr" {
			key = "pr"
		}
		fold := it.kind == "review" || it.kind == "pr" || len(it.replies) > 0
		open := fold && op(key, true)
		rows = append(rows, crow{key: key, kind: it.kind, depth: depth, parent: parent, fold: fold, open: open, it: it})
		if !open {
			return
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
		addReplies(&it.replies, depth+1, key, nil, 0)
	}
	addItem(c.items[0], 0, "")
	var vis []*citem
	for _, it := range c.items[1:] {
		if c.itShown(it) {
			vis = append(vis, it)
		}
	}
	for i := 0; i < len(vis); {
		j := i
		for j < len(vis) && vis[j].kind == "comment" && len(vis[j].replies) == 0 {
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
	kids = i+1 < len(rows) && rows[i+1].depth > d
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
	bw := max(10, cw-3*(r.depth+1)-3)
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
	var head, right string
	var body []string
	switch r.kind {
	case "pr":
		head = bold("#42 " + m.cv.title)
		right = meta(false, r.it.edited, "alice · 3d")
		if r.open {
			body = wrap(r.it.body, bw)
		}
	case "run":
		var who []string
		for _, it := range r.run {
			if !slices.Contains(who, it.who) {
				who = append(who, it.who)
			}
		}
		lead := ""
		if closed {
			lead = dim("⋯ ")
		}
		head = lead + bold(fmt.Sprintf("%d comments", len(r.run))) + dim(" · "+strings.Join(who, ", "))
		right = meta(false, false, r.run[0].when+" – "+r.run[len(r.run)-1].when)
	case "comment":
		head = bold(r.it.who) + dim(" commented")
		right = meta(false, r.it.edited, fold(len(r.it.replies)), r.it.when)
		body = wrap(r.it.body, bw)
	case "commit":
		head = dim("◆ ") + bold(r.it.who) + dim(" pushed a commit")
		right = meta(false, false, r.it.when)
		for _, l := range wrap(r.it.body, bw) {
			body = append(body, cyan(l))
		}
	case "event":
		head = yellow("● ") + bold(r.it.who)
		right = meta(false, false, r.it.when)
		body = wrap(r.it.body, bw)
	case "review":
		switch r.it.state {
		case "approve":
			head = green("✓ ") + bold(r.it.who)
		case "changes":
			head = red("✗ ") + bold(r.it.who)
		default:
			head = dim("○ ") + bold(r.it.who)
		}
		sum := ""
		if closed {
			if n := len(r.it.threads); n > 0 {
				sum = fmt.Sprintf("%d threads ⋯", n)
			} else {
				n := len(r.it.replies)
				sum = fold(n)
			}
		}
		right = meta(false, r.it.edited, sum, r.it.when)
		if r.open {
			body = wrap(r.it.body, bw)
		}
	case "thread":
		// The thread is its first comment's author; the file shows only in the
		// snippet block, and in the summary of a folded thread.
		first := r.th.replies[0]
		who := "@" + first.who
		if closed {
			head = threadIcon(r.th.resolved) + " " + bold(who) + dim(" · "+r.th.label())
			if r.th.resolved {
				head = threadIcon(true) + " " + dim(who+" · "+r.th.label())
			}
		} else {
			line, _, _ := strings.Cut(first.body, "\n")
			if r.th.resolved {
				head = threadIcon(true) + " " + dim(who+"  "+line)
			} else {
				head = threadIcon(false) + " " + bold(who) + "  " + dim(line)
			}
		}
		if r.th.outdated {
			head += " " + dim(outdatedIcon)
		}
		right = meta(false, first.edited, fold(len(r.th.replies)-1), first.when)
		if r.open {
			for _, l := range threadBlock(r.th, cw-3*(r.depth+1)-3-2) {
				if l != "" {
					l = "  " + dim("▏ ") + l
				}
				body = append(body, l)
			}
			body = append(body, wrap(first.body, bw)...)
		}
	case "reply":
		head = bold(r.rp.who)
		right = meta(r.rp.pending, r.rp.edited, r.rp.when)
	}
	if r.kind == "reply" {
		body = wrap(r.rp.body, bw)
	}
	out := []string{rightAlign(gm+head, right, cw)}
	for _, l := range body {
		switch {
		case kids:
			l = gb + dim("│  ") + l
		default:
			l = gb + "   " + l
		}
		out = append(out, l)
	}
	return out
}

func (m *model) convLines(cw int) []string {
	c := &m.cv
	rows := m.crows(false)
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
	if r.kind == "reply" {
		t.openKey = r.parent
	}
	return t
}

func (m *model) convKey(k string) tea.Cmd {
	c := &m.cv
	rows := m.crows(false)
	i := m.selIdx(rows)
	r := rows[i]
	switch k {
	case "l", "right":
		if r.fold && !r.open {
			c.open[r.key] = true
		} else if r.fold && i+1 < len(rows) && rows[i+1].depth > r.depth {
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
	case "enter":
		if r.fold {
			c.open[r.key] = !r.open
		}
	case "*":
		anyClosed := slices.ContainsFunc(rows, func(x crow) bool { return x.fold && !x.open })
		for _, x := range m.crows(true) {
			if x.fold {
				c.open[x.key] = anyClosed
			}
		}
	case "f":
		c.filter = (c.filter + 1) % len(filterNames)
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
		case "reply":
			rl, who = t.rl, t.rp.who
		case "thread":
			rl, who = &t.th.replies, "thread "+t.th.label()
		default:
			rl, who = &t.it.replies, t.it.who
		}
		ok := t.openKey
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
		case t.kind == "pr":
			it := t.it
			m.push(overlay{kind: "editpr", title: "Edit pull request #42", etitle: m.cv.title, ctext: it.body,
				save: func(m *model, title, body string) tea.Cmd {
					m.cv.title, it.body, it.edited = title, body, true
					return m.say("pull request updated")
				}})
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
		case t.kind == "pr":
			return m.say("the pull request itself can't be deleted here")
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
				text = bold("@"+first.who) + dim(" · "+t.label())
				if n := len(t.replies) - 1; n > 0 {
					text += dim(fmt.Sprintf(" ⋯ %d", n))
				}
			case t.resolved:
				text = dim("@" + first.who + ": " + first.body)
			default:
				text = bold("@"+first.who) + dim(": ") + first.body
			}
			if t.outdated {
				text += " " + dim(outdatedIcon)
			}
			out = append(out, vrow{li: li, draft: true, dk: [3]int{-t.id, 0, 0},
				text: yellow("   ┌ ") + rev(" thread ") + " " + threadIcon(t.resolved) + " " + text})
			if !isOpen {
				continue
			}
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
					text: dim("   "+gl+" ") + bold(r.who) + dim(": ") + r.body + tg})
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
