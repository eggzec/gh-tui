package pager

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/pkg/termimg"
	"github.com/eggzec/gh-tui/pkg/termtext"
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

// The line numbers keep room for the lines a reserve says the content
// may gain, and a reserve of none, as of a document without images,
// leaves them as wide as without one.
func TestRenderedReserve(t *testing.T) {
	src := strings.Repeat("line\n", 94) + "line"
	width := func(reserve func() int) int {
		var widths []int
		m := New(WithSize(30, 6))
		if reserve != nil {
			m.SetReserve(reserve)
		}
		m.SetRendered("README.md", src, func(w int) string {
			widths = append(widths, w)
			return src
		})
		if len(widths) != 1 {
			t.Fatalf("rendered at %v, want once", widths)
		}
		return widths[0]
	}
	plain := width(nil)
	if got := width(func() int { return 0 }); got != plain {
		t.Errorf("with no lines reserved the text is %d wide, want %d as without a reserve", got, plain)
	}
	if got := width(func() int { return 10 }); got != plain-1 {
		t.Errorf("with 10 lines reserved past 95 the text is %d wide, want %d", got, plain-1)
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

// The lines of rendered content that draw an image reach the view as they
// are, and searches see them blank; anywhere else the placeholder draws
// nothing.
func TestRenderedPictures(t *testing.T) {
	pic := termimg.Rows(7, 4, 2)
	text := "alt text\n" + "> " + pic[0] + "\n> " + pic[1] + "\nafter"
	m := New(WithSize(30, 6), WithLineNumbers(false))
	m.Focus()
	m.SetRendered("README.md", "![alt text](logo.png)", func(int) string { return text })
	v := m.View()
	assertFits(t, v, 30, 6)
	for _, row := range pic {
		if !strings.Contains(v, "> "+row) {
			t.Errorf("the view lacks the image row %q:\n%q", row, v)
		}
	}
	if m.Lines() != 4 {
		t.Errorf("%d lines, want 4", m.Lines())
	}
	m = typeText(t, m, "/>")
	m, _ = deliver(m, enter)
	if m.Matches() != 0 {
		t.Errorf("a search matched %d image rows", m.Matches())
	}
	// Narrower than the image, its rows are cut to the text.
	m.SetSize(3, 6)
	assertFits(t, m.View(), 3, 6)

	// Other content draws no image.
	_ = m.SetContent("a.txt", text)
	if strings.ContainsRune(m.View(), termtext.Placeholder) {
		t.Error("plain content drew an image")
	}
}

// picturePager returns a focused pager without line numbers that shows a
// rendered image of three rows between blank lines, wider than the
// pager's 6 cells, and the rows.
func picturePager(t *testing.T) (m Model, rows []string) {
	t.Helper()
	rows = termimg.Rows(7, 4, 3)
	text := "top\n\n\n" + strings.Join(rows, "\n") + "\n\n\nend of a long line"
	m = New(WithSize(10, 12), WithLineNumbers(false))
	m.Focus()
	m.SetRendered("README.md", "![a](a.png)", func(int) string { return text })
	return m, rows
}

// Squeeze keeps every row of an image, which isn't blank, and the filter
// that hides blank lines keeps them too.
func TestRenderedPicturesProject(t *testing.T) {
	m, pic := picturePager(t)
	m, _ = keys(t, m, "-", "s")
	v := m.View()
	for _, row := range pic {
		if !strings.Contains(v, row) {
			t.Errorf("squeezed, the view lacks the image row %q", row)
		}
	}
	if got := m.Shown(); got != 7 {
		t.Errorf("squeezed, %d lines show, want 7: top, blank, 3 rows, blank, end", got)
	}
	m, _ = keys(t, m, "-", "s")
	m = typeText(t, m, "&!^$")
	m, _ = deliver(m, enter)
	if got := m.Shown(); got != 5 {
		t.Errorf("filtered by !^$, %d lines show, want 5: top, 3 rows, end", got)
	}
	if v := m.View(); !strings.Contains(v, pic[1]) {
		t.Error("filtered by !^$, the image is gone")
	}
	m = typeText(t, m, "&end")
	m, _ = deliver(m, enter)
	if got := m.Shown(); got != 1 || strings.ContainsRune(m.View(), termtext.Placeholder) {
		t.Errorf("filtered by end, %d lines show, with the image: %v", got, strings.ContainsRune(m.View(), termtext.Placeholder))
	}
}

// Scrolled sideways, an image, which can't be cut on its left, shows
// blank.
func TestRenderedPicturesSideways(t *testing.T) {
	m, _ := picturePager(t)
	if !strings.ContainsRune(m.View(), termtext.Placeholder) {
		t.Fatal("the image doesn't show")
	}
	m, _ = keys(t, m, "right")
	if m.left == 0 {
		t.Fatal("didn't scroll sideways")
	}
	v := m.View()
	assertFits(t, v, 10, 12)
	if strings.ContainsRune(v, termtext.Placeholder) {
		t.Error("scrolled sideways, the image shows cut")
	}
}

func TestRenderedWaitsOutAResize(t *testing.T) {
	r := &wrapWords{src: words(200)}
	m := New(WithSize(30, 6), WithLineNumbers(false), WithResizeRest(time.Hour))
	m.Focus()
	m.SetRendered("README.md", r.src, r.render)
	m, _ = keys(t, m, "G")
	m = typeText(t, m, "/word150")
	m, _ = deliver(m, enter)
	renders := len(r.widths)

	// A resize that goes on renders nothing, and each step replaces the
	// rest before it.
	m.SetSize(40, 6)
	first := m.Settle()
	m.SetSize(50, 6)
	m.SetSize(60, 6)
	if first == nil || m.Settle() == nil {
		t.Fatal("a resize left no rest to wait out")
	}
	stale := settledMsg{id: m.id, seq: m.sizeSeq - 1}
	m, _ = m.Update(stale)
	m, _ = keys(t, m, "k")
	if len(r.widths) != renders {
		t.Fatalf("rendered at %v while the resize went on", r.widths[renders:])
	}
	if got := strings.Count(plain(m), "\n") + 1; got != m.height {
		t.Errorf("the old render shows %d rows at the new width, want %d", got, m.height)
	}

	// The last rest renders once, at the last width, and keeps the
	// search and the place.
	m, _ = m.Update(settledMsg{id: m.id, seq: m.sizeSeq})
	if len(r.widths) != renders+1 || r.widths[renders] != 60 {
		t.Fatalf("rendered at %v after the rest, want once at 60", r.widths[renders:])
	}
	if m.Query() != "word150" || m.Matches() != 1 {
		t.Errorf("after the rest the search is %q with %d matches", m.Query(), m.Matches())
	}
	if m.Settle() != nil {
		t.Error("a rendered pager still waits")
	}

	// Sizing back to the width rendered at owes nothing.
	m.SetSize(40, 6)
	m.SetSize(60, 6)
	if m.Settle() != nil {
		t.Error("the width rendered at again still waits")
	}
}
