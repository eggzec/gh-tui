package tabrow

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// boxHeight is the number of lines of a boxed row.
const boxHeight = 3

// span is a run of tabs shown by a boxed row, with the labels it shows them
// by.
type span struct {
	labels []string
	lo, hi int
}

// boxed draws the row in three lines: the top edge of the box of the tab on
// view, the labels, and a baseline that is open under the box. Every line is
// exactly the width wide.
func (m Model) boxed() string {
	width := m.width
	aw := ansi.StringWidth(m.aside)
	var sp span
	switch {
	case m.aside != "":
		var ok bool
		if sp, ok = m.fit(width - 1 - aw - 1); !ok {
			aw = 0
			sp, _ = m.fit(width - 1)
		}
	default:
		sp, _ = m.fit(width - 1)
	}
	return m.drawBox(sp, aw)
}

// cellWidth is the width of a tab: its label, a blank on each side and the
// sides of the box.
func cellWidth(label string) int { return ansi.StringWidth(label) + 4 }

func (m Model) markerWidth(s string) int { return ansi.StringWidth(s) + 2 }

// total is the width the tabs of sp take, with the markers of the tabs
// hidden on either side.
func (m Model) total(sp span) int {
	t := 0
	for _, l := range sp.labels {
		t += cellWidth(l)
	}
	if sp.lo > 0 {
		t += m.markerWidth(m.before)
	}
	if sp.hi < len(m.items)-1 {
		t += m.markerWidth(m.more)
	}
	return t
}

// fit picks what to show in avail cells: the names, the short names, then the
// short names scrolled to the tab on view. It reports whether that fit; when
// it didn't, the tab on view comes alone with its label cut to fit.
func (m Model) fit(avail int) (span, bool) {
	n := len(m.items)
	cur := min(max(m.cur, 0), n-1)
	for _, short := range []bool{false, true} {
		sp := span{labels: m.names(short), lo: 0, hi: n - 1}
		if m.total(sp) <= avail {
			return sp, true
		}
	}
	short := m.names(true)
	pick := func(lo, hi int) span { return span{labels: short[lo : hi+1], lo: lo, hi: hi} }
	lo, hi := cur, cur
	if m.total(pick(lo, hi)) > avail {
		label := short[cur]
		for m.total(span{labels: []string{label}, lo: cur, hi: cur}) > avail && label != "" {
			label = ansi.Truncate(label, ansi.StringWidth(label)-1, "")
		}
		return span{labels: []string{label}, lo: cur, hi: cur}, false
	}
	for {
		grew := false
		if lo > 0 && m.total(pick(lo-1, hi)) <= avail {
			lo--
			grew = true
		}
		if hi < n-1 && m.total(pick(lo, hi+1)) <= avail {
			hi++
			grew = true
		}
		if !grew {
			return pick(lo, hi), true
		}
	}
}

func (m Model) drawBox(sp span, aw int) string {
	b, st := m.border, m.styles
	cur := min(max(m.cur, 0), len(m.items)-1)
	var r0, r1, r2 strings.Builder
	var used int
	rule := func(n int) string { return st.Marker.Render(strings.Repeat(b.Bottom, n)) }
	blank := func(n int) string { return strings.Repeat(" ", n) }

	// The baseline always starts with a rule, so the first tab gets its joint.
	r0.WriteString(" ")
	r1.WriteString(" ")
	r2.WriteString(rule(1))
	used++

	marker := func(s string) {
		w := m.markerWidth(s)
		r0.WriteString(blank(w))
		r1.WriteString(" ")
		r1.WriteString(st.Marker.Render(s))
		r1.WriteString(" ")
		r2.WriteString(rule(w))
		used += w
	}
	if sp.lo > 0 {
		marker(m.before)
	}
	for k, label := range sp.labels {
		i := sp.lo + k
		inner := ansi.StringWidth(label) + 2
		if i == cur {
			r0.WriteString(st.Current.Render(b.TopLeft + strings.Repeat(b.Top, inner) + b.TopRight))
			r1.WriteString(st.Current.Render(b.Left + " " + label + " " + b.Right))
			r2.WriteString(st.Current.Render(b.BottomRight))
			r2.WriteString(blank(inner))
			r2.WriteString(st.Current.Render(b.BottomLeft))
		} else {
			r0.WriteString(blank(inner + 2))
			r1.WriteString("  ")
			r1.WriteString(st.Other.Render(label))
			r1.WriteString("  ")
			r2.WriteString(rule(inner + 2))
		}
		used += inner + 2
	}
	if sp.hi < len(m.items)-1 {
		marker(m.more)
	}

	// Past the tabs, the first two lines are blank, the second keeps the
	// aside at its right end, and the baseline runs to the edge.
	pad := max(m.width-used, 0)
	r0.WriteString(blank(pad))
	r1.WriteString(blank(max(pad-aw, 0)))
	if aw > 0 {
		r1.WriteString(m.aside)
	}
	r2.WriteString(rule(pad))
	rows := []string{r0.String(), r1.String(), r2.String()}
	for i, r := range rows {
		rows[i] = ansi.Truncate(r, m.width, "")
	}
	return strings.Join(rows, "\n")
}
