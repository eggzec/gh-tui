package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Variant 1: master-detail. Runs on the left, the selected run's jobs with
// steps expanded as a tree on the right; enter on a job opens its log.

func (m *model) v1key(k string) {
	r, ok := m.selRun()
	if !ok {
		return
	}
	if m.v1focus == 0 {
		switch k {
		case "j", "down":
			m.move(1)
		case "k", "up":
			m.move(-1)
		case "g":
			m.sel = 0
		case "G":
			m.move(1 << 20)
		case "enter", "l", "right", "tab":
			m.v1focus, m.v1job = 1, 0
		}
		return
	}
	switch k {
	case "j", "down":
		m.v1job = min(m.v1job+1, len(r.jobs)-1)
	case "k", "up":
		m.v1job = max(0, m.v1job-1)
	case "enter", "l":
		m.openLog(m.v1job)
	case "h", "left", "backspace", "esc", "tab":
		m.v1focus = 0
	}
}

func (m *model) runRow(r run, w int, sel bool) string {
	l := m.glyph(sel, r.status) + " " + r.title
	right := m.paint(sel, m.th.dim, r.age)
	if b := m.bar(r, sel); b != "" {
		right = b + " " + right
	}
	s := rightAlign(l, right, w)
	if sel {
		return m.selStyle(s, w)
	}
	return s
}

func (m *model) v1View(w, h int) string {
	r, ok := m.selRun()
	if !ok {
		return block([]string{"", m.dim("  no runs match")}, w, h)
	}
	if m.logJob >= 0 {
		return m.logView(r, r.jobs[m.logJob], w, h)
	}
	runs := m.vis()
	lw := max(30, w*2/5)
	rw := w - lw - 1
	left := []string{m.title("Runs", fmt.Sprint(len(runs)), m.v1focus == 0)}
	start := window(m.sel, len(runs), h-1)
	for i := start; i < len(runs) && i < start+h-1; i++ {
		left = append(left, m.runRow(runs[i], lw, i == m.sel && m.v1focus == 0))
	}
	// A selected run keeps a quiet marker while the jobs pane has the focus.
	if m.v1focus == 1 {
		if i := m.sel - start + 1; i > 0 && i < len(left) {
			left[i] = lipgloss.NewStyle().Underline(true).Render(trunc(left[i], lw))
		}
	}

	right := []string{
		lipgloss.NewStyle().Bold(true).Render(r.title),
		m.dim(fmt.Sprintf("%s · %s · #%d · %s %s", r.workflow, r.branch, r.id, r.dur, r.status)),
		m.title("Jobs", fmt.Sprint(len(r.jobs)), m.v1focus == 1),
	}
	selLine := 0
	for ji, j := range r.jobs {
		sel := m.v1focus == 1 && ji == m.v1job
		jl := m.glyph(sel, j.status) + " " + j.name
		line := rightAlign(jl, m.paint(sel, m.th.dim, j.dur), rw)
		if sel {
			line = m.selStyle(line, rw)
			selLine = len(right)
		}
		right = append(right, line)
		for si, s := range j.steps {
			tree := "├"
			if si == len(j.steps)-1 {
				tree = "└"
			}
			l := "  " + m.dim(tree) + " " + glyph(s.status) + " " + s.name
			d := m.dim(s.dur)
			if s.status == "failure" {
				l = lipgloss.NewStyle().Bold(true).Foreground(m.th.red).Render("  "+tree+" "+glyph(s.status)+" "+s.name+"  ◀ failed")
				d = m.th.col(m.th.red, s.dur)
			} else {
				l = "  " + m.dim(tree) + " " + m.th.col(m.th.statusCol(s.status), glyph(s.status)) + " " + s.name
			}
			right = append(right, rightAlign(l, d, rw))
		}
	}
	rs := window(selLine, len(right), h)
	right = right[rs:]
	sep := strings.TrimSuffix(strings.Repeat(m.dim("│")+"\n", h), "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top, block(left, lw, h), sep, block(right, rw, h))
}
