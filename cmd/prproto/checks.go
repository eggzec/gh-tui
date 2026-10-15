package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Checks live in two places only: the Merge pane's blockers and under a commit
// when it is open (Files tree, Conversation push). enter on any check row opens
// the workflow run of that check in an imitation of the real Actions modal.

// A run is one check of one commit.
type run struct{ label, state, dur string }

var checkDefs = []run{
	{"ci / build (linux)", "ok", "1m12s"},
	{"ci / test (linux)", "ok", "3m22s"},
	{"ci / lint", "ok", "0m48s"},
	{"codeql / analyze", "ok", "5m03s"},
}

// rerunning holds the checks re-run in this session, shown pending everywhere.
var rerunning = map[string]bool{}

func runKey(sha, label string) string { return sha + "|" + label }

func headSHA() string { return fcommits[len(fcommits)-1].sha }

// commitBase is the fake status a commit's checks had when they ran.
func commitBase(sha string) string {
	switch sha {
	case "a1b2c3d", "f9e8d7c", headSHA():
		return "fail"
	case "c3d4e5f":
		return "pending"
	}
	return "ok"
}

// commitChecks lists the checks of a commit, grouped by workflow.
func commitChecks(sha string) []run {
	base := commitBase(sha)
	var out []run
	for _, d := range checkDefs {
		r := d
		switch {
		case base == "fail" && d.label == "ci / test (linux)",
			base == "fail" && sha == headSHA() && d.label == "ci / lint":
			r.state = "fail"
		case base == "pending" && d.label == "ci / lint":
			r.state = "pending"
		}
		if rerunning[runKey(sha, d.label)] {
			r.state = "pending"
		}
		out = append(out, r)
	}
	return out
}

func runOf(sha, label string) run {
	for _, r := range commitChecks(sha) {
		if r.label == label {
			return r
		}
	}
	return run{label, "ok", "0m10s"}
}

// commitState is the combined status of a commit: failing, else pending, else ok.
func commitState(sha string) string {
	return worst(commitChecks(sha))
}

// worst is the combined status of some checks: failing, else pending, else ok.
func worst(rs []run) string {
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

// checkLabel paints a check's name, its workflow dim.
func checkLabel(label string) string {
	if wf, job, ok := strings.Cut(label, " / "); ok {
		return dim(wf+" / ") + job
	}
	return label
}

func stateWord(state string) string {
	switch state {
	case "fail":
		return "failed"
	case "pending":
		return "pending"
	}
	return "passed"
}

// ciRe reads a CI event of the conversation: its check and commit.
var ciRe = regexp.MustCompile(`Check \*\*(.+?)\*\* \S+ on \*\*(\w+)\*\*`)

func ciRef(it *citem) (sha, label string, ok bool) {
	m := ciRe.FindStringSubmatch(it.body)
	if m == nil {
		return "", "", false
	}
	return m[2], m[1], true
}

// ciText is the text of a CI event, following re-runs.
func ciText(it *citem) string {
	sha, label, ok := ciRef(it)
	if !ok {
		return it.body
	}
	switch runOf(sha, label).state {
	case "pending":
		return "Check **" + label + "** is running again on **" + sha + "**"
	case "ok":
		return "Check **" + label + "** passed on **" + sha + "**"
	}
	return it.body
}

// ---------- the Actions-style run modal ----------
//
// Like the real one: Runs | Jobs | Log side by side (one pane at a time when
// narrow or zoomed). Keys: tab/shift+tab and 1-3 pane, j/k g/G move, e/E next
// and previous error and w/W warning in the log, z zoom, R re-run the failed
// jobs, J re-run the job, esc/backspace close.

const (
	apRuns = iota
	apJobs
	apLog
)

const errBG = "\x1b[48;5;52m"

type logStep struct {
	name, state, dur string
	lines            []string
}

// runSteps is the fake log of a job, a group per step like GitHub's.
func runSteps(sha string, r run) []logStep {
	n := 0
	ts := func() string { return dim(fmt.Sprintf("12:%02d:%02d ", 4+n/60, n%60)) }
	line := func(s string) string {
		n++
		return ts() + s
	}
	job := r.label
	if _, j, ok := strings.Cut(r.label, " / "); ok {
		job = j
	}
	steps := []logStep{
		{"Set up job", "ok", "2s", []string{line("Current runner version: 2.321.0"), line("Runner: ubuntu-24.04, 4 cores"), line("Prepare workflow directory")}},
		{"Checkout", "ok", "3s", []string{line("Fetching the repository at " + sha), line("git fetch --depth=1 origin " + sha), line("HEAD is now at " + sha)}},
		{"Set up Go 1.25", "ok", "1s", []string{line("Using the cached toolchain go1.25.1"), line("go version go1.25.1 linux/amd64")}},
	}
	body := logStep{name: "Run " + job, state: "ok", dur: r.dur}
	for i := 1; i <= 24; i++ {
		body.lines = append(body.lines, line(fmt.Sprintf("=== RUN   TestCase%02d", i)))
		if i == 9 {
			body.lines = append(body.lines, line("\x1b[33m##[warning]Cache not found for key go-build-linux\x1b[39m"))
		}
		switch {
		case r.state == "fail" && i == 17:
			body.state = "fail"
			body.lines = append(body.lines,
				line(red("--- FAIL: TestCase17 (0.02s)")),
				line(red("    calc_test.go:41: Div(1, 0) panicked: runtime error: integer divide by zero")),
				line(red("FAIL")),
				line(red("##[error]Process completed with exit code 1.")))
			return append(steps, body, logStep{"Complete job", "ok", "0s", []string{line("Cleaning up orphan processes")}})
		case r.state == "pending" && i == 20:
			body.state = "pending"
			body.dur = "…"
			body.lines = append(body.lines, line(dim("… the job is still running")))
			return append(steps, body)
		}
		body.lines = append(body.lines, line(green(fmt.Sprintf("--- PASS: TestCase%02d (0.0%ds)", i, i%7))))
	}
	body.lines = append(body.lines, line(green("PASS")))
	steps = append(steps, body,
		logStep{"Post Checkout", "ok", "0s", []string{line("Post job cleanup.")}},
		logStep{"Complete job", "ok", "0s", []string{line("Cleaning up orphan processes")}})
	return steps
}

// alog is a line of the log pane.
type alog struct {
	text      string
	err, warn bool
}

func logLines(sha string, r run) []alog {
	var out []alog
	for _, st := range runSteps(sha, r) {
		name := st.name
		if st.state == "fail" {
			name = red(bold(name))
		}
		out = append(out, alog{text: dim("▾ ") + checkIcon(st.state) + " " + name + dim("  "+st.dur)})
		for _, l := range st.lines {
			p := ansi.Strip(l)
			out = append(out, alog{
				text: "  " + l,
				err:  strings.Contains(p, "FAIL") || strings.Contains(p, "##[error]") || strings.Contains(p, "panicked"),
				warn: strings.Contains(p, "##[warning]"),
			})
		}
	}
	return out
}

// wfs lists the workflows of a commit's runs in order.
func wfs(sha string) []string {
	var out []string
	for _, r := range commitChecks(sha) {
		wf, _, _ := strings.Cut(r.label, " / ")
		if len(out) == 0 || out[len(out)-1] != wf {
			out = append(out, wf)
		}
	}
	return out
}

// wfJobs lists the jobs of one workflow run of a commit.
func wfJobs(sha, wf string) []run {
	var out []run
	for _, r := range commitChecks(sha) {
		if strings.HasPrefix(r.label, wf+" / ") {
			out = append(out, r)
		}
	}
	return out
}

func runNumber(i int) int { return 1204 - i*3 }

// The run → job tree is drawn the same everywhere (Files, Conversation, Merge):
// a workflow row, then its jobs, after the shared guides gm.

// commitIcon is the combined status icon of a commit, folded or open; a commit
// with no checks has none.
func commitIcon(sha string) string {
	if len(commitChecks(sha)) == 0 {
		return ""
	}
	return checkIcon(allState(sha))
}

// jobName is the job part of a "workflow / job" label.
func jobName(label string) string {
	if _, j, ok := strings.Cut(label, " / "); ok {
		return j
	}
	return label
}

// wfNumber is the run number of a workflow of a commit.
func wfNumber(sha, wf string) int {
	return runNumber(max(0, slices.Index(wfs(sha), wf)))
}

// arrowOf is the fold arrow of a tree row.
func arrowOf(open bool) string {
	if open {
		return dim("▾ ")
	}
	return dim("▸ ")
}

// wfLine draws a workflow run row: fold arrow, status icon, name, run number
// and, on the right, right or else the run's status. The icon stays when the
// row is open.
func wfLine(gm, sha, wf string, open bool, right string, cw int) string {
	st := worst(wfJobs(sha, wf))
	if right == "" {
		right = stateWord(st)
	}
	left := gm + arrowOf(open) + checkIcon(st) + " " + wf + dim(fmt.Sprintf(" #%d", wfNumber(sha, wf)))
	return rightAlign(left, dim(right), cw)
}

// jobLine draws a job row of a run.
func jobLine(gm string, ck run, cw int) string {
	return rightAlign(gm+checkIcon(ck.state)+" "+jobName(ck.label), dim(stateWord(ck.state)+" · "+ck.dur), cw)
}

// openLog opens the run of a check: cursor on its failing job and the log at
// the first error.
func (m *model) openLog(sha, label string) {
	wf, _, _ := strings.Cut(label, " / ")
	o := overlay{kind: "log", title: label, text: sha, ap: apJobs}
	for i, w := range wfs(sha) {
		if w == wf {
			o.ar = i
		}
	}
	jobs := wfJobs(sha, wf)
	o.aj = 0
	for i, j := range jobs {
		if j.label == label {
			o.aj = i
		}
	}
	if jobs[o.aj].state != "fail" {
		for i, j := range jobs {
			if j.state == "fail" {
				o.aj = i
				break
			}
		}
	}
	o.jumpToError(sha)
	m.push(o)
}

// job is the run's job under the cursor.
func (o *overlay) job() (wf string, jobs []run, j run) {
	ws := wfs(o.text)
	o.ar = max(0, min(o.ar, len(ws)-1))
	wf = ws[o.ar]
	jobs = wfJobs(o.text, wf)
	o.aj = max(0, min(o.aj, len(jobs)-1))
	return wf, jobs, jobs[o.aj]
}

// jumpToError scrolls the log so that the first error is in view, else the top.
func (o *overlay) jumpToError(sha string) {
	_, _, j := o.job()
	o.ascroll = 0
	for i, l := range logLines(sha, j) {
		if l.err {
			o.ascroll = max(0, i-3)
			return
		}
	}
}

func bgRow(l string, w int, bg string) string {
	l = strings.NewReplacer("\x1b[0m", "\x1b[0m"+bg, "\x1b[m", "\x1b[m"+bg, "\x1b[39m", "\x1b[39m"+bg).Replace(l)
	pad := strings.Repeat(" ", max(0, w-ansi.StringWidth(l)))
	return bg + l + pad + "\x1b[0m"
}

func clampInt(v, lo, hi int) int { return max(lo, min(v, hi)) }

func (m *model) drawLog(o *overlay, w, h int) string {
	wf, jobs, job := o.job()
	three := w >= 100 && !o.az
	rw := clampInt(w*30/100, 34, 52)
	jw := clampInt(w*22/100, 24, 36)
	lw := w - rw - jw
	if !three {
		rw, jw, lw = w, w, w
	}
	slots := max(1, h-2)

	runsPane := func() string {
		var ls []string
		for i, name := range wfs(o.text) {
			js := wfJobs(o.text, name)
			l := rightAlign(checkIcon(worst(js))+" "+name+dim(fmt.Sprintf(" #%d", runNumber(i))), dim(o.text), rw-4)
			if i == o.ar {
				l = selRow(l, rw-4, o.ap == apRuns)
			}
			ls = append(ls, l)
		}
		return boxR("1 Runs", dim(fmt.Sprintf("%d runs", len(ls))), ls, rw, h, o.ap == apRuns)
	}
	jobsPane := func() string {
		var ls []string
		for i, j := range jobs {
			_, name, _ := strings.Cut(j.label, " / ")
			d := j.dur
			if j.state == "pending" {
				d = "running"
			}
			l := rightAlign(checkIcon(j.state)+" "+name, dim(d), jw-4)
			if i == o.aj {
				l = selRow(l, jw-4, o.ap == apJobs)
			}
			ls = append(ls, l)
		}
		return boxR("2 Jobs", dim(fmt.Sprintf("%s #%d · %s", wf, runNumber(o.ar), stateWord(worst(jobs)))), ls, jw, h, o.ap == apJobs)
	}
	logPane := func() string {
		rows := logLines(o.text, job)
		o.ascroll = clampInt(o.ascroll, 0, max(0, len(rows)-slots))
		var ls []string
		for i := o.ascroll; i < min(len(rows), o.ascroll+slots); i++ {
			l := rows[i].text
			switch {
			case rows[i].err:
				l = bgRow(red("▌")+ansi.Strip(l), lw-4, errBG)
			case rows[i].warn:
				l = "\x1b[33m!\x1b[39m" + l[1:]
			}
			ls = append(ls, l)
		}
		_, name, _ := strings.Cut(job.label, " / ")
		info := dim(name)
		if len(rows) > slots {
			info += dim(fmt.Sprintf(" · %d%%", (o.ascroll+slots)*100/len(rows)))
		}
		return boxR("3 Log", info, ls, lw, h, o.ap == apLog)
	}
	if !three {
		switch o.ap {
		case apRuns:
			return runsPane()
		case apJobs:
			return jobsPane()
		}
		return logPane()
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, runsPane(), jobsPane(), logPane())
}

func (m *model) logKey(o *overlay, k string) tea.Cmd {
	_, jobs, job := o.job()
	slots := max(1, m.h-1-4-2)
	switch k {
	case "esc", "backspace":
		m.pop()
	case "tab":
		o.ap = (o.ap + 1) % 3
	case "shift+tab":
		o.ap = (o.ap + 2) % 3
	case "1", "2", "3":
		o.ap = int(k[0] - '1')
	case "z":
		o.az = !o.az
	case "R":
		return m.rerunFailed(o)
	case "J":
		if o.ap == apRuns {
			return m.say("Pick a job to re-run.")
		}
		return m.rerunJob(o, job)
	case "e", "E", "w", "W":
		if o.ap != apLog {
			return m.say(k + " jumps between errors and warnings in the log pane")
		}
		o.nextMark(k, slots, job)
	default:
		d := moveDelta(k)
		if d == 0 {
			return nil
		}
		switch o.ap {
		case apRuns:
			o.ar = clampInt(o.ar+d, 0, len(wfs(o.text))-1)
			o.aj = 0
			o.job()
			if _, js, _ := o.job(); worst(js) == "fail" {
				for i, j := range js {
					if j.state == "fail" {
						o.aj = i
						break
					}
				}
			}
			o.jumpToError(o.text)
		case apJobs:
			o.aj = clampInt(o.aj+d, 0, len(jobs)-1)
			o.jumpToError(o.text)
		default:
			o.ascroll = max(0, o.ascroll+d)
		}
	}
	return nil
}

// nextMark scrolls to the next (e, w) or previous (E, W) error or warning.
func (o *overlay) nextMark(k string, slots int, job run) {
	rows := logLines(o.text, job)
	mark := func(l alog) bool {
		if k == "e" || k == "E" {
			return l.err
		}
		return l.warn
	}
	// The line shown a few rows below the top is the current one.
	cur := o.ascroll + 3
	if k == "e" || k == "w" {
		for i := cur + 1; i < len(rows); i++ {
			if mark(rows[i]) {
				o.ascroll = max(0, i-3)
				return
			}
		}
		return
	}
	for i := cur - 1; i >= 0; i-- {
		if mark(rows[i]) {
			o.ascroll = max(0, i-3)
			return
		}
	}
}

// rerunFailed asks to re-run the failed jobs of the run, then shows them
// pending everywhere.
func (m *model) rerunFailed(o *overlay) tea.Cmd {
	wf, jobs, _ := o.job()
	var failed []run
	for _, j := range jobs {
		if j.state == "fail" {
			failed = append(failed, j)
		}
	}
	name := fmt.Sprintf("%s #%d", wf, runNumber(o.ar))
	if len(failed) == 0 {
		return m.say(name + " has no failed jobs to re-run.")
	}
	q := fmt.Sprintf("Re-run %d failed jobs of %s? ", len(failed), name)
	if len(failed) == 1 {
		q = fmt.Sprintf("Re-run 1 failed job of %s? ", name)
	}
	sha := o.text
	m.confirmDo(q, "(prototype) re-running the failed jobs of "+name, func(m *model) tea.Cmd {
		for _, j := range failed {
			rerunning[runKey(sha, j.label)] = true
		}
		return nil
	})
	return nil
}

// rerunJob asks to re-run one job of the run.
func (m *model) rerunJob(o *overlay, j run) tea.Cmd {
	wf, _, _ := o.job()
	_, name, _ := strings.Cut(j.label, " / ")
	rn := fmt.Sprintf("%s #%d", wf, runNumber(o.ar))
	if j.state == "pending" {
		return m.say(name + " is still running")
	}
	sha := o.text
	m.confirmDo(fmt.Sprintf("Re-run %s of %s? ", name, rn), "(prototype) re-running "+name, func(m *model) tea.Cmd {
		rerunning[runKey(sha, j.label)] = true
		return nil
	})
	return nil
}
