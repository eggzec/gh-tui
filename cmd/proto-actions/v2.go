package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Variant 2: one list grouped by workflow, a row per run with columns, and a
// preview on the right of the run's jobs plus the tail of the failing log.

func (m *model) v2key(k string) {
	r, ok := m.selRun()
	if !ok {
		return
	}
	switch k {
	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "g":
		m.sel = 0
	case "G":
		m.move(1 << 20)
	case "enter", "l":
		ji, _ := r.failing()
		m.openLog(max(0, ji))
	}
}

func (m *model) v2Row(r run, w int, sel bool, wide bool) string {
	left := m.glyph(sel, r.status) + " " + r.title
	var cols []string
	if b := m.bar(r, sel); b != "" {
		cols = append(cols, b)
	}
	if wide {
		cols = append(cols, m.paint(sel, m.th.blue, fit(r.branch, 14)))
	}
	cols = append(cols, m.paint(sel, m.th.dim, fmt.Sprintf("%6s", r.dur)), m.paint(sel, m.th.dim, fmt.Sprintf("%3s", r.age)))
	s := rightAlign(left, strings.Join(cols, " "), w)
	if sel {
		return m.selStyle(s, w)
	}
	return s
}

func (m *model) v2View(w, h int) string {
	r, ok := m.selRun()
	if !ok {
		return block([]string{"", m.dim("  no runs match")}, w, h)
	}
	if m.logJob >= 0 {
		return m.logView(r, r.jobs[m.logJob], w, h)
	}
	runs := m.vis()
	lw := w * 11 / 20
	if w >= 100 {
		lw = w * 5 / 8
	}
	rw := w - lw - 1
	var left []string
	selLine, last := 0, ""
	for i, x := range runs {
		if x.workflow != last {
			last = x.workflow
			n := 0
			for _, y := range runs {
				if y.workflow == last {
					n++
				}
			}
			left = append(left, lipgloss.NewStyle().Bold(true).Foreground(m.th.accent).Render(fmt.Sprintf("%s (%d)", last, n)))
		}
		if i == m.sel {
			selLine = len(left)
		}
		left = append(left, m.v2Row(x, lw, i == m.sel, lw >= 60))
	}
	left = left[window(selLine, len(left), h):]

	right := []string{
		lipgloss.NewStyle().Bold(true).Render(r.title),
		m.dim(fmt.Sprintf("%s · %s · %s", r.branch, r.status, r.dur)),
		"",
	}
	for _, j := range r.jobs {
		right = append(right, rightAlign(m.glyph(false, j.status)+" "+j.name, m.dim(j.dur), rw))
	}
	right = append(right, "")
	if ji, si := r.failing(); ji >= 0 {
		j := r.jobs[ji]
		right = append(right, m.th.col(m.th.red, "✗ "+j.steps[si].name+" · "+j.name))
		lg := logFor(r, j)
		for _, l := range lg[len(lg)-8:] {
			_, rest, _ := strings.Cut(l, " ")
			if strings.Contains(rest, "##[error]") {
				rest = lipgloss.NewStyle().Bold(true).Foreground(m.th.red).Render(rest)
			} else {
				rest = m.dim(rest)
			}
			right = append(right, rest)
		}
	} else if r.status == "running" {
		right = append(right, m.th.col(m.th.yellow, fmt.Sprintf("%s %d/%d jobs done", m.bar(r, false), r.doneJobs(), len(r.jobs))))
	} else {
		right = append(right, m.dim("no failures"))
	}
	sep := strings.TrimSuffix(strings.Repeat(m.dim("│")+"\n", h), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, block(left, lw, h), sep, block(right, rw, h))
}
