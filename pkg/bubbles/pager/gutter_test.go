package pager

import (
	"reflect"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// timedRows is a host's rows with a header row at index 0.
type timedRows []string

func (r timedRows) Len() int                   { return len(r) }
func (r timedRows) Text(i int) string          { return r[i] }
func (r timedRows) Marks(int) []termtext.Style { return nil }

func (r timedRows) Header(i int) (string, bool) { return r[i], i == 0 }

// timeGutter is a column of "12:00 " before each row that hides itself when
// less than 20 columns are left for the text.
type timeGutter struct{}

func (timeGutter) Width(total int) int {
	if total < 26 {
		return 0
	}
	return 6
}

func (timeGutter) Write(b *strings.Builder, i int, first bool) {
	if !first {
		b.WriteString("      ")
		return
	}
	b.WriteByte('1')
	b.WriteRune(rune('0' + i))
	b.WriteString(":00 ")
}

func bannerWriter(b *strings.Builder, _ int, text string, width int) {
	s := ansi.Truncate("== "+text+" ", width, "")
	b.WriteString(s)
	b.WriteString(strings.Repeat(" ", width-ansi.StringWidth(s)))
}

func hostView(t *testing.T, w int, rows timedRows, opts ...Option) (m Model, lines []string) {
	t.Helper()
	opts = append([]Option{WithSize(w, 12), WithWrap(true), WithRows(rows), WithGutter(timeGutter{}), WithHeaderWriter(bannerWriter)}, opts...)
	m = fresh(t, opts...)
	m.Focus()
	return m, strings.Split(ansi.Strip(m.View()), "\n")
}

func TestGutterOfTheHostWrapsAndContinues(t *testing.T) {
	rows := timedRows{"step one", strings.Repeat("word ", 8), "tail"}
	m, lines := hostView(t, 30, rows)
	// The gutter takes 6 columns, so the text wraps at 24: 40 cells, two
	// rows, the second starting with the space that ended the first.
	want := []string{"== step one", "11:00 word word word word word", "       word word word", "12:00 tail"}
	for i, w := range want {
		if got := strings.TrimRight(lines[i], " "); got != w {
			t.Errorf("row %d = %q, want %q", i, got, w)
		}
	}
	if tw := m.textWidth(); tw != 24 {
		t.Errorf("text width %d, want 24", tw)
	}
	if n := m.rowsIn(0); n != 1 {
		t.Errorf("the header takes %d rows, want 1", n)
	}
	if n := m.rowsIn(1); n != 2 {
		t.Errorf("the wrapped row takes %d rows, want 2", n)
	}
	for i, l := range lines[:len(lines)-1] {
		if w := ansi.StringWidth(l); w != 30 {
			t.Errorf("row %d is %d cells wide, want 30", i, w)
		}
	}
}

func TestAppendKeepsRowsAlignedWithTheHostGutter(t *testing.T) {
	l := liveOf(3, "alpha beta gamma delta epsilon")
	m := fresh(t, WithSize(30, 8), WithWrap(true), WithRows(l), WithGutter(timeGutter{}))
	m.Focus()
	l.add("alpha beta gamma delta epsilon", 3, 6)
	m.Append(3)
	ref := fresh(t, WithSize(30, 8), WithWrap(true), WithRows(l), WithGutter(timeGutter{}))
	ref.Focus()
	if got, want := m.View(), ref.View(); got != want {
		t.Errorf("appended view differs from a fresh one:\n%s\nwant:\n%s", ansi.Strip(got), ansi.Strip(want))
	}
	same(t, m, l)
}

// growRows is timedRows that grow, with a header at index 0.
type growRows struct{ lines []string }

func (r *growRows) Len() int                   { return len(r.lines) }
func (r *growRows) Text(i int) string          { return r.lines[i] }
func (r *growRows) Marks(int) []termtext.Style { return nil }

func (r *growRows) Header(i int) (string, bool) { return r.lines[i], i == 0 }

// growGutter is a gutter that takes the width its host last set, as one
// numbering the rows does as they grow.
type growGutter struct{ w *int }

func (g growGutter) Width(int) int { return *g.w }

func (g growGutter) Write(b *strings.Builder, _ int, first bool) {
	if !first {
		b.WriteString(strings.Repeat(" ", *g.w))
		return
	}
	b.WriteString(strings.Repeat("#", *g.w))
}

func TestAppendSeesTheHostGutterGrow(t *testing.T) {
	// The window is one row, so the width of the text decides which words
	// of the line are in it, while the window keeps its first and last line.
	long := strings.Repeat("word ", 8)
	for _, follow := range []bool{false, true} {
		rows := &growRows{lines: []string{"head", long, long}}
		w := 2
		opts := func() []Option {
			return []Option{WithSize(24, 2), WithWrap(true), WithRows(rows), WithGutter(growGutter{&w}), WithHeaderWriter(bannerWriter)}
		}
		start := func() Model {
			m := fresh(t, opts()...)
			m.Focus()
			m = typeText(t, m, "/")
			m = typeText(t, m, "word")
			m, _ = keys(t, m, "enter")
			// A jump to a match turns the follow off.
			m.SetFollow(follow)
			return m
		}
		m := start()
		if !follow {
			// The first row of the top line is out of the window, and where
			// the second starts depends on the width of the text.
			m.top, m.row = 1, 1
			m.clamp()
		}
		// The host widens the gutter as it adds the rows, before it calls
		// Append.
		w = 7
		rows.lines = append(rows.lines, long, "tail word")
		m.Append(3)
		ref := start()
		// The matches the window shows are those of a pager that finds them
		// anew, which the view only paints.
		found := m
		found.hits = hits{}
		found.findHits()
		if !reflect.DeepEqual(m.hits.lines, found.hits.lines) {
			t.Errorf("follow %v: window matches %v, want %v", follow, m.hits.lines, found.hits.lines)
		}
		if m.Following() != follow || m.Matches() != ref.Matches() {
			t.Errorf("follow %v: following %v, %d matches; want %d", follow, m.Following(), m.Matches(), ref.Matches())
		}
	}
}

func TestGutterOfTheHostShrinksAtNarrowWidths(t *testing.T) {
	rows := timedRows{"step", "alpha beta gamma", "tail"}
	m, lines := hostView(t, 20, rows)
	if gw := m.gutterWidth(); gw != 0 {
		t.Fatalf("gutter %d at 20 columns, want 0", gw)
	}
	if got := strings.TrimRight(lines[1], " "); got != "alpha beta gamma" {
		t.Errorf("row = %q, want the text from the first column", got)
	}
}

func TestHeaderIsOneRowEvenWhenLong(t *testing.T) {
	long := strings.Repeat("header ", 10)
	_, lines := hostView(t, 30, timedRows{long, "a"})
	if !strings.HasPrefix(lines[0], "== header") || !strings.HasPrefix(lines[1], "11:00 a") {
		t.Errorf("the header wrapped or is missing:\n%s", strings.Join(lines, "\n"))
	}
}

func TestSearchFindsRowsOfTheHostGutter(t *testing.T) {
	rows := timedRows{"step", strings.Repeat("word ", 8) + "needle", "tail"}
	m, _ := hostView(t, 30, rows)
	m = typeText(t, m, "/")
	m = typeText(t, m, "needle")
	m, _ = keys(t, m, "enter")
	if m.Matches() != 1 {
		t.Fatalf("%d matches, want 1", m.Matches())
	}
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "       word word word needle") {
		t.Errorf("the match isn't shown on its continuation row:\n%s", v)
	}
}

func TestRowStyleIsTheBaseOfARow(t *testing.T) {
	bold := NewPair(lipgloss.NewStyle().Bold(true))
	style := func(i int) Pair {
		if i == 1 {
			return bold
		}
		return Pair{}
	}
	rows := timedRows{"step", "plain needle text", "other needle text"}
	m, _ := hostView(t, 40, rows, WithRowStyle(style))
	v := m.View()
	if !strings.Contains(v, bold.p.on+"plain needle text") {
		t.Errorf("the row isn't in its style:\n%q", v)
	}
	if strings.Contains(v, bold.p.on+"other") {
		t.Errorf("a row with the zero pair is in the style of another:\n%q", v)
	}
	m = typeText(t, m, "/")
	m = typeText(t, m, "needle")
	m, _ = keys(t, m, "enter")
	v = m.View()
	// The match keeps its own style over the row's, and the row's comes
	// back after it.
	want := bold.p.on + "plain " + bold.p.off + m.esc.current.on + "needle" + m.esc.current.off + bold.p.on + " text"
	if !strings.Contains(v, want) {
		t.Errorf("the match isn't drawn over the row's style:\n%q", v)
	}
}
