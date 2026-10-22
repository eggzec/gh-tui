package main

import (
	"fmt"
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

var variantNames = []string{"Runs | Jobs | Log", "Runs | Preview", "Grouped runs | Preview"}

func (m *model) defs() []panes.Def {
	switch m.variant {
	case 1:
		return []panes.Def{{Name: "Runs", Short: "Runs"}, {Name: "Jobs", Short: "Jobs"}, {Name: "Log", Short: "Log"}}
	}
	return []panes.Def{{Name: "Runs", Short: "Runs"}, {Name: "Preview", Short: "Prev"}}
}

func (m *model) tabs() []string {
	if m.variant == 3 {
		return []string{"All", "Failing", "Running", "Queued", "Done"}
	}
	return []string{"All", "Failing", "Running", "Mine"}
}

func (m *model) match(tab int, r run) bool {
	if tab == 0 {
		return true
	}
	switch m.tabs()[tab] {
	case "Failing":
		return r.status == "failure"
	case "Running":
		return r.status == "running" || (m.variant != 3 && r.status == "queued")
	case "Queued":
		return r.status == "queued"
	case "Done":
		return r.status == "success"
	case "Mine":
		return strings.HasPrefix(r.branch, "feat/") || strings.HasPrefix(r.branch, "fix/")
	}
	return false
}

func (m *model) tabCounts() []int {
	out := make([]int, len(m.tabs()))
	for i := range out {
		for _, r := range allRuns {
			if m.match(i, r) {
				out[i]++
			}
		}
	}
	return out
}

func (m *model) vis() []run {
	var out []run
	for _, r := range allRuns {
		if m.match(m.tab, r) {
			out = append(out, r)
		}
	}
	if m.variant == 3 {
		slices.SortStableFunc(out, func(a, b run) int {
			return slices.Index(workflows, a.workflow) - slices.Index(workflows, b.workflow)
		})
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

// content is what a box shows: the title with its digit (without it when the
// pane is zoomed, since the strip has the digits), a count and the body.
func (m *model) content(b panes.Box, l panes.Layout) panes.Content {
	title := l.Title(b.Pane)
	if l.Zoomed {
		title = l.Panes[b.Pane-1].Name
	}
	c := panes.Content{TopLeft: m.titleForms(title, b), Focus: b.Pane == l.Focus}
	in := b.Rect.Inner()
	r, ok := m.cur()
	switch {
	case b.Pane == 1:
		v := m.vis()
		c.TopRight = []string{fmt.Sprintf("%d runs", len(v))}
		if len(v) > 0 {
			c.BottomRight = []string{fmt.Sprintf("%d/%d", m.sel+1, len(v))}
		}
		c.Body = m.runsBody(v, in.W, in.H, c.Focus)
	case !ok:
		c.Body = m.th.Subtle.Render("No run selected")
	case m.variant == 1 && b.Pane == 2:
		c.TopRight = []string{fmt.Sprintf("%d jobs", len(r.jobs))}
		c.Body = m.jobsBody(r, in.W, in.H, c.Focus)
	case m.variant == 1:
		j := r.jobs[m.job]
		c.TopRight = []string{j.name}
		c.Body = m.logBody(r, j, in.H)
	default:
		c.TopRight = []string{fmt.Sprintf("#%d", r.id)}
		c.Body = m.previewBody(r, in.W, in.H)
	}
	return c
}

func (m *model) runsBody(v []run, w, h int, focus bool) string {
	var lines []string
	selLine := 0
	last := ""
	for i, r := range v {
		if m.variant == 3 && r.workflow != last {
			last = r.workflow
			n := 0
			for _, x := range v {
				if x.workflow == last {
					n++
				}
			}
			lines = append(lines, m.th.Accent.Bold(true).Render(m.ic.Unfolded+" "+last)+" "+m.th.Subtle.Render(fmt.Sprint(n)))
		}
		if i == m.sel {
			selLine = len(lines)
		}
		title := m.th.Text.Render(r.title)
		if i == m.sel && focus {
			title = m.th.Title.Render(r.title)
		}
		meta := m.th.Subtle.Render(r.age)
		if w >= 44 {
			meta = m.th.Muted.Render(r.branch) + " " + meta
		}
		pre := m.cursor(i == m.sel, focus) + m.glyph(r.status) + " "
		if m.variant == 3 {
			pre = " " + pre
		}
		lines = append(lines, m.row(pre+title, meta, w))
	}
	if len(v) == 0 {
		return m.th.Subtle.Render("No runs match this tab")
	}
	return strings.Join(window(lines, selLine, h), "\n")
}

func (m *model) jobsBody(r run, w, h int, focus bool) string {
	var lines []string
	selLine := 0
	for ji, j := range r.jobs {
		mark := m.ic.Folded
		if ji == m.job {
			mark = m.ic.Unfolded
			selLine = len(lines)
		}
		name := m.th.Text.Render(j.name)
		if ji == m.job && focus {
			name = m.th.Title.Render(j.name)
		}
		lines = append(lines, m.row(m.cursor(ji == m.job, focus)+m.th.Subtle.Render(mark)+" "+m.glyph(j.status)+" "+name, m.th.Subtle.Render(j.dur), w))
		if ji != m.job {
			continue
		}
		for _, s := range j.steps {
			lines = append(lines, m.row("    "+m.glyph(s.status)+" "+m.th.Muted.Render(s.name), m.th.Subtle.Render(s.dur), w))
		}
	}
	return strings.Join(window(lines, selLine, h), "\n")
}

func (m *model) logLine(s string) string {
	ts, rest, _ := strings.Cut(strings.ReplaceAll(s, "\t", "  "), " ")
	switch {
	case strings.HasPrefix(rest, "##[error]"):
		rest = m.th.Error.Render(rest)
	case strings.HasPrefix(rest, "##[warning]"):
		rest = m.th.Warning.Render(rest)
	case strings.HasPrefix(rest, "##[group]"):
		rest = m.th.Accent.Render(rest)
	default:
		rest = m.th.Text.Render(rest)
	}
	return m.th.Subtle.Render(ts) + " " + rest
}

func (m *model) logBody(r run, j job, h int) string {
	lg := logFor(r, j)
	end := len(lg) - min(m.logOff, max(0, len(lg)-h))
	start := max(0, end-h)
	var out []string
	for _, s := range lg[start:end] {
		out = append(out, m.logLine(s))
	}
	return strings.Join(out, "\n")
}

// previewBody is the run's jobs and the log tail of the failing step.
func (m *model) previewBody(r run, w, h int) string {
	lines := []string{
		m.row(m.th.Title.Render(r.title), m.glyph(r.status), w),
		m.th.Muted.Render(strings.Join([]string{r.workflow, r.branch, r.dur, r.age}, m.ic.Separator)),
		"",
		m.th.Accent.Render("Jobs"),
	}
	fj, fs := r.failing()
	for ji, j := range r.jobs {
		lines = append(lines, m.row(m.glyph(j.status)+" "+m.th.Text.Render(j.name), m.th.Subtle.Render(j.dur), w))
		if ji == fj {
			s := j.steps[fs]
			lines = append(lines, m.row("  "+m.th.Subtle.Render(m.ic.Border.BottomLeft)+" "+m.glyph(s.status)+" "+m.th.Error.Render(s.name), m.th.Subtle.Render(s.dur), w))
		}
	}
	j := r.jobs[len(r.jobs)-1]
	head := "Log"
	if fj >= 0 {
		j = r.jobs[fj]
		head = "Log" + m.ic.Separator + j.name + m.ic.Separator + j.steps[fs].name
	}
	lines = append(lines, "", m.th.Accent.Render(head))
	room := max(0, h-len(lines))
	lg := logFor(r, j)
	tail := lg[len(lg)-min(room, len(lg)):]
	for _, s := range tail {
		lines = append(lines, m.logLine(s))
	}
	// A scroll moves the whole preview up, so the jobs can be read on small screens.
	if m.prevOff > 0 && m.prevOff < len(lines) {
		lines = lines[m.prevOff:]
	}
	return strings.Join(lines, "\n")
}
