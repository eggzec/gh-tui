package pager

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// hostRows is a host's store of rows, which the pager reads through Rows.
type hostRows []string

func (h hostRows) Len() int                   { return len(h) }
func (h hostRows) Text(i int) string          { return h[i] }
func (h hostRows) Marks(int) []termtext.Style { return nil }

func hostRowsOf(n int, word string) hostRows {
	h := make(hostRows, n)
	for i := range h {
		h[i] = word + " " + strconv.Itoa(i)
	}
	return h
}

func TestRowsShowsAndSearchesTheHostsRows(t *testing.T) {
	m := fresh(t, WithSize(40, 8), WithRows(hostRowsOf(50, "entry")))
	m.Focus()
	if m.Lines() != 50 {
		t.Fatalf("%d lines, want 50", m.Lines())
	}
	if v := m.View(); !strings.Contains(v, "entry 0") || strings.Contains(v, "entry 30") {
		t.Errorf("the view doesn't start at the first row:\n%s", v)
	}
	m = typeText(t, m, "/")
	m = typeText(t, m, "entry 3")
	m, _ = keys(t, m, "enter")
	// entry 3 and entry 30 to 39.
	if got := m.Matches(); got != 11 {
		t.Errorf("%d matches, want 11", got)
	}
	if v := m.View(); !strings.Contains(v, "entry 3") {
		t.Errorf("the view doesn't show the match:\n%s", v)
	}
}

func TestRowsWrapsTheHostsRows(t *testing.T) {
	long := strings.Repeat("word ", 20)
	m := fresh(t, WithSize(20, 8), WithWrap(true), WithRows(hostRows{long, "tail"}))
	// Twenty words of five cells wrap to six rows of 20 cells at most.
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "tail") || !strings.Contains(v, "\n  d word") {
		t.Errorf("the wrapped row isn't shown whole above the next one:\n%s", v)
	}
}

func TestSetRowsResetsLikeSetContent(t *testing.T) {
	m := fresh(t, WithSize(40, 8))
	m.Focus()
	m = settle(m, m.SetContent("a.txt", numbered(100)))
	m, _ = keys(t, m, "j", "j", "j")
	m = typeText(t, m, "/")
	m = typeText(t, m, "xxx 4")
	m, _ = keys(t, m, "enter")
	if m.top == 0 || m.Matches() == 0 {
		t.Fatalf("setup: top %d, %d matches", m.top, m.Matches())
	}
	m.SetRows(hostRowsOf(30, "other"))
	if m.top != 0 || m.Matches() != 0 || m.Query() != "" || m.Lines() != 30 {
		t.Errorf("top %d, %d matches, query %q, %d lines after SetRows", m.top, m.Matches(), m.Query(), m.Lines())
	}
	if v := m.View(); !strings.Contains(v, "other 0") {
		t.Errorf("the view doesn't show the new rows:\n%s", v)
	}
	m.SetRows(nil)
	if m.Lines() != 0 {
		t.Errorf("%d lines after SetRows(nil)", m.Lines())
	}
}

func TestDocsKeepTheirRows(t *testing.T) {
	m := docsPager(t, Doc{Name: "a.txt", Text: numbered(20)}, Doc{Name: "b.txt", Text: numbered(10)})
	h := hostRowsOf(40, "host")
	m.rows = h
	m = step(t, m, 1)
	if m.Lines() != 10 {
		t.Fatalf("%d lines in the second document, want 10", m.Lines())
	}
	m = step(t, m, -1)
	if m.Lines() != 40 || m.rows.Text(5) != "host 5" {
		t.Errorf("the first document has %d lines, want its 40 rows", m.Lines())
	}
}
