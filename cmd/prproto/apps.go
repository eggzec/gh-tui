package main

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
)

func blue(t string) string { return "\x1b[34m" + t + "\x1b[39m" }

// link is an OSC 8 terminal hyperlink. The ansi width helpers skip the
// escapes, so the text keeps its width.
func link(url, text string) string {
	return "\x1b]8;;" + url + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}

// openApp opens the page of an app check run in a modal.
func (m *model) openApp(sha, label string) {
	m.push(overlay{kind: "app", title: label, text: sha})
}

func (o *overlay) appRun() appRun {
	app, name, _ := strings.Cut(o.title, " / ")
	return appRunOf(o.text, app, name)
}

func (m *model) drawApp(o *overlay, w, h int) string {
	a := o.appRun()
	head, rows := appPage(a, w-4)
	o.pcur = clampInt(o.pcur, 0, max(0, len(rows)-1))
	all := slices.Clone(head)
	for i, r := range rows {
		if i == o.pcur {
			r = selRow(r, w-4, true)
		}
		all = append(all, r)
	}
	slots := max(1, h-2)
	cur := len(head) + o.pcur
	if len(rows) == 0 {
		cur = 0
	}
	o.scroll = clampInt(o.scroll, max(0, cur-slots+1), cur)
	o.scroll = clampInt(o.scroll, 0, max(0, len(all)-slots))
	end := min(len(all), o.scroll+slots)
	return boxR(a.app, dim(a.name), all[o.scroll:end], w, h, true)
}

func (m *model) appKey(o *overlay, k string) tea.Cmd {
	a := o.appRun()
	switch k {
	case "esc", "backspace":
		m.pop()
	case "R":
		return m.rerunApp(o.text, o.title)
	case "enter":
		if len(a.anns) > 0 {
			n := a.anns[clampInt(o.pcur, 0, len(a.anns)-1)]
			m.pop()
			return m.jumpAnnot(n)
		}
	default:
		if d := moveDelta(k); d != 0 {
			o.pcur = clampInt(o.pcur+d, 0, max(0, len(a.anns)-1))
		}
	}
	return nil
}

// jumpAnnot opens the diff at the annotated line, like enter on a review thread.
func (m *model) jumpAnnot(n annot) tea.Cmd {
	fi := slices.IndexFunc(files, func(f file) bool { return f.path() == n.path })
	if fi < 0 {
		return m.say("the file " + n.path + " is not in this diff")
	}
	if !m.fileIn(fi) {
		m.lo, m.hi = 0, len(fcommits)-1
		m.rebuild()
	}
	m.push(overlay{kind: "file", arg: fi, cur: m.rowOf(fi)})
	o := &m.stack[len(m.stack)-1]
	doc, vis, _ := m.dview(o)
	for i, d := range vis {
		if doc[d].fi == fi && doc[d].nw == n.line && doc[d].kind != '-' {
			o.vcur = i
			break
		}
	}
	m.fixDiff(o)
	return nil
}

// rerunApp asks to re-request the check suite of an app check run.
func (m *model) rerunApp(sha, label string) tea.Cmd {
	app, name, _ := strings.Cut(label, " / ")
	a := appRunOf(sha, app, name)
	if a.state == "pending" {
		return m.say(name + " is already running")
	}
	m.confirmDo("Re-request check suite? ", "(prototype) re-requested "+app+" / "+name, func(m *model) tea.Cmd {
		rerunning[appKey(sha, a)] = true
		return nil
	})
	return nil
}

// Three kinds of checks, one tree grouped by source: GitHub Actions workflow
// runs (checks.go), check runs of other GitHub apps (an app row, its check runs
// below it) and plain commit statuses (a leaf with context and description).

// A distinct dim glyph tells the kinds apart before the name.
const (
	wfGlyph  = "" // nf-oct-play: an Actions workflow
	appGlyph = "" // nf-oct-apps: a GitHub app
	stGlyph  = "" // nf-oct-dot_fill: a commit status
)

// Level icons of annotations.
const (
	lvFailure = "" // nf-oct-x_circle_fill
	lvWarning = "" // nf-oct-alert
	lvNotice  = "" // nf-oct-info
)

// annot is an annotation of a check run: a finding on a line of the diff.
type annot struct {
	path  string
	line  int
	level string // failure, warning, notice
	msg   string
}

// appRun is a check run of a GitHub app.
type appRun struct {
	app, name, state, dur string
	title, summary, url   string
	anns                  []annot
}

// cstatus is a plain commit status.
type cstatus struct {
	ctx, state, desc, url string
}

func levelIcon(level string) string {
	switch level {
	case "failure":
		return red(lvFailure)
	case "warning":
		return yellow(lvWarning)
	}
	return blue(lvNotice)
}

func levelRank(level string) int {
	switch level {
	case "failure":
		return 3
	case "warning":
		return 2
	}
	return 1
}

// appRunsOf lists the app check runs of a commit; the head commit carries the
// findings.
func appRunsOf(sha string) []appRun {
	head := sha == headSHA()
	gg := appRun{
		app: "GitGuardian", name: "Security checks", state: "ok", dur: "4s",
		title: "No secrets detected", url: "https://dashboard.gitguardian.com/workspace/1/incidents/2271",
		summary: "GitGuardian scanned the commits of this pull request.\n\n- 0 secrets found",
	}
	if head {
		gg.state = "fail"
		gg.title = "1 secret uncovered"
		gg.summary = "## GitGuardian scan\n\nGitGuardian scanned **7 commits** and found **1 secret** that is still valid.\n\n- Generic High Entropy Secret in `pkg/calc/calc_test.go`\n\nRevoke the key first, then rewrite the history. Marking the incident as a false positive hides it for the whole repository."
		gg.anns = []annot{{"pkg/calc/calc_test.go", 5, "failure", "Generic High Entropy Secret: a hard-coded key was found in this test"}}
	}
	cq := appRun{
		app: "CodeQL", name: "Code scanning results", state: "ok", dur: "5m03s",
		title: "No new alerts", url: "https://github.com/acme/calc/security/code-scanning?query=pr%3A482",
		summary: "CodeQL analysed the Go code of this pull request.\n\n- 0 new alerts",
	}
	if head {
		cq.title = "1 new alert"
		cq.summary = "## Code scanning results\n\n**1 new alert** of level *warning*:\n\n- `go/division-by-zero`: the divisor `b` can be zero when it reaches `Div`\n\nFix it by checking the divisor, or return an error."
		cq.anns = []annot{{"pkg/calc/calc.go", 8, "warning", "Potential division by zero: b is not checked before the division"}}
	}
	cc := appRun{
		app: "Codecov", name: "codecov/patch", state: "ok", dur: "36s",
		title: "100.00% of diff hit", url: "https://app.codecov.io/gh/acme/calc/pull/482",
		summary: "Patch coverage is **100%**.",
	}
	if head {
		cc.title = "94.12% of diff hit (target 80.00%)"
		cc.summary = "## Codecov report\n\nPatch coverage is **94.12%**, above the target of 80.00%.\n\n- 1 line is not covered by tests: `pkg/calc/calc.go`"
		cc.anns = []annot{{"pkg/calc/calc.go", 42, "notice", "This line is not covered by tests"}}
	}
	out := []appRun{gg, cq, cc}
	for i := range out {
		if rerunning[runKey(sha, "app:"+out[i].app+" / "+out[i].name)] {
			out[i].state = "pending"
		}
	}
	return out
}

// statusesOf lists the plain commit statuses of a commit.
func statusesOf(sha string) []cstatus {
	out := []cstatus{{"vercel", "ok", "Deployment ready", "https://acme-calc-" + sha + ".vercel.app"}}
	if sha == headSHA() {
		out = append(out, cstatus{"ci/jenkins", "pending", "Build 482 is running", "https://jenkins.acme.dev/job/calc/482/"})
	}
	return out
}

// appNames lists the apps with check runs on a commit, in order.
func appNames(sha string) []string {
	var out []string
	for _, a := range appRunsOf(sha) {
		if !slices.Contains(out, a.app) {
			out = append(out, a.app)
		}
	}
	return out
}

// appRunsIn lists the check runs of one app on a commit.
func appRunsIn(sha, app string) []appRun {
	var out []appRun
	for _, a := range appRunsOf(sha) {
		if a.app == app {
			out = append(out, a)
		}
	}
	return out
}

func appRunOf(sha, app, name string) appRun {
	for _, a := range appRunsIn(sha, app) {
		if a.name == name {
			return a
		}
	}
	return appRun{app: app, name: name, state: "ok"}
}

func statusOf(sha, ctx string) cstatus {
	for _, s := range statusesOf(sha) {
		if s.ctx == ctx {
			return s
		}
	}
	return cstatus{ctx: ctx, state: "ok"}
}

func stateOfApps(rs []appRun) string {
	st := "ok"
	for _, r := range rs {
		switch r.state {
		case "fail":
			return "fail"
		case "pending":
			st = "pending"
		}
	}
	return st
}

// allState is the combined status of every kind of check of a commit.
func allState(sha string) string {
	st := commitState(sha)
	if st == "fail" {
		return st
	}
	if s := stateOfApps(appRunsOf(sha)); s == "fail" {
		return s
	} else if s == "pending" {
		st = s
	}
	for _, s := range statusesOf(sha) {
		switch s.state {
		case "fail":
			return "fail"
		case "pending":
			st = "pending"
		}
	}
	return st
}

// appKey is the re-run key of an app check run of a commit.
func appKey(sha string, a appRun) string { return runKey(sha, "app:"+a.app+" / "+a.name) }

// findings lists the annotations of the app checks on the head commit of the
// selected range that land on a file.
func (m *model) findings(path string) []annot {
	var out []annot
	for _, a := range appRunsOf(fcommits[m.hi].sha) {
		for _, n := range a.anns {
			if n.path == path {
				out = append(out, n)
			}
		}
	}
	return out
}

// findingApp is the app of an annotation of the range's head commit.
func (m *model) findingApp(n annot) string {
	for _, a := range appRunsOf(fcommits[m.hi].sha) {
		if slices.Contains(a.anns, n) {
			return a.app
		}
	}
	return ""
}

// findingRows are the dim lines under an annotated diff line, like GitHub's
// annotations on the Files changed tab: level icon, app and message. They are
// text only (no cursor stop), so ) and ( keep stepping over threads and drafts.
func (m *model) findingRows(x dl, li int) []vrow {
	var out []vrow
	path := files[x.fi].path()
	for _, a := range appRunsOf(fcommits[m.hi].sha) {
		for _, n := range a.anns {
			if n.path == path && n.line == x.nw {
				out = append(out, vrow{text: "     " + dim("▎ ") + levelIcon(n.level) + " " + dim(a.app+": "+n.msg), li: li, find: true})
			}
		}
	}
	return out
}

// findingBadge is the count of findings of a file with the worst level's icon.
func (m *model) findingBadge(path string) string {
	fs := m.findings(path)
	if len(fs) == 0 {
		return ""
	}
	w := fs[0].level
	for _, n := range fs {
		if levelRank(n.level) > levelRank(w) {
			w = n.level
		}
	}
	return levelIcon(w) + dim(fmt.Sprintf("%d", len(fs)))
}

// ---------- tree rows ----------

// appLine draws an app row: fold arrow, status icon, glyph, name.
func appLine(gm, sha, app string, open bool, cw int) string {
	rs := appRunsIn(sha, app)
	st := stateOfApps(rs)
	left := gm + arrowOf(open) + checkIcon(st) + " " + dim(appGlyph) + " " + app
	return rightAlign(left, dim(stateWord(st)), cw)
}

// appRunLine draws a check run of an app.
func appRunLine(gm string, a appRun, cw int) string {
	right := stateWord(a.state)
	if n := len(a.anns); n > 0 {
		w := a.anns[0].level
		for _, x := range a.anns {
			if levelRank(x.level) > levelRank(w) {
				w = x.level
			}
		}
		right = fmt.Sprintf("%d finding", n)
		if n > 1 {
			right += "s"
		}
		right = levelIcon(w) + " " + dim(right+" · "+stateWord(a.state))
		return rightAlign(gm+checkIcon(a.state)+" "+a.name, right, cw)
	}
	return rightAlign(gm+checkIcon(a.state)+" "+a.name, dim(right+" · "+a.dur), cw)
}

// statusLine draws a plain commit status: a leaf with context and description.
func statusLine(gm string, s cstatus, cw int) string {
	return rightAlign(gm+checkIcon(s.state)+" "+dim(stGlyph)+" "+link(s.url, s.ctx), dim(s.desc), cw)
}

// wfSummary is the text of a workflow row's glyph prefix: the kind marker.
func wfKind(s string) string { return dim(wfGlyph) + " " + s }

// ---------- the app check run page ----------

// appPage is the content of an app check run: title, summary and annotations.
// The annotation rows are indexed from 0 in the returned slice of rows.
func appPage(a appRun, w int) (head []string, rows []string) {
	head = append(head,
		bold(a.name)+dim("  ·  "+a.app+"  ·  ")+stateText(a.state),
		link(a.url, dim(a.url)),
		"",
		bold(a.title),
		"")
	for _, l := range mdPlain(a.summary, w) {
		head = append(head, l)
	}
	if len(a.anns) > 0 {
		head = append(head, "", bold(fmt.Sprintf("Annotations (%d)", len(a.anns))))
	}
	for _, n := range a.anns {
		rows = append(rows, fmt.Sprintf("%s:%d  %s %s", n.path, n.line, levelIcon(n.level), n.msg))
	}
	return head, rows
}

func stateText(state string) string {
	switch state {
	case "fail":
		return red("failing")
	case "pending":
		return yellow("pending")
	}
	return green("passing")
}

// statusPage is the preview of a plain status: description and URL.
func statusPage(s cstatus) []string {
	return []string{
		bold(s.ctx) + dim("  ·  commit status  ·  ") + stateText(s.state),
		"",
		s.desc,
		"",
		link(s.url, dim(s.url)),
	}
}
