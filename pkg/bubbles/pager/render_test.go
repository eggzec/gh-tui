package pager

import (
	"slices"
	"strconv"
	"strings"
	"testing"
)

// wrapWords is a Render that fills lines of width cells with the words
// of src, in bold, and counts its calls and the widths it was asked for.
type wrapWords struct {
	src    string
	widths []int
}

func (r *wrapWords) render(width int) string {
	r.widths = append(r.widths, width)
	var lines []string
	line := ""
	for w := range strings.FieldsSeq(r.src) {
		if line != "" && len(line)+1+len(w) > width {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += w
	}
	if line != "" {
		lines = append(lines, line)
	}
	for i, l := range lines {
		lines[i] = "\x1b[1m" + l + "\x1b[m"
	}
	return strings.Join(lines, "\n")
}

// words returns n numbered words.
func words(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("word")
		b.WriteString(strconv.Itoa(i + 1))
		b.WriteByte(' ')
	}
	return b.String()
}

// rendered returns a focused pager of 30 by 6 cells, without line numbers,
// that shows what r renders.
func rendered(tb testing.TB, r *wrapWords) Model {
	tb.Helper()
	m := New(WithSize(30, 6), WithLineNumbers(false))
	m.Focus()
	m.SetRendered("README.md", r.src, r.render)
	return m
}

func TestRenderedFitsTheWidth(t *testing.T) {
	r := &wrapWords{src: words(40)}
	m := rendered(t, r)
	if !m.Rendered() {
		t.Fatal("Rendered() = false")
	}
	if !slices.Equal(r.widths, []int{30}) {
		t.Fatalf("rendered at %v, want [30]", r.widths)
	}
	v := plain(m)
	assertFits(t, m.View(), 30, 6)
	if !strings.HasPrefix(v, "word1 word2 word3 word4 word5") {
		t.Errorf("view starts:\n%s", v)
	}
	// The status line counts the rendered lines.
	if want := "line 1/" + strconv.Itoa(m.Lines()); !strings.Contains(v, want) {
		t.Errorf("status lacks %q:\n%s", want, v)
	}
	// The same width renders nothing again.
	m.SetSize(30, 10)
	if len(r.widths) != 1 {
		t.Errorf("a resize to the same width rendered again: %v", r.widths)
	}
}

func TestRenderedFollowsTheWidth(t *testing.T) {
	r := &wrapWords{src: words(200)}
	m := rendered(t, r)
	before := m.Lines()
	m, _ = keys(t, m, "G")
	m.SetSize(60, 6)
	if r.widths[len(r.widths)-1] != 60 {
		t.Fatalf("rendered at %v, want 60 last", r.widths)
	}
	if m.Lines() >= before {
		t.Errorf("%d lines at 60 cells, %d at 30", m.Lines(), before)
	}
	// The window stays near the end.
	if top := m.topLine(); top < m.Lines()/2 {
		t.Errorf("top line %d of %d after a resize at the end", top, m.Lines())
	}
	// Line numbers narrow the text, which renders again.
	m.SetLineNumbers(true)
	if w := r.widths[len(r.widths)-1]; w != m.textWidth() || w >= 60 {
		t.Errorf("rendered at %d with line numbers, text is %d wide", w, m.textWidth())
	}
}

func TestRenderedSearchSurvivesAResize(t *testing.T) {
	r := &wrapWords{src: words(200)}
	m := rendered(t, r)
	m = typeText(t, m, "/word150")
	m, _ = deliver(m, enter)
	if m.Matches() != 1 {
		t.Fatalf("%d matches of word150, want 1", m.Matches())
	}
	m.SetSize(50, 6)
	if m.Query() != "word150" || m.Matches() != 1 {
		t.Errorf("after a resize the search is %q with %d matches", m.Query(), m.Matches())
	}
	if !strings.Contains(plain(m), "word150") {
		t.Errorf("the match isn't on view:\n%s", plain(m))
	}
}

func TestRenderedFilterSurvivesAResize(t *testing.T) {
	r := &wrapWords{src: words(200)}
	m := rendered(t, r)
	m = typeText(t, m, "&word17")
	m, _ = deliver(m, enter)
	shown := m.Shown()
	if m.Filter() != "word17" || shown == 0 || shown == m.Lines() {
		t.Fatalf("filter %q shows %d of %d lines", m.Filter(), shown, m.Lines())
	}
	m.SetSize(50, 6)
	if m.Filter() != "word17" || m.Shown() == 0 || m.Shown() == m.Lines() {
		t.Errorf("after a resize the filter %q shows %d of %d lines", m.Filter(), m.Shown(), m.Lines())
	}
}

func TestRerender(t *testing.T) {
	r := &wrapWords{src: words(10)}
	m := rendered(t, r)
	r.src = words(20)
	m.Rerender()
	if len(r.widths) != 2 || r.widths[1] != 30 {
		t.Fatalf("rendered at %v, want [30 30]", r.widths)
	}
	if !strings.Contains(plain(m), "word20") {
		t.Errorf("the new render isn't shown:\n%s", plain(m))
	}
	// Other content has nothing to render.
	_ = m.SetContent("a.txt", "plain")
	m.Rerender()
	if len(r.widths) != 2 || m.Rendered() {
		t.Errorf("plain content rendered: %v", r.widths)
	}
}

// The editor gets the source, from its first line.
func TestEditRendered(t *testing.T) {
	var got ran
	r := &wrapWords{src: words(200)}
	m := rendered(t, r)
	m.editorCmd = "vim"
	m.exec, m.tempDir = fakeExec(t, &got, nil), t.TempDir()
	m, _ = keys(t, m, "G")
	_, cmd := m.Update(press("v"))
	_ = cmd()
	if got.content != r.src {
		t.Errorf("the editor got %q, want the source", got.content)
	}
	if !slices.Contains(got.args, "+1") {
		t.Errorf("the editor ran with %q, want +1", got.args)
	}
}

// An inverted search has a gutter of its own while the line numbers are
// off: the text renders at the width beside it, once, and a resize that
// keeps that width keeps the match.
func TestRenderedInvertedSearch(t *testing.T) {
	r := &wrapWords{src: words(200)}
	m := rendered(t, r)
	m = typeText(t, m, "/!word1")
	m, _ = deliver(m, enter)
	m, _ = keys(t, m, "n")
	cur, n := m.search.cur, len(r.widths)
	if !m.search.invert || cur < 0 {
		t.Fatalf("no inverted search: %+v", m.search)
	}
	if w := r.widths[n-1]; w != m.textWidth() {
		t.Errorf("rendered at %d, the text is %d wide", w, m.textWidth())
	}
	m.SetSize(30, 8)
	m.SetSize(30, 6)
	if len(r.widths) != n {
		t.Errorf("a resize to the same width rendered again: %v", r.widths[n:])
	}
	if m.search.cur != cur {
		t.Errorf("the match is %d after a resize, want %d", m.search.cur, cur)
	}
}
