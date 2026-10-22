package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

var (
	tabNames = []string{"All", "Failing", "Running", "Mine"}
	groups   = []string{"workflow", "event", "branch", "status", "actor", "none"}
	sorts    = []string{"newest", "oldest", "longest", "failing first"}
	facets   = []string{"status", "branch", "event", "actor", "workflow"}
)

func (m *model) defs() []panes.Def {
	return []panes.Def{{Name: "Runs", Short: "Runs"}, {Name: "Annotations", Short: "Annot."}}
}

func (r run) facet(f string) string {
	switch f {
	case "status":
		return r.status
	case "branch":
		return r.branch
	case "event":
		return r.event
	case "actor":
		return r.actor
	}
	return r.workflow
}

// facetValues is "any" and then each value the runs have, sorted.
func facetValues(f string) []string {
	var vals []string
	for _, r := range allRuns {
		if v := r.facet(f); !slices.Contains(vals, v) {
			vals = append(vals, v)
		}
	}
	slices.Sort(vals)
	return append([]string{"any"}, vals...)
}

func (m *model) passes(r run) bool {
	if m.fltOff {
		return true
	}
	for f, v := range m.flt {
		if len(v) > 0 && !slices.Contains(v, r.facet(f)) {
			return false
		}
	}
	return true
}

func (m *model) grouped() string {
	if m.groupOff || m.groupBy == "none" {
		return ""
	}
	return m.groupBy
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
			if m.match(i, r) && m.passes(r) {
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
	kHead = iota
	kRun
	kGroup
	kJob
)

// node is one selectable row of the list.
type node struct {
	kind  int
	run   int // index in allRuns
	job   int
	group string // header value, or matrix parent name
	cnt   int
}

func (m *model) sortRuns(runs []int) {
	rank := func(s string) int {
		switch s {
		case "failure":
			return 0
		case "running":
			return 1
		}
		return 2
	}
	slices.SortStableFunc(runs, func(a, b int) int {
		ra, rb := allRuns[a], allRuns[b]
		if m.sortRev {
			ra, rb = rb, ra
		}
		switch m.sortBy {
		case "oldest":
			return rb.id - ra.id
		case "longest":
			return rb.secs() - ra.secs()
		case "failing first":
			return rank(ra.status) - rank(rb.status)
		}
		return ra.id - rb.id
	})
}

func (m *model) nodes() []node {
	var runs []int
	for i, r := range allRuns {
		if m.match(m.tab, r) && m.passes(r) {
			runs = append(runs, i)
		}
	}
	m.sortRuns(runs)
	var out []node
	add := func(ri int) {
		r := allRuns[ri]
		out = append(out, node{kind: kRun, run: ri})
		if !m.openRun[r.id] {
			return
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
	g := m.grouped()
	if g == "" {
		for _, ri := range runs {
			add(ri)
		}
		return out
	}
	var order []string
	by := map[string][]int{}
	for _, ri := range runs {
		v := allRuns[ri].facet(g)
		if _, ok := by[v]; !ok {
			order = append(order, v)
		}
		by[v] = append(by[v], ri)
	}
	for _, v := range order {
		out = append(out, node{kind: kHead, group: v, cnt: len(by[v]), run: by[v][0]})
		if m.collapsed[g+"/"+v] {
			continue
		}
		for _, ri := range by[v] {
			add(ri)
		}
	}
	return out
}

// toggleAll collapses every header, or opens them all when all are collapsed.
func (m *model) toggleAll() {
	g := m.grouped()
	if g == "" {
		return
	}
	open := false
	for _, n := range m.nodes() {
		if n.kind == kHead && !m.collapsed[g+"/"+n.group] {
			open = true
		}
	}
	for _, v := range facetValues(g)[1:] {
		if open {
			m.collapsed[g+"/"+v] = true
		} else {
			delete(m.collapsed, g+"/"+v)
		}
	}
	m.sel = 0
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

// ctx is the run under the cursor and the job the cursor is on (-1 for a run row).
func (m *model) ctx() (ri, pref int, ok bool) {
	ns := m.nodes()
	if m.sel < 0 || m.sel >= len(ns) || ns[m.sel].kind == kHead {
		return 0, -1, false
	}
	n := ns[m.sel]
	switch n.kind {
	case kJob:
		return n.run, n.job, true
	case kGroup:
		for ji, j := range allRuns[n.run].jobs {
			if j.group == n.group {
				return n.run, ji, true
			}
		}
	}
	return n.run, -1, true
}

// selJob is the job the preview shows: the cursor's, else the failing one.
func (m *model) selJob() (run, int, bool) {
	ri, pref, ok := m.ctx()
	if !ok {
		return run{}, 0, false
	}
	r := allRuns[ri]
	if ns := m.nodes(); ns[m.sel].kind == kGroup {
		for ji, j := range r.jobs {
			if j.group == ns[m.sel].group && j.status == "failure" {
				return r, ji, true
			}
		}
	}
	if pref >= 0 {
		return r, pref, true
	}
	if fj, _ := r.failing(); fj >= 0 {
		return r, fj, true
	}
	return r, len(r.jobs) - 1, true
}

type annoRow struct {
	annotation
	job int
}

// annos are the annotations of the run under the cursor, the cursor's job first.
func (m *model) annos() []annoRow {
	ri, pref, ok := m.ctx()
	if !ok {
		return nil
	}
	r := allRuns[ri]
	order := []int{}
	if pref >= 0 {
		order = append(order, pref)
	}
	for ji := range r.jobs {
		if ji != pref {
			order = append(order, ji)
		}
	}
	var out []annoRow
	for _, ji := range order {
		for _, a := range r.annotations(r.jobs[ji]) {
			out = append(out, annoRow{a, ji})
		}
	}
	return out
}

// sync keeps the cursor in range and resets the right pane's rows when the
// selection moves to another run or job.
func (m *model) sync() {
	ns := m.nodes()
	m.sel = max(0, min(len(ns)-1, m.sel))
	ri, pref, ok := m.ctx()
	k := fmt.Sprint(ri, "/", pref, ok)
	if k != m.selKey {
		m.selKey = k
		m.asel, m.dsel = 0, 0
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
	c := panes.Content{TopLeft: m.titleForms(title, b), Focus: b.Pane == l.Focus}
	in := b.Rect.Inner()
	if m.view == 2 {
		r := allRuns[m.logRun]
		c.TopLeft = []string{"Workflow " + r.workflowPath(), "Workflow " + r.workflow + ".yml", "Workflow"}
		c.TopRight = []string{r.workflow + " #" + fmt.Sprint(r.id), ""}
		c.BottomLeft = []string{"j k scroll  / search  read-only  backspace back", "/ backspace", ""}
		m.pg.SetSize(in.W, in.H)
		c.Body = m.pg.View()
		return c
	}
	if m.view == 1 {
		r := allRuns[m.logRun]
		j := r.jobs[m.logJob]
		at := ""
		if n := attempts(r); n > 1 {
			at = fmt.Sprintf("  attempt %d/%d", m.attempt(m.logRun), n)
		}
		c.TopLeft = []string{"Log " + j.name + at, "Log " + j.name, "Log"}
		id := r.title + " #" + fmt.Sprint(r.id)
		c.TopRight = []string{id + "  " + r.event + "  " + r.branch + "  " + r.actor + "  " + r.sha() + "  " + r.dur,
			id + "  " + r.branch + "  " + r.sha(), id, ""}
		c.BottomLeft = []string{"enter fold  x raw  ] [ step  :n :p attempt  backspace back", "x ] [ :n :p", ""}
		m.lv.SetSize(in.W, in.H)
		c.Body = m.lv.View()
		return c
	}
	if b.Pane == 1 {
		ns := m.nodes()
		runs := 0
		for _, n := range ns {
			if n.kind == kRun {
				runs++
			}
		}
		by := "no groups"
		if g := m.grouped(); g != "" {
			by = "by " + g
		}
		meta := fmt.Sprintf("%d runs, %s, %s", runs, by, m.sortBy)
		if f := len(m.flt); f > 0 {
			meta += fmt.Sprintf(", %d filters", f)
			if m.fltOff {
				meta += " (off)"
			}
		}
		if m.sortRev {
			meta += " reversed"
		}
		c.TopRight = []string{meta, fmt.Sprintf("%d runs", runs), ""}
		if len(ns) > 0 {
			c.BottomRight = []string{fmt.Sprintf("%d/%d", m.sel+1, len(ns))}
		}
		c.BottomLeft = []string{"enter open  g group  f filter  s sort  F clear", "g f s F", ""}
		c.Body = m.treeBody(ns, in.W, in.H, c.Focus)
		return c
	}
	as := m.annos()
	c.TopRight = []string{fmt.Sprintf("%d", len(as)), ""}
	if m.graph {
		c.TopLeft = []string{l.Title(2) + " (graph)", "graph"}
		if ri, _, ok := m.ctx(); ok {
			r := allRuns[ri]
			c.TopRight = []string{r.workflow + " #" + fmt.Sprint(r.id)}
			c.Body = m.graphBody(r, in.W, in.H)
		} else {
			c.Body = m.th.Subtle.Render("Select a run to draw its graph")
		}
		c.BottomLeft = []string{"arrows scroll  l LR/TD  G close", "arrows l G", ""}
		return c
	}
	if m.preview && m.w >= wideMin && !m.zoom.Zoomed() {
		c.TopLeft = []string{"Log preview"}
		r, ji, ok := m.selJob()
		if !ok {
			c.Body = m.th.Subtle.Render("Select a run or a job to preview its log")
			return c
		}
		rows := r.logRows(r.jobs[ji])
		_, fs := r.failing()
		if r.jobs[ji].status != "failure" {
			fs = 0
		}
		sel := markerRow(rows, fs)
		c.TopRight = []string{r.jobs[ji].name}
		c.BottomLeft = []string{"enter open the log  z close preview", "z", ""}
		c.Body = m.logBody(rows, sel, in.W, in.H, false)
		return c
	}
	c.BottomLeft = []string{"enter open log  v preview  G graph", "enter v G", ""}
	c.Body = m.annoBody(as, in.W, in.H, c.Focus)
	return c
}

// depthOf is the outline depth of a tree row; base is 1 when grouped.
func depthOf(n node, base int) int {
	switch n.kind {
	case kHead:
		return 0
	case kRun:
		return base
	case kGroup:
		return base + 1
	}
	if n.group != "" {
		return base + 2
	}
	return base + 1
}

// runsWidth is the width of the Runs pane: wide enough for the longest job,
// matrix parent or group name with its indent and suffix, within a cap.
// Only run labels give way beyond that.
func (m *model) runsWidth() int {
	base := 0
	if m.grouped() != "" {
		base = 1
	}
	need := 0
	for _, n := range m.nodes() {
		r := allRuns[n.run]
		var nm, suf string
		switch n.kind {
		case kHead:
			nm, suf = n.group, fmt.Sprint(n.cnt)
		case kGroup:
			_, pass, total := r.rollup(n.group)
			nm, suf = n.group, fmt.Sprintf("%d/%d passed", pass, total)
		case kJob:
			nm, suf = r.jobs[n.job].name, r.jobs[n.job].dur
		default:
			continue
		}
		// border and padding, cursor, guide, fold mark, glyph, gap.
		need = max(need, 4+2+3*depthOf(n, base)+4+ansi.StringWidth(nm)+1+ansi.StringWidth(suf))
	}
	return max(30, min(need, max(34, m.w*45/100)))
}

// arts is the downloads of the run in view.
func (m *model) arts() []artifact {
	if ri, _, ok := m.ctx(); ok {
		return allRuns[ri].artifacts()
	}
	return nil
}

func (m *model) treeBody(ns []node, w, h int, focus bool) string {
	if len(ns) == 0 {
		return m.th.Subtle.Render("No runs match. F clears the filter.")
	}
	g := m.grouped()
	base := 0
	if g != "" {
		base = 1
	}
	depth := func(n node) int { return depthOf(n, base) }
	// guide is the outline of row i: a dim line per ancestor that has more
	// rows below it, then a branch, or a corner for the last child.
	guide := func(i int) string {
		d := depth(ns[i])
		if d == 0 {
			return ""
		}
		last := func(lv int) bool {
			for _, n := range ns[i+1:] {
				if dd := depth(n); dd < lv {
					return true
				} else if dd == lv {
					return false
				}
			}
			return true
		}
		var sb strings.Builder
		for lv := 1; lv < d; lv++ {
			if last(lv) {
				sb.WriteString("   ")
			} else {
				sb.WriteString("│  ")
			}
		}
		if last(d) {
			sb.WriteString("╰─ ")
		} else {
			sb.WriteString("├─ ")
		}
		return m.th.Subtle.Render(sb.String())
	}
	var lines []string
	selLine := 0
	for i, n := range ns {
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
		case kHead:
			mark := m.ic.Unfolded
			if m.collapsed[g+"/"+n.group] {
				mark = m.ic.Folded
			}
			nm := m.th.Accent.Bold(true).Render(n.group)
			if sel && focus {
				nm = m.th.Title.Render(n.group)
			}
			lines = append(lines, m.row(cur+m.th.Subtle.Render(mark)+" "+nm, m.th.Subtle.Render(fmt.Sprint(n.cnt)), w))
		case kRun:
			mark := m.ic.Folded
			if m.openRun[r.id] {
				mark = m.ic.Unfolded
			}
			// The rest of the run's attributes show in Annotations.
			suffix := r.dur
			if r.status == "running" {
				suffix = fmt.Sprintf("%d/%d", r.doneJobs(), len(r.jobs))
			}
			// Only the run's label gives way: it gets what the indent, glyph
			// and suffix leave.
			prefix := cur + guide(i) + m.th.Subtle.Render(mark) + " " + m.glyph(r.status) + " "
			avail := w - ansi.StringWidth(prefix) - ansi.StringWidth(suffix) - 1
			title := ansi.Truncate(r.title, max(avail, 1), m.ic.Ellipsis)
			left := prefix + name(title, m.th.Text.Render(title))
			lines = append(lines, m.row(left, m.th.Subtle.Render(suffix), w))
		case kGroup:
			st, pass, total := r.rollup(n.group)
			mark := m.ic.Folded
			if m.openGrp[fmt.Sprint(r.id, "/", n.group)] {
				mark = m.ic.Unfolded
			}
			lines = append(lines, m.row(cur+guide(i)+m.th.Subtle.Render(mark)+" "+m.glyph(st)+" "+name(n.group, m.th.Text.Render(n.group)), m.th.Subtle.Render(fmt.Sprintf("%d/%d passed", pass, total)), w))
		default:
			j := r.jobs[n.job]
			lines = append(lines, m.row(cur+guide(i)+m.glyph(j.status)+" "+name(j.name, m.th.Text.Render(j.name)), m.th.Subtle.Render(j.dur), w))
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
	}
	return m.th.Text.Render(rest)
}

// logBody draws the timestamped log with a marker row per step.
func (m *model) logBody(rows []logRow, sel, w, h int, focus bool) string {
	var lines []string
	for i, r := range rows {
		cur := m.cursor(i == sel, focus || true)
		if !focus {
			cur = m.cursor(i == sel, false)
		}
		if r.marker {
			nm := m.th.Accent.Bold(true).Render(r.text)
			if r.status == "failure" {
				nm = m.th.Error.Bold(true).Render(r.text)
			}
			lines = append(lines, m.row(cur+m.glyph(r.status)+" "+nm, m.th.Subtle.Render(r.dur), w))
			continue
		}
		lines = append(lines, ansi.Truncate(cur+m.th.Subtle.Render(r.ts)+" "+m.styleLine(r.text), w, m.ic.Ellipsis))
	}
	if len(lines) == 0 {
		return m.th.Subtle.Render("No log")
	}
	return strings.Join(window(lines, sel, h), "\n")
}

// annoBody lists the annotations of the run in view.
func (m *model) annoBody(as []annoRow, w, h int, focus bool) string {
	ri, _, ok := m.ctx()
	if !ok {
		return m.th.Subtle.Render("Select a run to see its annotations")
	}
	if len(as) == 0 {
		head := m.attrLines(ri, w)
		return strings.Join(append(head, m.th.Subtle.Render("No annotations.")), "\n")
	}
	r := allRuns[ri]
	head := m.attrLines(ri, w)
	h = max(h-len(head), 1)
	var out []string
	for i, a := range as {
		g := m.th.Accent.Render("i")
		switch a.level {
		case "error":
			g = m.glyph("failure")
		case "warning":
			g = m.th.Warning.Render("!")
		}
		sel := i == m.asel
		loc := m.th.Muted.Render(fmt.Sprintf("%s:%d", a.file, a.line))
		msg := m.th.Text.Render(a.msg)
		if sel && focus {
			msg = m.th.Title.Render(a.msg)
		}
		out = append(out, m.row(m.cursor(sel, focus)+g+" "+loc+"  "+msg, m.th.Subtle.Render(r.jobs[a.job].name), w))
	}
	return strings.Join(append(head, window(out, min(m.asel, len(as)-1), h)...), "\n")
}

// attrLines are the attributes of run ri, as dim label:value lines.
func (m *model) attrLines(ri, w int) []string {
	r := allRuns[ri]
	kv := func(pairs ...string) string {
		var parts []string
		for i := 0; i+1 < len(pairs); i += 2 {
			parts = append(parts, m.th.Subtle.Render(pairs[i]+":")+m.th.Muted.Render(pairs[i+1]))
		}
		return ansi.Truncate(strings.Join(parts, "  "), w, m.ic.Ellipsis)
	}
	att := "1/1"
	if n := attempts(r); n > 0 {
		att = fmt.Sprintf("%d/%d", m.attempt(ri), n)
	}
	dur := r.dur
	if dur == "" {
		dur = "-"
	}
	return []string{
		kv("attempt", att, "event", r.event),
		kv("branch", r.branch, "actor", r.actor),
		kv("started", r.age+" ago", "duration", dur),
		kv("sha", r.sha()),
	}
}

// dlBody lists the artifacts in the downloads popup, with a mark column.
func (m *model) dlBody(ar []artifact, w, h int) string {
	var out []string
	for i, a := range ar {
		mark := "[ ] "
		if m.dpick[i] {
			mark = "[x] "
		}
		nm := m.th.Text.Render(mark + a.name)
		if i == m.dsel {
			nm = m.th.Title.Render(mark + a.name)
		}
		out = append(out, m.row(m.cursor(i == m.dsel, true)+nm, m.th.Muted.Render(a.size)+"  "+m.th.Subtle.Render("expires "+a.expiry), w))
	}
	return strings.Join(window(out, m.dsel, h), "\n")
}
