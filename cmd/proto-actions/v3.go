package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// Variant 3: triage list. Tries to improve narrow terminals and "what needs
// me": a status chip strip on top (tab cycles it), one full-width list sorted
// by attention (failed, running, queued, rest), and no side pane: space
// expands a run in place to show its jobs and the first error line.

func (m *model) v3key(k string) {
	r, ok := m.selRun()
	switch k {
	case "tab":
		m.chip = (m.chip + 1) % len(chips)
		m.sel = 0
		return
	case "shift+tab":
		m.chip = (m.chip + len(chips) - 1) % len(chips)
		m.sel = 0
		return
	}
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
	case "space", "enter":
		m.exp[r.id] = !m.exp[r.id]
	case "l":
		ji, _ := r.failing()
		m.openLog(max(0, ji))
	}
}

func (m *model) v3View(w, h int) string {
	count := map[string]int{}
	for _, r := range allRuns {
		count[r.status]++
	}
	var strip []string
	for i, c := range chips {
		s := c
		if c == "all" {
			s = fmt.Sprintf("all %d", len(allRuns))
		} else {
			s = fmt.Sprintf("%s %s %d", glyph(c), c, count[c])
		}
		st := lipgloss.NewStyle()
		if c != "all" {
			st = st.Foreground(m.th.statusCol(c))
		}
		if i == m.chip {
			st = st.Bold(true).Reverse(true)
		}
		strip = append(strip, st.Render(" "+s+" "))
	}
	head := strings.Join(strip, " ")
	r, ok := m.selRun()
	if !ok {
		return block([]string{head, "", m.dim("  no runs match")}, w, h)
	}
	if m.logJob >= 0 {
		return m.logView(r, r.jobs[m.logJob], w, h)
	}
	runs := m.vis()
	var lines []string
	selLine := 0
	for i, x := range runs {
		sel := i == m.sel
		if sel {
			selLine = len(lines)
		}
		left := m.glyph(sel, x.status) + " " + m.paint(sel, m.th.dim, fmt.Sprintf("#%d", x.id)) + " " + x.title
		var cols []string
		if b := m.bar(x, sel); b != "" {
			cols = append(cols, b)
		}
		if w >= 90 {
			cols = append(cols, m.paint(sel, m.th.blue, fit(x.branch, 14)))
		}
		cols = append(cols, m.paint(sel, m.th.dim, fmt.Sprintf("%6s", x.dur)), m.paint(sel, m.th.dim, fmt.Sprintf("%3s", x.age)))
		row := rightAlign(left, strings.Join(cols, " "), w)
		if sel {
			row = m.selStyle(row, w)
		}
		lines = append(lines, row)
		if !m.exp[x.id] {
			continue
		}
		fj, fs := x.failing()
		for ji, j := range x.jobs {
			l := "    " + m.glyph(false, j.status) + " " + j.name
			if ji == fj {
				l += m.th.col(m.th.red, " › "+j.steps[fs].name)
			}
			lines = append(lines, rightAlign(l, m.dim(j.dur), w))
			if ji == fj {
				for _, ll := range logFor(x, j) {
					if _, rest, _ := strings.Cut(ll, " "); strings.Contains(rest, "##[error]") {
						lines = append(lines, "      "+lipgloss.NewStyle().Foreground(m.th.red).Render(rest))
						break
					}
				}
			}
		}
	}
	bh := h - 2
	lines = lines[window(selLine, len(lines), bh):]
	return block(append([]string{head, ""}, lines...), w, h)
}
