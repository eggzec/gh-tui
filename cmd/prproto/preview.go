package main

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
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

	// A preview of reviews and comments is the Conversation itself, narrowed
	// to conv's items, under the fixed head lines.
	head []string
	conv *convSpec
}

// convSpec says which items of the Conversation a preview shows; key names the
// view, which keeps its folds, cursor and scroll while the row stays hovered.
type convSpec struct {
	key    string
	keepIt func(*citem) bool
	keepTh func(*thread) bool
}

// pconv is the state of one such view.
type pconv struct {
	cv     convState
	scroll int
}

// convFocus tells whether the Conversation view being drawn or handled has the
// focus: the pane itself, or the preview when it has the tab focus.
func (m *model) convFocus() bool {
	if m.pcOn {
		return m.pfocus == 1
	}
	return m.focus == pMain
}

// pcFor returns the view of spec, started on its first use.
func (m *model) pcFor(spec *convSpec) *pconv {
	if m.pcs == nil {
		m.pcs = map[string]*pconv{}
	}
	p := m.pcs[spec.key]
	if p == nil {
		p = &pconv{cv: convState{open: map[string]bool{}, hideSys: true, noRuns: true, keepIt: spec.keepIt, keepTh: spec.keepTh}}
		m.pcs[spec.key] = p
	}
	return p
}

// inPC runs fn with the view p standing in for the Conversation (m.cv and
// m.scroll), w x h being its box. Every Conversation function then works on
// the preview as it does on the pane: rows, folds, cursor, pins and keys.
func (m *model) inPC(p *pconv, w, h int, fn func()) {
	p.cv.items = m.cv.items
	m.cv, p.cv = p.cv, m.cv
	m.scroll, p.scroll = p.scroll, m.scroll
	m.pcOn, m.pcSize = true, [2]int{w, h}
	defer func() {
		m.cv, p.cv = p.cv, m.cv
		m.scroll, p.scroll = p.scroll, m.scroll
		m.pcOn = false
	}()
	fn()
}

// pcInner is the height of the Conversation inside the preview box: the box
// without its border and head lines.
func (m *model) pcInner(pv pview, h int) int { return max(1, h-2-len(pv.head)) }

// convPreview draws the Conversation view of pv in a box of w x h.
func (m *model) convPreview(pv pview, w, h int) string {
	ih := m.pcInner(pv, h)
	var body []string
	info := pv.info
	m.inPC(m.pcFor(pv.conv), w, ih+2, func() {
		all := m.convLines(w - 4)
		off := min(m.scroll, m.scrollMax(len(all), ih))
		var pins []string
		for _, j := range m.convPins(off) {
			if j == m.cv.first && m.convFocus() {
				pins = append(pins, all[j])
			} else {
				pins = append(pins, pinRow(all[j], w-4))
			}
		}
		pins = pins[:min(len(pins), max(0, ih-1))]
		end := min(len(all), off+ih-len(pins))
		body = append(append(slices.Clone(pv.head), pins...), all[off:end]...)
		if len(all) > ih {
			if info != "" {
				info += dim(" · ")
			}
			info += dim(fmt.Sprintf("%d%%", (off+ih)*100/len(all)))
		}
	})
	return boxR(pv.title, info, body, w, h, m.pfocus == 1)
}

// pcKey handles a key in the focused Conversation preview with the
// Conversation's own key handling: the cursor moves line by line, h/l/enter
// fold, * folds all, ) ( jump between unresolved threads, and c T e X act on
// the row as they do in the pane. f and s (filter, events) do nothing here,
// since the preview is already narrowed.
func (m *model) pcKey(pv pview, k string) tea.Cmd {
	w, h := m.w-listW, m.bodyH()
	var cmd tea.Cmd
	m.inPC(m.pcFor(pv.conv), w, m.pcInner(pv, h)+2, func() {
		switch d := moveDelta(k); {
		case k == "f" || k == "s":
		case d != 0:
			if rows := m.crows(false); len(rows) > 0 {
				m.setSel(rows, m.selIdx(rows)+d)
			}
		default:
			cmd = m.convKey(k)
		}
		m.fixConv()
	})
	return cmd
}

// anyItem tells whether the Conversation has an item that spec keeps.
func (m *model) anyItem(spec *convSpec) bool {
	c := convState{keepIt: spec.keepIt, keepTh: spec.keepTh, hideSys: true}
	return slices.ContainsFunc(m.cv.items[1:], c.itShown)
}

// previewBox draws the preview beside the zoomed list.
func (m *model) previewBox() string {
	w, h := m.w-listW, m.bodyH()
	pv := m.preview(w - 4)
	if pv.conv != nil {
		return m.convPreview(pv, w, h)
	}
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
		bs := m.blockers()
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

// ---------- Merge ----------

func (m *model) mergePreview(b blocker, cw int) pview {
	switch b.kind {
	case "checks":
		sha := headSHA()
		pv := pview{key: "checks|" + b.state + fmt.Sprint(m.blockers()), title: b.text, info: dim("failing runs of the head commit " + sha)}
		for _, wf := range wfs(sha) {
			jobs := wfJobs(sha, wf)
			if worst(jobs) == "ok" {
				continue
			}
			pv.lines = append(pv.lines, checkIcon(worst(jobs))+" "+bold(wf)+dim(fmt.Sprintf(" #%d · %s", wfNumber(sha, wf), stateWord(worst(jobs)))))
			for _, j := range jobs {
				if j.state != "ok" {
					pv.lines = append(pv.lines, "   "+checkIcon(j.state)+" "+jobName(j.label)+dim(" "+stateWord(j.state)+" · "+j.dur))
				}
			}
			pv.lines = append(pv.lines, "")
		}
		for _, app := range appNames(sha) {
			runs := appRunsIn(sha, app)
			if stateOfApps(runs) == "ok" {
				continue
			}
			pv.lines = append(pv.lines, checkIcon(stateOfApps(runs))+" "+dim(appGlyph)+" "+bold(app))
			for _, a := range runs {
				if a.state != "ok" {
					pv.lines = append(pv.lines, "   "+checkIcon(a.state)+" "+a.name+dim(" "+a.title))
				}
			}
			pv.lines = append(pv.lines, "")
		}
		for _, s := range statusesOf(sha) {
			if s.state != "ok" {
				pv.lines = append(pv.lines, checkIcon(s.state)+" "+dim(stGlyph)+" "+link(s.url, s.ctx)+dim(" "+s.desc))
			}
		}
		pv.lines = append(pv.lines, "", dim("enter opens a job's log or a check run's page; statuses only show here"))
		return pv
	case "app":
		sha := headSHA()
		runs := appRunsIn(sha, b.wf)
		st := stateOfApps(runs)
		pv := pview{key: "app|" + b.wf + "|" + st, title: b.wf, info: dim(stateWord(st))}
		pv.lines = append(pv.lines, dim("check runs on "+sha), "")
		for _, a := range runs {
			pv.lines = append(pv.lines, checkIcon(a.state)+" "+a.name+dim(" "+stateWord(a.state)+" · "+a.dur), "   "+dim(a.title))
		}
		pv.lines = append(pv.lines, "", dim("enter folds the app; enter on a check run opens its page"))
		return pv
	case "arun":
		app, name, _ := strings.Cut(b.label, " / ")
		return appRunPreview(appRunOf(headSHA(), app, name), cw)
	case "status":
		s := statusOf(headSHA(), b.label)
		return pview{key: "status|" + s.ctx + "|" + s.state, title: s.ctx, info: dim(stateWord(s.state)), lines: statusPage(s)}
	case "wf":
		sha := headSHA()
		jobs := wfJobs(sha, b.wf)
		st := worst(jobs)
		pv := pview{key: "wf|" + b.wf + "|" + st, title: fmt.Sprintf("%s · run #%d", b.wf, wfNumber(sha, b.wf)), info: dim(stateWord(st))}
		pv.lines = append(pv.lines, dim("on push · "+sha), "")
		for _, j := range jobs {
			pv.lines = append(pv.lines, checkIcon(j.state)+" "+jobName(j.label)+dim(" "+stateWord(j.state)+" · "+j.dur))
		}
		pv.lines = append(pv.lines, "", dim("enter folds the run; enter on a job opens its log"))
		return pv
	case "matrix":
		n := nodeOf(headSHA(), b.wf, b.label)
		return pview{key: "matrix|" + b.wf + "|" + b.label + "|" + n.summary(), title: b.wf + " · " + b.label, info: dim(n.summary()), lines: mxPreview(n)}
	case "check":
		sha := headSHA()
		r := runOf(sha, b.label)
		wf, job, _ := strings.Cut(b.label, " / ")
		pv := pview{key: "check|" + b.label + "|" + r.state,
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
		pv.key = fmt.Sprint("changes|", it.id)
		pv.conv = &convSpec{key: pv.key, keepIt: func(x *citem) bool { return x == it }}
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
			pv.lines[len(pv.lines)-1] += dim("  ⋯")
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
		said := func(t *thread) bool { return slices.ContainsFunc(t.replies, func(p *reply) bool { return p.who == login }) }
		spec := &convSpec{key: "person|" + login, keepTh: said, keepIt: func(x *citem) bool {
			return (x.kind == "comment" || x.kind == "review") && (x.who == login || slices.ContainsFunc(x.threads, said))
		}}
		pv.lines = append(pv.lines, rule(cw))
		if !m.anyItem(spec) {
			pv.lines = append(pv.lines, dim("no comments or reviews from "+disp(login)+" in this pull request"))
			return pv
		}
		pv.head, pv.lines = pv.lines, nil
		pv.conv = spec
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
