package logview

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

// synthetic returns a log of n lines of about 50 bytes, with a section for
// every 5,000 lines, a group of 20 lines in every 100, colors on every
// tenth line, an error every 997 and a line of 2,000 bytes every 1,000.
func synthetic(n int) ([]Line, []Section) {
	start := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)
	lines := make([]Line, n)
	for i := range lines {
		l := Line{Time: start.Add(time.Duration(i) * time.Millisecond)}
		switch {
		case i%100 == 10:
			l.Kind, l.Text = Group, fmt.Sprintf("Run step %d", i/100)
		case i%100 == 30:
			l.Kind = EndGroup
		case i%997 == 0:
			l.Kind, l.Text = Error, fmt.Sprintf("main_test.go:%d: expected no error, got timeout", i)
		case i%1000 == 500:
			l.Text = strings.Repeat("0123456789abcdefghij", 100)
		case i%10 == 0:
			l.Text = fmt.Sprintf("\x1b[36;1mgo test ./pkg/%05d/...\x1b[0m \x1b[32mok\x1b[0m 0.%03ds", i, i%1000)
		default:
			l.Text = fmt.Sprintf("=== RUN   TestSomething/case_%06d_with_a_name", i)
		}
		lines[i] = l
	}
	var sections []Section
	for s := 0; s < n; s += 5000 {
		sections = append(sections, Section{
			Title: fmt.Sprintf("Step %d", s/5000), Start: s, End: min(s+5000, n),
			Failed: s == n/2, Duration: 5 * time.Second,
		})
	}
	return lines, sections
}

var big = sync.OnceValues(func() ([]Line, []Section) { return synthetic(100_000) })

// benchView returns a view of a synthetic log of n lines, expanded, with
// the cursor in the middle and every "error" found.
func benchView(tb testing.TB, n int) Model {
	tb.Helper()
	lines, secs := synthetic(n)
	if n == 100_000 {
		lines, secs = big()
	}
	m := New(WithSize(120, 40), WithTimeMode(TimeRelative))
	m.Focus()
	m.SetLines(lines, secs)
	m.ExpandAll()
	m.runSearch("error")
	m.cur = len(m.vis) / 2
	m.top = m.cur - 10
	m.clamp()
	return m
}

func BenchmarkSetLines(b *testing.B) {
	lines, secs := big()
	size := 0
	for _, l := range lines {
		size += len(l.Text) + 1
	}
	m := New(WithSize(120, 40))
	b.SetBytes(int64(size))
	b.ReportAllocs()
	for b.Loop() {
		m.SetLines(lines, secs)
	}
}

// BenchmarkSetLog shows a big log that Prepare read in a tea.Cmd: what is
// left for Update.
func BenchmarkSetLog(b *testing.B) {
	lines, secs := big()
	m := New(WithSize(120, 40))
	l := m.Prepare(lines, secs)
	b.ReportAllocs()
	for b.Loop() {
		m.SetLog(l)
	}
}

// BenchmarkView renders the same window of a small and a big log: the cost
// is the window's, not the log's.
func BenchmarkView(b *testing.B) {
	for _, n := range []int{1_000, 100_000} {
		for _, wrap := range []bool{false, true} {
			name := fmt.Sprintf("%dk/scroll", n/1000)
			if wrap {
				name = fmt.Sprintf("%dk/wrap", n/1000)
			}
			b.Run(name, func(b *testing.B) {
				m := benchView(b, n)
				m.SetWrap(wrap)
				b.ReportAllocs()
				for b.Loop() {
					_ = m.View()
				}
			})
		}
	}
}

func BenchmarkUpdate(b *testing.B) {
	for _, tt := range []struct {
		name     string
		fwd, bwd tea.Msg
		wrap     bool
	}{
		{name: "line", fwd: press("j"), bwd: press("k")},
		{name: "page", fwd: press("ctrl+f"), bwd: press("ctrl+b")},
		{name: "page/wrap", fwd: press("ctrl+f"), bwd: press("ctrl+b"), wrap: true},
		{name: "error", fwd: press("e"), bwd: press("E")},
		{name: "match", fwd: press("n"), bwd: press("N")},
		{name: "end", fwd: press("G"), bwd: press("g")},
		// Folding the section under the cursor and back lists the rows
		// shown again.
		{name: "fold", fwd: press("-"), bwd: press("+")},
		{name: "all", fwd: press("*"), bwd: press("*")},
	} {
		b.Run(tt.name, func(b *testing.B) {
			m := benchView(b, 100_000)
			m.SetWrap(tt.wrap)
			b.ReportAllocs()
			i := 0
			for b.Loop() {
				// Go forward and back so the window stays in the middle.
				msg := tt.fwd
				if i%2 == 1 {
					msg = tt.bwd
				}
				i++
				m, _ = m.Update(msg)
			}
		})
	}
}
