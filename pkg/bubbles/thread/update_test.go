package thread

import (
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestUpdateKeys(t *testing.T) {
	const height = 10
	tests := []struct {
		name  string
		start int
		keys  []string
		want  func(m Model[comment]) int
	}{
		{"down", 0, []string{"j"}, fixed(1)},
		{"down arrow", 0, []string{"down"}, fixed(1)},
		{"up", 5, []string{"k"}, fixed(4)},
		{"up at top", 0, []string{"k"}, fixed(0)},
		{"half page down", 0, []string{"d"}, fixed(height / 2)},
		{"half page down ctrl", 0, []string{"ctrl+d"}, fixed(height / 2)},
		{"half page up", 8, []string{"u"}, fixed(8 - height/2)},
		{"page down", 0, []string{"f"}, fixed(height)},
		{"page up", 15, []string{"b"}, fixed(15 - height)},
		{"top", 12, []string{"g"}, fixed(0)},
		{"home", 12, []string{"home"}, fixed(0)},
		{"bottom", 0, []string{"G"}, bottom},
		{"end", 0, []string{"end"}, bottom},
		{"unbound key", 3, []string{"x"}, fixed(3)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// One chunk and no more, so scrolling loads nothing.
			m := loaded(t, newSource(1, 12), nil, 60, height)
			m.vp.SetYOffset(tt.start)
			m = press(t, m, tt.keys...)
			if got, want := m.YOffset(), tt.want(m); got != want {
				t.Errorf("YOffset() = %d, want %d", got, want)
			}
		})
	}
}

func fixed(n int) func(Model[comment]) int { return func(Model[comment]) int { return n } }

func bottom(m Model[comment]) int { return m.TotalLines() - m.Height() }

func TestUpdateBlurredIgnoresKeys(t *testing.T) {
	m := loaded(t, newSource(1, 12), nil, 60, 10)
	m.Blur()
	m = press(t, m, "j", "G")
	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.YOffset() != 0 {
		t.Fatalf("blurred thread scrolled to %d", m.YOffset())
	}
	m.Focus()
	m = press(t, m, "j")
	if !m.Focused() || m.YOffset() != 1 {
		t.Fatalf("focused thread at %d, want 1", m.YOffset())
	}
}

func TestUpdateMouseWheel(t *testing.T) {
	m := loaded(t, newSource(1, 12), nil, 60, 10)
	m, _ = m.Update(tea.MouseWheelMsg{Button: tea.MouseWheelDown})
	if m.YOffset() != 3 {
		t.Fatalf("YOffset() = %d after a wheel step, want 3", m.YOffset())
	}
}

func TestFirstChunkLoadsAfterDocument(t *testing.T) {
	src := newSource(3, 20)
	m := newTest(src, nil, 60, 10)
	if cmd := m.Init(); cmd == nil {
		t.Fatal("Init() = nil, want the spinner tick")
	}
	m, _ = m.Update(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if got := src.calls(); len(got) != 0 {
		t.Fatalf("fetched %v before the document was set", got)
	}
	m = drain(t, m, m.SetDocument(testHeader, testBody))
	if got, want := src.calls(), []string{""}; !slices.Equal(got, want) {
		t.Fatalf("fetched %v, want %v", got, want)
	}
	if !strings.Contains(ansi.Strip(strings.Join(m.lines, "\n")), "@user0") {
		t.Fatal("first comment is not laid out")
	}
}

func TestNextChunkLoadsNearBottom(t *testing.T) {
	src := newSource(3, 20)
	m := loaded(t, src, nil, 60, 10)
	if got := len(src.calls()); got != 1 {
		t.Fatalf("fetched %d chunks before scrolling, want 1", got)
	}
	// Scroll one line at a time until the next chunk is requested, and
	// check it happens within two screens of the end.
	for len(src.calls()) == 1 {
		if m.AtBottom() {
			t.Fatal("reached the bottom without loading the next chunk")
		}
		m = press(t, m, "j")
	}
	if left := m.TotalLines() - m.YOffset(); left > 2*m.Height()+chunkLines(m, 1) {
		t.Fatalf("loaded the next chunk %d lines from the end", left)
	}
	m = press(t, m, "G", "G", "G")
	if got, want := src.calls(), []string{"", "1", "2"}; !slices.Equal(got, want) {
		t.Fatalf("fetched %v, want %v", got, want)
	}
	if m.statusIdx != -1 {
		t.Fatalf("status %q after the last chunk, want none", ansi.Strip(m.status))
	}
}

func chunkLines(m Model[comment], i int) int {
	if i < len(m.chunks) {
		return m.chunks[i].height
	}
	return 0
}

func TestShortDocumentFillsScreen(t *testing.T) {
	src := newSource(5, 1)
	_ = loaded(t, src, nil, 60, 40)
	// Each chunk is a few lines, so it takes several to fill two screens.
	if got := len(src.calls()); got < 5 {
		t.Fatalf("fetched %d chunks, want all 5 to fill the screen", got)
	}
}

func TestErrorAndRetry(t *testing.T) {
	src := newSource(1, 3)
	src.fail[""] = 1
	m := loaded(t, src, nil, 60, 24)
	if !m.failed() || !strings.Contains(ansi.Strip(m.View()), "· r to retry") {
		t.Fatalf("no error line:\n%s", ansi.Strip(m.View()))
	}
	if !slices.ContainsFunc(m.ShortHelp(), func(b key.Binding) bool { return b.Enabled() && b.Help().Desc == "retry" }) {
		t.Fatal("ShortHelp() doesn't offer retry after an error")
	}
	// Scrolling doesn't retry on its own.
	m = press(t, m, "j", "G")
	if got := len(src.calls()); got != 1 {
		t.Fatalf("fetched %d times before retry, want 1", got)
	}
	m = press(t, m, "r")
	if m.failed() || !strings.Contains(ansi.Strip(m.View()), "@user0") {
		t.Fatalf("retry didn't load the comments:\n%s", ansi.Strip(m.View()))
	}
	if slices.ContainsFunc(m.ShortHelp(), func(b key.Binding) bool { return b.Enabled() && b.Help().Desc == "retry" }) {
		t.Fatal("ShortHelp() offers retry without an error")
	}
}

func TestIgnoresOtherInstances(t *testing.T) {
	a := loaded(t, newSource(2, 20), nil, 60, 10)
	b := newTest(newSource(2, 20), nil, 60, 10)
	cmd := b.SetDocument("other", "body")
	msg := cmd().(tea.BatchMsg)[0]()
	before := a.TotalLines()
	a, next := a.Update(msg)
	if next != nil || a.TotalLines() != before {
		t.Fatal("thread applied another instance's chunk")
	}
	tick := b.spin.Tick()
	if _, next := a.Update(tick); next != nil {
		t.Fatal("thread reacted to another instance's spinner")
	}
}

func TestSpinnerStopsWhenIdle(t *testing.T) {
	m := newTest(newSource(1, 1), nil, 60, 10)
	tick := m.spin.Tick()
	m, cmd := m.Update(tick)
	if cmd == nil {
		t.Fatal("spinner doesn't tick while the document loads")
	}
	m = drain(t, m, m.SetDocument("h", "b"))
	if _, cmd := m.Update(m.spin.Tick()); cmd != nil {
		t.Fatal("spinner keeps ticking with nothing loading")
	}
}

func TestRenderCaching(t *testing.T) {
	src := newSource(2, 10)
	r := &renders{}
	m := loaded(t, src, r, 60, 10)
	runs, items := m.md.Renders(), r.count()
	if runs != 1 || items != 10 {
		t.Fatalf("rendered markdown %d and comments %d times, want 1 and 10", runs, items)
	}
	for range 20 {
		m = press(t, m, "j")
		_ = m.View()
	}
	m = press(t, m, "g")
	if m.md.Renders() != runs || r.count() != items {
		t.Fatalf("scrolling rendered markdown %d and comments %d times", m.md.Renders()-runs, r.count()-items)
	}
	// The same width again is free; a new one renders once more.
	m.SetSize(60, 12)
	if m.md.Renders() != runs {
		t.Fatal("a new height rendered the markdown again")
	}
	m.SetSize(50, 12)
	if m.md.Renders() != runs+1 || r.count() != 2*items {
		t.Fatalf("a new width rendered markdown %d and comments %d times, want 1 and %d",
			m.md.Renders()-runs, r.count()-items, items)
	}
}

func TestNewStylesRenderAgain(t *testing.T) {
	src := newSource(2, 10)
	r := &renders{}
	m := loaded(t, src, r, 60, 10)
	runs, items := m.md.Renders(), r.count()
	m.SetStyles(DefaultStyles(false))
	if r.count() != 2*items {
		t.Errorf("new styles rendered the comments %d times, want %d", r.count()-items, items)
	}
	// The body keeps its pinned markdown style, but its renderer forgot
	// what it rendered, since it can't tell the style didn't change.
	if m.md.Renders() != runs+1 {
		t.Errorf("new styles rendered the body %d times, want once", m.md.Renders()-runs)
	}
}

func TestCutHint(t *testing.T) {
	m := loaded(t, newSource(1, 1), nil, 120, 10)
	_ = m.SetDocument("", strings.Repeat("line\n\n", 600))
	m.SetCutHint("o to open on GitHub")
	last := strings.TrimSpace(ansi.Strip(m.doc[len(m.doc)-2]))
	// The pinned ASCII style marks emphasis with asterisks.
	if last != "*⋯ The rest is too long to show here · o to open on GitHub*" {
		t.Errorf("the cut body ends %q, without the hint", last)
	}
}

func TestMarkdownRendersLikeTheBody(t *testing.T) {
	m := loaded(t, newSource(1, 1), nil, 60, 10)
	got := m.Markdown("line one\nline two<!-- hidden -->", 40)
	if want := "line one\nline two"; ansi.Strip(got) != want {
		t.Errorf("Markdown = %q, want %q", ansi.Strip(got), want)
	}
	runs := m.md.Renders()
	if m.Markdown("line one\nline two<!-- hidden -->", 40) != got || m.md.Renders() != runs {
		t.Error("the same markdown at the same width rendered again")
	}
}

func TestResizeKeepsReadingPosition(t *testing.T) {
	m := loaded(t, newSource(1, 30), nil, 80, 10)
	// Put the top of @user7 at the top of the screen.
	m.vp.SetYOffset(m.starts[0] + m.chunks[0].starts[7])
	for _, w := range []int{40, 30, 100, 80} {
		m.SetSize(w, 10)
		if top := ansi.Strip(m.lines[m.YOffset()]); !strings.Contains(top, "@user7") {
			t.Fatalf("width %d: top line is %q, want @user7", w, strings.TrimSpace(top))
		}
	}
	// Part way into a comment stays part way into it.
	m.vp.SetYOffset(m.starts[0] + m.chunks[0].starts[8] + 1)
	m.SetSize(30, 10)
	c := m.chunks[0]
	if y := m.YOffset() - m.starts[0]; y <= c.starts[8] || y >= c.starts[9] {
		t.Fatalf("top line %d is outside @user8 [%d, %d)", y, c.starts[8], c.starts[9])
	}
}

func TestSetDocumentAgainKeepsComments(t *testing.T) {
	src := newSource(1, 30)
	m := loaded(t, src, nil, 60, 10)
	m.vp.SetYOffset(m.starts[0] + m.chunks[0].starts[5])
	m = drain(t, m, m.SetDocument(testHeader, testBody+"\n\nEdited: one more paragraph."))
	if got := len(src.calls()); got != 1 {
		t.Fatalf("fetched %d times, want 1", got)
	}
	if top := ansi.Strip(m.lines[m.YOffset()]); !strings.Contains(top, "@user5") {
		t.Fatalf("top line is %q, want @user5", strings.TrimSpace(top))
	}
}

func TestAccessors(t *testing.T) {
	m := loaded(t, newSource(1, 30), nil, 60, 10)
	if m.ScrollPercent() != 0 || m.AtBottom() {
		t.Fatalf("at top: ScrollPercent() = %v, AtBottom() = %v", m.ScrollPercent(), m.AtBottom())
	}
	m = press(t, m, "G")
	if m.ScrollPercent() != 1 || !m.AtBottom() {
		t.Fatalf("at bottom: ScrollPercent() = %v, AtBottom() = %v", m.ScrollPercent(), m.AtBottom())
	}
	if m.Width() != 60 || m.Height() != 10 || m.ID() == 0 {
		t.Fatalf("Width, Height, ID = %d, %d, %d", m.Width(), m.Height(), m.ID())
	}
	k := DefaultKeyMap()
	k.Down.SetKeys("n")
	m.SetKeyMap(k)
	m.vp.SetYOffset(0)
	m = press(t, m, "n")
	if m.YOffset() != 1 || !slices.Equal(m.KeyMap().Down.Keys(), []string{"n"}) {
		t.Fatal("SetKeyMap didn't rebind Down")
	}
	if len(m.FullHelp()) == 0 {
		t.Fatal("FullHelp() is empty")
	}
	s := DefaultStyles(false)
	m.SetStyles(s)
	if m.Styles().Markdown.Heading.Color == nil {
		t.Fatal("SetStyles didn't keep the markdown style")
	}
}

func TestDefaultStylesPickMarkdownTheme(t *testing.T) {
	dark, light := DefaultStyles(true).Markdown, DefaultStyles(false).Markdown
	if *dark.Heading.Color == *light.Heading.Color {
		t.Fatal("light and dark markdown styles are the same")
	}
	m := New(newSource(0, 0).fetch, renderComment, WithSize(40, 5), WithStyles(DefaultStyles(false)))
	_ = m.SetDocument("", "## Title")
	light1 := strings.Join(m.doc, "\n")
	m.SetStyles(DefaultStyles(true))
	if strings.Join(m.doc, "\n") == light1 {
		t.Fatal("SetStyles didn't render the body with the new markdown style")
	}
}
