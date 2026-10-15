package main

import (
	"fmt"
	"regexp"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Checks live in two places only: the Merge pane's blockers and under a commit
// when it is open (Files tree, Conversation push). enter on any check row opens
// its log; R on a failed one re-runs it.

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
	st := "ok"
	for _, r := range commitChecks(sha) {
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

// ---------- the log ----------

type logStep struct {
	name, state, dur string
	lines            []string
}

// runSteps is the fake log of a check, a step per fold like GitHub's groups.
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

func (o *overlay) stepOpen(i int, st logStep) bool {
	if v, ok := o.open[i]; ok {
		return v
	}
	return st.state == "fail" || st.state == "pending"
}

type lrow struct {
	step int
	hdr  bool
	text string
}

func (o *overlay) logRows() []lrow {
	var rows []lrow
	for i, st := range runSteps(o.text, runOf(o.text, o.title)) {
		open := o.stepOpen(i, st)
		arrow := "▸ "
		if open {
			arrow = "▾ "
		}
		name := st.name
		if st.state == "fail" {
			name = red(bold(name))
		}
		rows = append(rows, lrow{i, true, arrow + checkIcon(st.state) + " " + name + dim("  "+st.dur)})
		if open {
			for _, l := range st.lines {
				rows = append(rows, lrow{i, false, "    " + l})
			}
		}
	}
	return rows
}

func (m *model) openLog(sha, label string) {
	m.push(overlay{kind: "log", title: label, text: sha})
}

func (m *model) logTitle(o *overlay) string {
	r := runOf(o.text, o.title)
	dur := r.dur
	if r.state == "pending" {
		dur = "running"
	}
	return fmt.Sprintf("%s · %s · %s · %s", r.label, o.text, stateWord(r.state), dur)
}

func (m *model) drawLog(o *overlay, w, h int) string {
	rows := o.logRows()
	slots := max(1, h-2)
	o.pcur = max(0, min(o.pcur, len(rows)-1))
	off := max(0, min(o.pcur-slots/2, len(rows)-slots))
	var lines []string
	for i := off; i < min(len(rows), off+slots); i++ {
		l := " " + rows[i].text
		if i == o.pcur {
			l = selRow(l, w-4, true)
		}
		lines = append(lines, l)
	}
	title := m.logTitle(o)
	if len(rows) > slots {
		title += fmt.Sprintf(" · %d%%", (off+slots)*100/len(rows))
	}
	return box(title, lines, w, h, true)
}

func (m *model) logKey(o *overlay, k string) tea.Cmd {
	rows := o.logRows()
	if len(rows) == 0 {
		return nil
	}
	o.pcur = max(0, min(o.pcur, len(rows)-1))
	step := rows[o.pcur].step
	sets := func(v bool) {
		if o.open == nil {
			o.open = map[int]bool{}
		}
		o.open[step] = v
		for i, r := range o.logRows() {
			if r.hdr && r.step == step {
				o.pcur = i
			}
		}
	}
	steps := runSteps(o.text, runOf(o.text, o.title))
	switch k {
	case "esc", "backspace", "q":
		m.pop()
	case "Z":
		o.max = !o.max
	case "enter":
		sets(!o.stepOpen(step, steps[step]))
	case "l", "right":
		sets(true)
	case "h", "left":
		sets(false)
	case "R":
		return m.rerun(o.text, o.title)
	default:
		if d := moveDelta(k); d != 0 {
			o.pcur = max(0, min(o.pcur+d, len(rows)-1))
		}
	}
	return nil
}

// rerun asks to re-run a failed check, then shows it pending everywhere.
func (m *model) rerun(sha, label string) tea.Cmd {
	r := runOf(sha, label)
	if r.state != "fail" {
		return m.say("R re-runs a failed check; " + label + " is " + stateWord(r.state))
	}
	m.confirmDo("Re-run "+label+" on "+sha+"? ", "(prototype) re-running "+label, func(m *model) tea.Cmd {
		rerunning[runKey(sha, label)] = true
		if n := len(m.stack); n > 0 && m.stack[n-1].kind == "log" {
			m.stack[n-1].open = nil
		}
		return nil
	})
	return nil
}
