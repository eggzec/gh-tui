package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

var tabNames = []string{"All", "Failing", "Running", "Mine"}

func (m *model) defs() []panes.Def {
	return []panes.Def{{Name: "Runs", Short: "Runs"}, {Name: "Detail", Short: "Detail"}}
}

func (m *model) match(tab int, r run) bool {
	switch tabNames[tab] {
	case "Failing":
		return r.status == "failure"
	case "Running":
		return r.status == "running" || r.status == "queued"
	case "Mine":
		return strings.HasPrefix(r.branch, "feat/") || strings.HasPrefix(r.branch, "fix/")
	}
	return true
}

func (m *model) tabCounts() []int {
	out := make([]int, len(tabNames))
	for i := range out {
		for _, r := range allRuns {
			if m.match(i, r) {
				out[i]++
			}
		}
	}
	return out
}

func state(status string) ui.RunState {
	switch status {
	case "success":
		return ui.RunSuccess
	case "failure":
		return ui.RunFailure
	case "running":
		return ui.RunInProgress
	case "queued":
		return ui.RunQueued
	case "cancelled":
		return ui.RunCancelled
	}
	return ui.RunSkipped
}

func (m *model) glyph(status string) string { return m.rs.Glyphs[state(status)] }

const (
	kRun = iota
	kGroup
	kJob
)

// node is one selectable row of the tree; pre are unselectable lines above it.
type node struct {
	kind  int
	run   int // index in allRuns
	job   int
	group string
	pre   []string
}

func (m *model) nodes() []node {
	var runs []int
	for i, r := range allRuns {
		if m.match(m.tab, r) {
			runs = append(runs, i)
		}
	}
	slices.SortStableFunc(runs, func(a, b int) int {
		return slices.Index(workflows, allRuns[a].workflow) - slices.Index(workflows, allRuns[b].workflow)
	})
	var out []node
	last := ""
	for _, ri := range runs {
		r := allRuns[ri]
		var pre []string
		if r.workflow != last {
			last = r.workflow
			n := 0
			for _, x := range runs {
				if allRuns[x].workflow == last {
					n++
				}
			}
			pre = []string{m.th.Accent.Bold(true).Render(last) + " " + m.th.Subtle.Render(fmt.Sprint(n))}
		}
		out = append(out, node{kind: kRun, run: ri, pre: pre})
		if !m.openRun[r.id] {
			continue
		}
		seen := map[string]bool{}
		for ji, j := range r.jobs {
			if j.group == "" {
				out = append(out, node{kind: kJob, run: ri, job: ji})
				continue
			}
			if !seen[j.group] {
				seen[j.group] = true
				out = append(out, node{kind: kGroup, run: ri, group: j.group})
			}
			if m.openGrp[fmt.Sprint(r.id, "/", j.group)] {
				out = append(out, node{kind: kJob, run: ri, job: ji, group: j.group})
			}
		}
	}
	return out
}

// stages gives each job its stage: 0 without needs, else one past its needs.
func (r run) stages() []int {
	key := func(j job) string {
		if j.group != "" {
			return j.group
		}
		return j.name
	}
	memo := map[string]int{}
	var st func(k string) int
	st = func(k string) int {
		if v, ok := memo[k]; ok {
			return v
		}
		memo[k] = 0
		v := 0
		for _, j := range r.jobs {
			if key(j) == k {
				for _, n := range j.needs {
					v = max(v, st(n)+1)
				}
				break
			}
		}
		memo[k] = v
		return v
	}
	out := make([]int, len(r.jobs))
	for i, j := range r.jobs {
		out[i] = st(key(j))
	}
	return out
}

// rollup is the status of a matrix parent and how many members passed.
func (r run) rollup(group string) (string, int, int) {
	pass, total := 0, 0
	has := map[string]bool{}
	for _, j := range r.jobs {
		if j.group != group {
			continue
		}
		total++
		has[j.status] = true
		if j.status == "success" {
			pass++
		}
	}
	for _, s := range []string{"failure", "running", "queued", "cancelled"} {
		if has[s] {
			return s, pass, total
		}
	}
	if pass == 0 {
		return "skipped", pass, total
	}
	return "success", pass, total
}

// selJob is the job the right pane shows for the selected node.
func (m *model) selJob() (run, int, bool) {
	ns := m.nodes()
	if m.sel < 0 || m.sel >= len(ns) {
		return run{}, 0, false
	}
	n := ns[m.sel]
	r := allRuns[n.run]
	switch n.kind {
	case kJob:
		return r, n.job, true
	case kGroup:
		first := -1
		for ji, j := range r.jobs {
			if j.group != n.group {
				continue
			}
			if first < 0 {
				first = ji
			}
			if j.status == "failure" {
				return r, ji, true
			}
		}
		return r, first, true
	}
	if fj, _ := r.failing(); fj >= 0 {
		return r, fj, true
	}
	return r, len(r.jobs) - 1, true
}

// sync resets the right pane when the selected job changed.
func (m *model) sync() {
	ns := m.nodes()
	m.sel = max(0, min(len(ns)-1, m.sel))
	r, ji, ok := m.selJob()
	if !ok {
		return
	}
	k := fmt.Sprint(r.id, "/", ji)
	if k == m.jobKey {
		return
	}
	m.jobKey = k
	m.raw = false
	m.lsel = [3]int{}
	m.rawSel = 59
	if _, fs := r.failing(); fs >= 0 && r.jobs[ji].status == "failure" {
		m.lsel[0] = fs
	}
}

func (m *model) selectFailing() {
	for i, n := range m.nodes() {
		if n.kind == kJob && allRuns[n.run].jobs[n.job].status == "failure" {
			m.sel = i
			return
		}
	}
}

func (m *model) content(b panes.Box, l panes.Layout) panes.Content {
	title := l.Title(b.Pane)
	if l.Zoomed {
		title = l.Panes[b.Pane-1].Name
	}
	if b.Pane == 2 && m.raw && m.sect == 0 {
		title += " raw"
	}
	c := panes.Content{TopLeft: m.titleForms(title, b), Focus: b.Pane == l.Focus}
	in := b.Rect.Inner()
	ns := m.nodes()
	if b.Pane == 1 {
		c.TopRight = []string{fmt.Sprintf("%d runs", countKind(ns, kRun))}
		if len(ns) > 0 {
			c.BottomRight = []string{fmt.Sprintf("%d/%d", m.sel+1, len(ns))}
		}
		c.BottomLeft = []string{"enter fold  g graph", "g graph"}
		c.Body = m.treeBody(ns, in.W, in.H, c.Focus)
		return c
	}
	r, ji, ok := m.selJob()
	if !ok {
		c.Body = m.th.Subtle.Render("No run selected")
		return c
	}
	if m.graph {
		c.TopLeft = []string{"graph"}
		c.TopRight = []string{r.workflow + " #" + fmt.Sprint(r.id)}
		c.BottomLeft = []string{"hjkl scroll  m mermaid  o LR/TD  g back", "m o g", ""}
		if m.gsrc {
			c.BottomLeft = []string{"hjkl scroll  m drawing  g back", "m g", ""}
			c.TopLeft = []string{"graph mermaid"}
		}
		c.Body = m.graphBody(r, in.W, in.H)
		return c
	}
	j := r.jobs[ji]
	c.TopRight = []string{j.name, ansi.Truncate(j.name, 14, m.ic.Ellipsis)}
	c.BottomLeft = []string{"l log  p problems  a artifacts  t raw", "l p a t", ""}
	if m.sect == 2 {
		c.BottomLeft = []string{"d download  D all  l p a", "d D", ""}
	}
	c.Body = m.detailBody(r, j, in.W, in.H, c.Focus)
	return c
}

func countKind(ns []node, k int) int {
	n := 0
	for _, x := range ns {
		if x.kind == k {
			n++
		}
	}
	return n
}

func (m *model) treeBody(ns []node, w, h int, focus bool) string {
	if len(ns) == 0 {
		return m.th.Subtle.Render("No runs match this tab")
	}
	var lines []string
	selLine := 0
	for i, n := range ns {
		lines = append(lines, n.pre...)
		sel := i == m.sel
		if sel {
			selLine = len(lines)
		}
		name := func(s string, st string) string {
			if sel && focus {
				return m.th.Title.Render(s)
			}
			return st
		}
		r := allRuns[n.run]
		cur := m.cursor(sel, focus)
		switch n.kind {
		case kRun:
			mark := m.ic.Folded
			if m.openRun[r.id] {
				mark = m.ic.Unfolded
			}
			meta := m.th.Subtle.Render(r.age)
			if w >= 50 {
				meta = m.th.Muted.Render(r.branch) + " " + meta
			}
			lines = append(lines, m.row(cur+m.th.Subtle.Render(mark)+" "+m.glyph(r.status)+" "+name(r.title, m.th.Text.Render(r.title)), meta, w))
		case kGroup:
			st, pass, total := r.rollup(n.group)
			mark := m.ic.Folded
			if m.openGrp[fmt.Sprint(r.id, "/", n.group)] {
				mark = m.ic.Unfolded
			}
			lines = append(lines, m.row(cur+"  "+m.th.Subtle.Render(mark)+" "+m.glyph(st)+" "+name(n.group, m.th.Text.Render(n.group)), m.th.Subtle.Render(fmt.Sprintf("%d/%d passed", pass, total)), w))
		default:
			j := r.jobs[n.job]
			ind := "  "
			if n.group != "" {
				ind = "    "
			}
			label := j.name
			left := cur + ind + "  " + m.glyph(j.status) + " " + name(label, m.th.Text.Render(label))
			lines = append(lines, m.row(left, m.th.Subtle.Render(j.dur), w))
		}
	}
	return strings.Join(window(lines, selLine, h), "\n")
}

// styleLine colours a log line by its workflow command.
func (m *model) styleLine(rest string) string {
	switch {
	case strings.HasPrefix(rest, "##[error]"):
		return m.th.Error.Render(rest)
	case strings.HasPrefix(rest, "##[warning]"):
		return m.th.Warning.Render(rest)
	case strings.HasPrefix(rest, "##[group]"):
		return m.th.Accent.Render(rest)
	}
	return m.th.Text.Render(rest)
}

func (m *model) logLine(s string) string {
	ts, rest, _ := strings.Cut(strings.ReplaceAll(s, "\t", "  "), " ")
	return m.th.Subtle.Render(ts) + " " + m.styleLine(rest)
}

func (m *model) sectionStrip(r run, j job) string {
	probs := len(r.annotations(j))
	arts := len(r.artifacts())
	labels := []string{"Log", "Problems", "Artifacts"}
	counts := []int{0, probs, arts}
	var parts []string
	for i, l := range labels {
		if counts[i] > 0 {
			l += fmt.Sprint(" ", counts[i])
		}
		if i == m.sect {
			parts = append(parts, m.th.Accent.Bold(true).Underline(true).Render(l))
		} else {
			parts = append(parts, m.th.Subtle.Render(l))
		}
	}
	return strings.Join(parts, "  ")
}

func (m *model) stepOpened(r run, ji, si int) bool {
	if v, ok := m.stepOpen[fmt.Sprint(r.id, "/", ji, "/", si)]; ok {
		return v
	}
	return r.jobs[ji].steps[si].status == "failure"
}

func (m *model) detailBody(r run, j job, w, h int, focus bool) string {
	ji := slices.IndexFunc(r.jobs, func(x job) bool { return x.name == j.name })
	head := []string{m.sectionStrip(r, j), ""}
	h = max(0, h-len(head))
	var lines []string
	selLine := 0
	switch m.sect {
	case 0:
		if m.raw {
			for _, s := range logFor(r, j) {
				lines = append(lines, m.logLine(s))
			}
			selLine = m.rawSel
			break
		}
		for si, s := range j.steps {
			open := m.stepOpened(r, ji, si)
			mark := m.ic.Folded
			if open {
				mark = m.ic.Unfolded
			}
			if si == m.lsel[0] {
				selLine = len(lines)
			}
			nm := m.th.Text.Render(s.name)
			if s.status == "failure" {
				nm = m.th.Error.Render(s.name)
			} else if si == m.lsel[0] && focus {
				nm = m.th.Title.Render(s.name)
			}
			lines = append(lines, m.row(m.cursor(si == m.lsel[0], focus)+m.th.Subtle.Render(mark)+" "+m.glyph(s.status)+" "+nm, m.th.Subtle.Render(s.dur), w))
			if open {
				for _, l := range r.stepLines(j, si) {
					lines = append(lines, "      "+m.styleLine(l))
				}
			}
		}
	case 1:
		as := r.annotations(j)
		if len(as) == 0 {
			return strings.Join(append(head, m.th.Subtle.Render("No problems in this job")), "\n")
		}
		for i, a := range as {
			g := m.th.Accent.Render("i")
			switch a.level {
			case "error":
				g = m.glyph("failure")
			case "warning":
				g = m.th.Warning.Render("!")
			}
			loc := fmt.Sprintf("%s:%d", a.file, a.line)
			if i == m.lsel[1] {
				selLine = i
			}
			l := m.cursor(i == m.lsel[1], focus) + g + " " + m.th.Muted.Render(loc) + "  " + m.th.Text.Render(a.msg)
			lines = append(lines, ansi.Truncate(l, w, m.ic.Ellipsis))
		}
	case 2:
		as := r.artifacts()
		if len(as) == 0 {
			return strings.Join(append(head, m.th.Subtle.Render("No artifacts for this run")), "\n")
		}
		for i, a := range as {
			if i == m.lsel[2] {
				selLine = i
			}
			nm := m.th.Text.Render(a.name)
			if i == m.lsel[2] && focus {
				nm = m.th.Title.Render(a.name)
			}
			lines = append(lines, m.row(m.cursor(i == m.lsel[2], focus)+nm, m.th.Muted.Render(a.size)+"  "+m.th.Subtle.Render("expires "+a.expiry), w))
		}
	}
	return strings.Join(append(head, window(lines, selLine, h)...), "\n")
}
