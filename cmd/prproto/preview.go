package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// A zoomed list pane (Merge, Involved, Development) is list | preview of the
// hovered row, like the zoomed Files tree | diff. The preview is read-only and
// follows the cursor; tab moves the focus into it to scroll.

// pview is what the preview shows for the row under the cursor.
type pview struct {
	key, title, info string
	lines            []string
	off              int // where it starts scrolled when the row changes
}

// previewBox draws the preview beside the zoomed list.
func (m *model) previewBox() string {
	w, h := m.w-listW, m.bodyH()
	pv := m.preview(w - 4)
	if pv.key != m.pvKey {
		m.pvKey, m.pvScroll = pv.key, pv.off
	}
	ih := max(1, h-2)
	m.pvScroll = clampInt(m.pvScroll, 0, max(0, len(pv.lines)-ih))
	info := pv.info
	if len(pv.lines) > ih {
		if info != "" {
			info += dim(" · ")
		}
		info += dim(fmt.Sprintf("%d%%", (m.pvScroll+ih)*100/len(pv.lines)))
	}
	end := min(len(pv.lines), m.pvScroll+ih)
	return boxR(pv.title, info, pv.lines[m.pvScroll:end], w, h, m.pfocus == 1)
}

func (m *model) preview(cw int) pview {
	var pv pview
	switch m.focus {
	case pMerge:
		bs := blockers()
		pv = m.mergePreview(bs[clampInt(m.cur[pMerge], 0, len(bs)-1)], cw)
	case pReviewers:
		ns := m.involved()
		pv = m.involvedPreview(ns[clampInt(m.cur[pReviewers], 0, len(ns)-1)], cw)
	case pDev:
		ns := m.devNodes()
		pv = m.devPreview(ns[clampInt(m.cur[pDev], 0, len(ns)-1)], cw)
	}
	pv.key = fmt.Sprintf("%d|%d|%s", cw, m.focus, pv.key)
	return pv
}

func rule(cw int) string { return dim(strings.Repeat("─", max(1, cw))) }

// convBlocks renders, with the Conversation's own row renderer, the rows that
// match and what hangs under them, each block flush left.
func (m *model) convBlocks(all bool, match func(crow) bool, cw int) []string {
	c := &m.cv
	f, hs := c.filter, c.hideSys
	c.filter, c.hideSys = 0, false
	rows := m.crows(all)
	c.filter, c.hideSys = f, hs
	var out []string
	for i := 0; i < len(rows); i++ {
		if !match(rows[i]) {
			continue
		}
		j := i + 1
		for j < len(rows) && rows[j].depth > rows[i].depth {
			j++
		}
		blk := slices.Clone(rows[i:j])
		for k := range blk {
			blk[k].depth -= rows[i].depth
		}
		for k, r := range blk {
			gm, gb, kids := guides(blk, k)
			out = append(out, m.crowLines(r, cw, gm, gb, kids)...)
		}
		out = append(out, "")
		i = j - 1
	}
	return out
}

// ---------- Merge ----------

func (m *model) mergePreview(b blocker, cw int) pview {
	switch b.kind {
	case "check":
		sha := headSHA()
		r := runOf(sha, b.text)
		wf, job, _ := strings.Cut(b.text, " / ")
		pv := pview{key: "check|" + b.text + "|" + r.state,
			title: fmt.Sprintf("%s · %s · run #%d", wf, job, runNumber(max(0, slices.Index(wfs(sha), wf)))),
			info:  dim(stateWord(r.state) + " · " + r.dur)}
		for i, l := range logLines(sha, r) {
			t := l.text
			switch {
			case l.err:
				t = bgRow(red("▌")+ansi.Strip(t), cw, errBG)
				if pv.off == 0 {
					pv.off = max(0, i-3)
				}
			case l.warn:
				t = "\x1b[33m!\x1b[39m" + t[1:]
			}
			pv.lines = append(pv.lines, t)
		}
		return pv
	case "changes":
		var it *citem
		for _, x := range m.cv.items[1:] {
			if x.kind == "review" && x.state == "changes" {
				it = x
				break
			}
		}
		pv := pview{key: "changes", title: "Changes requested", info: dim("as in the Conversation")}
		if it == nil {
			pv.lines = []string{dim("no review requests changes")}
			return pv
		}
		pv.title = disp(it.who) + " · changes requested"
		pv.info = dim(it.when)
		pv.lines = m.convBlocks(false, func(r crow) bool { return r.kind == "review" && r.it == it }, cw)
		return pv
	case "conflict":
		pv := pview{key: "conflict", title: "Conflicts with main"}
		pv.lines = append(wrap("This branch has conflicts with main. Resolve them on GitHub or locally.", cw), "",
			bold("enter")+" "+dim("would open the conflict info (prototype)"))
		return pv
	case "review":
		return ownersPreview(cw)
	case "behind":
		pv := pview{key: "behind", title: "Behind main", info: dim("3 commits")}
		pv.lines = append(pv.lines, wrap("main has 3 commits that feat/calc does not. GitHub can merge main into the branch (or rebase it) so the checks run on the combined code.", cw)...)
		pv.lines = append(pv.lines, "", dim("main has, since the branch started:"))
		for _, c := range [][2]string{{"7d41e0b", "Bump the Go toolchain to 1.25.1"}, {"2fa9c36", "Rename loadConfig to config.Load"}, {"90bb1d4", "Fix the flaky e2e fixture server"}} {
			pv.lines = append(pv.lines, "  "+cyan(c[0])+" "+c[1])
		}
		pv.lines = append(pv.lines, "", bold("enter")+" updates the branch with main "+dim("(asks y/n; U does the same)"))
		return pv
	}
	pv := pview{key: "other|" + b.kind, title: b.text}
	pv.lines = append(wrap("This stops the merge until it is fixed.", cw), "", bold("enter")+" "+dim("opens what can fix it"))
	if b.kind == "draft" {
		pv.lines = []string{"The pull request is a draft; drafts cannot be merged.", "", bold("enter") + " marks it ready for review " + dim("(asks y/n)")}
	}
	return pv
}

// ownersOf is the fake CODEOWNERS: the owners of a changed file.
func ownersOf(path string) []string {
	switch {
	case strings.HasPrefix(path, "pkg/calc/internal/"), strings.HasPrefix(path, "cmd/"):
		return []string{"org/core"}
	case strings.HasPrefix(path, "pkg/"):
		return []string{"org/core", "bob"}
	}
	return []string{"dave"}
}

var coreMembers = []string{"carol", "erin", "dave"}

// ownersPreview says who the missing required review is from and which of the
// changed files each owner owns.
func ownersPreview(cw int) pview {
	pv := pview{key: "owners", title: "Required review · core team", info: dim("CODEOWNERS")}
	pv.lines = append(wrap("Review is required from the owners of the changed files. The core team owns 6 of the 8 changed files and has not reviewed yet.", cw), "")
	for _, who := range []string{"org/core", "bob", "dave"} {
		state := dim("code owner")
		switch who {
		case "org/core":
			state = yellow("required") + dim(" · no review yet")
		case "bob":
			state = green("approved") + dim(" · not required")
		}
		pv.lines = append(pv.lines, bold(disp(who))+" "+state)
		if who == "org/core" {
			var names []string
			for _, c := range coreMembers {
				names = append(names, disp(c))
			}
			pv.lines = append(pv.lines, dim("  members: "+strings.Join(names, ", ")))
		}
		var mine []file
		for _, f := range files {
			if slices.Contains(ownersOf(f.path()), who) {
				mine = append(mine, f)
			}
		}
		for i, f := range mine {
			g := "├─ "
			if i == len(mine)-1 {
				g = "╰─ "
			}
			pv.lines = append(pv.lines, ansi.Truncate(dim(g)+fileGlyph(f.name)+" "+f.path()+dim(fmt.Sprintf("  +%d −%d", f.add, f.del)), cw, "…"))
		}
		pv.lines = append(pv.lines, "")
	}
	pv.lines = append(pv.lines, bold("enter")+" "+dim("opens the reviewer picker to request them"))
	return pv
}

// ---------- Development ----------

type fakeComment struct{ who, when, body string }

// fakeIssue is the made-up content of a linked issue or pull request.
func fakeIssue(d devItem) (author, when string, labels []string, body string, cs []fakeComment) {
	switch d.num {
	case 12:
		return "dave", "2 weeks ago", []string{"bug", "api"},
			"Calling `Div` in a loop is **slow** on large inputs.\n\n- `Div(a, b)` allocates on every call\n- the benchmark in `calc_test.go` shows 4x the time of `a / b`\n\nExpected: no allocations. See also #9.",
			[]fakeComment{{"alice", "1 week ago", "Reproduced; the allocation comes from the error wrapper."}, {"bob", "5d ago", "This PR should fix it once the panic is replaced."}, {"dave", "2d ago", "Thanks, will retest when it merges."}}
	case 15:
		return "erin", "3 weeks ago", []string{"docs"},
			"The README says `calc.Add` takes three arguments. It takes two.",
			[]fakeComment{{"erin", "3 weeks ago", "Fixed in the docs commit."}}
	case 9:
		return "dave", "1 month ago", []string{"enhancement"},
			"The benchmark suite is slow on CI: it runs every size twice.",
			[]fakeComment{{"carol", "3 weeks ago", "Run the sizes once and use -count for repeats."}}
	}
	return "bob", "1 week ago", []string{"enhancement"}, "A made-up description of this item.", []fakeComment{{"alice", "3d ago", "Looks good."}}
}

func chips(labels []string) string {
	var cs []string
	for _, l := range labels {
		cs = append(cs, chip(l))
	}
	return strings.Join(cs, " ")
}

func (m *model) devPreview(nd tnode, cw int) pview {
	switch nd.kind {
	case "hdr":
		pv := pview{key: "dhdr|" + nd.grp, title: strings.TrimSpace(ansi.Strip(nd.text))}
		want := "closes"
		if nd.grp == "prs" {
			want = "pr"
		}
		for _, d := range m.dev {
			if d.kind == want {
				pv.lines = append(pv.lines, devGlyph(d.kind, d.state)+fmt.Sprintf(" #%d ", d.num)+d.title+dim("  "+d.state))
			}
		}
		if len(pv.lines) == 0 {
			pv.lines = []string{dim("nothing linked")}
		}
		return pv
	case "issue", "blocker":
		d := m.dev[nd.idx]
		if nd.kind == "blocker" {
			for _, b := range d.by {
				if b.num == nd.num {
					d = b
				}
			}
		}
		author, when, labels, body, cs := fakeIssue(d)
		st := ic.issueOpen.String() + " " + green("open")
		if d.state == "closed" {
			st = ic.issueClosed.String() + " " + icPurple("closed")
		}
		pv := pview{key: fmt.Sprintf("issue|%d|%s", d.num, d.state), title: fmt.Sprintf("Issue #%d", d.num), info: dim(d.state)}
		pv.lines = append(pv.lines, wrap(bold(d.title), cw)...)
		pv.lines = append(pv.lines, st+dim(" · "+disp(author)+" opened this "+when), chips(labels))
		for _, b := range d.by {
			pv.lines = append(pv.lines, ic.blocked.String()+" "+yellow(fmt.Sprintf("blocked by #%d %s", b.num, b.title)))
		}
		pv.lines = append(pv.lines, "")
		pv.lines = append(pv.lines, mdPlain(body, cw)...)
		pv.lines = append(pv.lines, "", rule(cw), bold(fmt.Sprintf("%d comments", len(cs))))
		for _, c := range cs[max(0, len(cs)-3):] {
			pv.lines = append(pv.lines, "", bold(disp(c.who))+dim(" · "+c.when))
			for _, l := range wrap(c.body, cw-2) {
				pv.lines = append(pv.lines, "  "+l)
			}
		}
		return pv
	case "pr":
		d := m.dev[nd.idx]
		_, _, labels, body, _ := fakeIssue(d)
		pv := pview{key: fmt.Sprintf("pr|%d|%s", d.num, d.state), title: fmt.Sprintf("Pull request #%d", d.num), info: dim(d.state)}
		pv.lines = append(pv.lines, wrap(devGlyph("pr", d.state)+" "+bold(fmt.Sprintf("#%d %s", d.num, d.title)), cw)...)
		pv.lines = append(pv.lines,
			dim("by ")+disp("bob")+dim(" · ")+cyan("refactor/arith")+dim(" → ")+cyan("main"),
			"",
			checkIcon("pass")+" "+dim("checks: ")+"3 of 4 passing, 1 pending",
			ic.approved.String()+" "+dim("review: ")+"approved by "+disp("erin"),
			dim("changes: ")+green("+84")+" "+red("−31")+dim(" in 6 files"),
			dim("labels: ")+chips(labels), "", rule(cw))
		ex := mdPlain(body, cw)
		pv.lines = append(pv.lines, ex[:min(len(ex), 6)]...)
		if len(ex) > 6 {
			pv.lines = append(pv.lines, dim("⋯"))
		}
		return pv
	}
	return pview{key: "dempty", title: "Development", lines: []string{dim("nothing here yet"), "", dim("a links an issue")}}
}

// ---------- Involved ----------

func (m *model) involvedPreview(nd tnode, cw int) pview {
	switch nd.kind {
	case "hdr":
		pv := pview{key: "ihdr|" + nd.grp, title: strings.TrimSpace(ansi.Strip(nd.text))}
		switch nd.grp {
		case "rev":
			for _, r := range m.reviewers {
				pv.lines = append(pv.lines, reviewIcon(r.state)+" "+dispS(r.name)+dim("  "+r.state))
			}
		case "asg":
			for _, a := range m.assignees {
				pv.lines = append(pv.lines, disp(a)+dim("  "+a))
			}
		default:
			for _, c := range m.commenters() {
				pv.lines = append(pv.lines, disp(c)+dim(fmt.Sprintf("  %d comments or reviews", m.said(c))))
			}
		}
		if len(pv.lines) == 0 {
			pv.lines = []string{dim("nobody")}
		}
		return pv
	case "person":
		login := nd.key
		pv := pview{key: "person|" + login + fmt.Sprint(m.reviewerState(login)), title: disp(login)}
		if _, _, ok := strings.Cut(login, "/"); ok {
			pv.lines = append(pv.lines, dim("Team members:"))
			for _, c := range coreMembers {
				pv.lines = append(pv.lines, "  "+disp(c)+dim("  "+c))
			}
			pv.lines = append(pv.lines, "")
		}
		var roles []string
		if login == prAuthor {
			roles = append(roles, "author")
		}
		if st := m.reviewerState(login); st != "" {
			pv.lines = append(pv.lines, reviewIcon(st)+" "+bold("review: "+st))
			roles = append(roles, "reviewer")
		}
		if slices.Contains(m.assignees, login) {
			roles = append(roles, "assignee")
		}
		pv.info = dim(strings.Join(roles, " · "))
		pv.lines = append(pv.lines, rule(cw))
		blocks := m.convBlocks(true, func(r crow) bool {
			switch r.kind {
			case "comment", "review":
				return r.it.who == login
			case "thread":
				return slices.ContainsFunc(r.th.replies, func(p *reply) bool { return p.who == login })
			}
			return false
		}, cw)
		if len(blocks) == 0 {
			blocks = []string{dim("no comments or reviews from " + disp(login) + " in this pull request")}
		}
		pv.lines = append(pv.lines, blocks...)
		return pv
	}
	return pview{key: "iempty", title: "Involved", lines: []string{dim("nobody yet"), "", dim("enter requests or assigns")}}
}

func reviewIcon(state string) string {
	switch state {
	case "approved":
		return ic.approved.String()
	case "changes":
		return ic.changes.String()
	case "re-requested":
		return ic.rerequested.String()
	}
	return ic.requested.String()
}

// reviewerState is the review state of a requested reviewer, "" for others.
func (m *model) reviewerState(login string) string {
	for _, r := range m.reviewers {
		if r.name == login {
			return r.state
		}
	}
	return ""
}

// said counts the comments and reviews of login, threads included.
func (m *model) said(login string) int {
	n := 0
	for _, it := range m.cv.items[1:] {
		if (it.kind == "comment" || it.kind == "review") && it.who == login {
			n++
		}
		for _, t := range it.threads {
			for _, r := range t.replies {
				if r.who == login {
					n++
				}
			}
		}
	}
	return n
}
