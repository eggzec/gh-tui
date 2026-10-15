package main

import (
	"fmt"
	"slices"

	"github.com/charmbracelet/x/ansi"
)

// tnode is one row of a pane tree (Involved, Development). Its depth drives
// the same dim guides as the conversation.
type tnode struct {
	depth int
	kind  string // hdr, person, empty, issue, pr, blocker
	grp   string // author, rev, asg, com; closes, prs
	key   string // fold key of a header, login of a person
	idx   int    // index into m.reviewers or m.dev
	num   int    // issue or PR number
	text  string
}

const prAuthor = "alice"

// foldOpen tells whether a group is unfolded; def is its state until the user
// toggles it.
func (m *model) foldOpen(key string, def bool) bool {
	if m.defFolds {
		return def
	}
	if v, ok := m.fold[key]; ok {
		return v
	}
	return def
}

func (m *model) toggleFold(key string, def bool) {
	if m.fold == nil {
		m.fold = map[string]bool{}
	}
	m.fold[key] = !m.foldOpen(key, def)
}

// nodeMove is l and h in the Involved and Development trees: a header opens
// or closes like a fold, l on an open node steps to its first child, h on a
// closed node or a leaf goes to its parent.
func (m *model) nodeMove(right bool) {
	p := m.focus
	var nodes []tnode
	if p == pReviewers {
		nodes = m.involved()
	} else {
		nodes = m.devNodes()
	}
	cur := m.cur[p]
	n := nodes[cur]
	if n.kind == "hdr" {
		def := true
		if n.grp == "com" {
			def = len(m.commenters()) <= 3
		}
		open := m.foldOpen(n.key, def)
		if right != open {
			m.toggleFold(n.key, def)
			return
		}
	}
	if right {
		if cur+1 < len(nodes) && nodes[cur+1].depth > n.depth {
			m.cur[p] = cur + 1
		}
		return
	}
	for j := cur - 1; j >= 0; j-- {
		if nodes[j].depth < n.depth {
			m.cur[p] = j
			return
		}
	}
}

// mergeMove is l and h in the Merge tree, like nodeMove: a checks or workflow
// row opens or closes, l on an open row steps in, h goes to the parent.
func (m *model) mergeMove(right bool) {
	bs := m.blockers()
	cur := clampInt(m.cur[pMerge], 0, len(bs)-1)
	b := bs[cur]
	if b.kind == "checks" || b.kind == "wf" || b.kind == "app" {
		if right != m.foldOpen(b.key, true) {
			m.toggleFold(b.key, true)
			return
		}
	}
	if right {
		if cur+1 < len(bs) && bs[cur+1].depth > b.depth {
			m.cur[pMerge] = cur + 1
		}
		return
	}
	for j := cur - 1; j >= 0; j-- {
		if bs[j].depth < b.depth {
			m.cur[pMerge] = j
			return
		}
	}
}

func hdrText(open bool, name string, n int) string {
	a := "▾ "
	if !open {
		a = "▸ "
	}
	return dim(a) + name + dim(fmt.Sprintf(" %d", n))
}

// commenters lists who commented or reviewed but is not named elsewhere.
func (m *model) commenters() []string {
	seen := map[string]bool{prAuthor: true}
	for _, r := range m.reviewers {
		seen[r.name] = true
	}
	for _, a := range m.assignees {
		seen[a] = true
	}
	var out []string
	add := func(w string) {
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	for _, it := range m.cv.items[1:] {
		if it.kind != "comment" && it.kind != "review" {
			continue
		}
		add(it.who)
		for _, t := range it.threads {
			for _, r := range t.replies {
				add(r.who)
			}
		}
	}
	return out
}

func (m *model) involved() []tnode {
	out := []tnode{{kind: "person", grp: "author", key: prAuthor, text: dim("Author ") + disp(prAuthor)}}
	open := m.foldOpen("inv:rev", true)
	out = append(out, tnode{kind: "hdr", grp: "rev", key: "inv:rev", text: hdrText(open, "Reviewers", len(m.reviewers))})
	if open {
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
			out = append(out, tnode{depth: 1, kind: "person", grp: "rev", key: r.name, idx: k, text: icon + " " + dispS(r.name) + dim(" "+r.state)})
		}
		if len(m.reviewers) == 0 {
			out = append(out, tnode{depth: 1, kind: "empty", grp: "rev", text: dim("none (enter to request)")})
		}
	}
	open = m.foldOpen("inv:asg", true)
	out = append(out, tnode{kind: "hdr", grp: "asg", key: "inv:asg", text: hdrText(open, "Assignees", len(m.assignees))})
	if open {
		for _, a := range m.assignees {
			out = append(out, tnode{depth: 1, kind: "person", grp: "asg", key: a, text: disp(a)})
		}
		if len(m.assignees) == 0 {
			out = append(out, tnode{depth: 1, kind: "empty", grp: "asg", text: dim("none (enter to assign)")})
		}
	}
	com := m.commenters()
	open = m.foldOpen("inv:com", len(com) <= 3)
	out = append(out, tnode{kind: "hdr", grp: "com", key: "inv:com", text: hdrText(open, "Commenters", len(com))})
	if open {
		for _, c := range com {
			out = append(out, tnode{depth: 1, kind: "person", grp: "com", key: c, text: disp(c)})
		}
	}
	return out
}

func devGlyph(kind, state string) string {
	switch {
	case kind == "pr" && state == "closed":
		return ic.prClosed.String()
	case kind == "pr":
		return ic.prOpen.String()
	case state == "closed":
		return ic.issueClosed.String()
	}
	return ic.issueOpen.String()
}

func (m *model) devNodes() []tnode {
	var out []tnode
	var closes, prs []int
	for i, d := range m.dev {
		if d.kind == "pr" {
			prs = append(prs, i)
		} else {
			closes = append(closes, i)
		}
	}
	open := m.foldOpen("dev:closes", true)
	out = append(out, tnode{kind: "hdr", grp: "closes", key: "dev:closes", text: hdrText(open, "Closes", len(closes))})
	if open {
		for _, i := range closes {
			d := m.dev[i]
			out = append(out, tnode{depth: 1, kind: "issue", grp: "closes", idx: i, num: d.num, text: devGlyph("issue", d.state) + fmt.Sprintf(" #%d ", d.num) + d.title})
			for _, b := range d.by {
				out = append(out, tnode{depth: 2, kind: "blocker", grp: "closes", idx: i, num: b.num,
					text: dim("blocked by ") + devGlyph("issue", b.state) + dim(fmt.Sprintf(" #%d %s", b.num, b.title))})
			}
		}
		if len(closes) == 0 {
			out = append(out, tnode{depth: 1, kind: "empty", grp: "closes", text: dim("none (a to link)")})
		}
	}
	open = m.foldOpen("dev:prs", true)
	out = append(out, tnode{kind: "hdr", grp: "prs", key: "dev:prs", text: hdrText(open, "Linked PRs", len(prs))})
	if open {
		for _, i := range prs {
			d := m.dev[i]
			out = append(out, tnode{depth: 1, kind: "pr", grp: "prs", idx: i, num: d.num, text: devGlyph("pr", d.state) + fmt.Sprintf(" #%d ", d.num) + d.title})
		}
	}
	return out
}

// treeLines draws nodes with the conversation's guides; every node is one
// line, so the cursor line is the cursor index.
func (m *model) treeLines(pane, cw int, nodes []tnode) ([]string, int) {
	rows := make([]crow, len(nodes))
	for i, n := range nodes {
		rows[i].depth = n.depth
	}
	out := make([]string, len(nodes))
	for i, n := range nodes {
		g, _, _ := guides(rows, i)
		l := ansi.Truncate(g+n.text, cw, "…")
		if m.cur[pane] == i {
			l = selRow(l, cw, m.focus == pane)
		}
		out[i] = l
	}
	return out, m.cur[pane]
}

// showPerson focuses the Conversation on the latest comment or review of login.
func (m *model) showPerson(login string) string {
	var last *citem
	for _, it := range m.cv.items[1:] {
		if it.who == login && (it.kind == "comment" || it.kind == "review") {
			last = it
		}
	}
	if last == nil {
		return "no comments from " + disp(login)
	}
	m.focus = pMain
	m.cv.sel = itKey(last)
	rows := m.crows(false)
	j := slices.IndexFunc(rows, func(r crow) bool { return r.key == itKey(last) || slices.Contains(r.run, last) })
	if j < 0 {
		m.ensureSel()
		return ""
	}
	m.setSel(rows, j)
	return ""
}
