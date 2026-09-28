package finder

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var sample = []string{
	"cursed_renderer.go",
	"cursed_renderer_test.go",
	"examples/altscreen-toggle/main.go",
	"examples/render/render.go",
	"nil_renderer.go",
	"README.md",
	"renderer.go",
	"tea.go",
}

func TestLoadListsEverything(t *testing.T) {
	m := open(t, 40, 10, sample)
	if m.Loading() || m.Total() != len(sample) || m.Matches() != len(sample) {
		t.Fatalf("loading %v, %d of %d paths listed", m.Loading(), m.Matches(), m.Total())
	}
	if got := selected(m); got != sample[0] {
		t.Errorf("selected %q, want the first path", got)
	}
}

func TestTyping(t *testing.T) {
	tests := []struct {
		name  string
		query string
		keys  []string
		want  string
		n     int
	}{
		{"best first", "rend", nil, "renderer.go", 5},
		{"down", "rend", []string{"down"}, "examples/render/render.go", 5},
		{"ctrl+n and ctrl+p", "rend", []string{"ctrl+n", "ctrl+n", "ctrl+p"}, "examples/render/render.go", 5},
		{"up stops at the top", "rend", []string{"up"}, "renderer.go", 5},
		{"down stops at the bottom", "rend", slices.Repeat([]string{"down"}, 9), "cursed_renderer_test.go", 5},
		{"page down", "", []string{"pgdown"}, "renderer.go", len(sample)},
		{"backspace widens", "rendx", []string{"backspace"}, "renderer.go", 5},
		{"no match", "zzz", nil, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Eight rows leave six for matches.
			m := open(t, 40, 8, sample)
			m = typed(t, m, tt.query)
			m = keys(t, m, tt.keys...)
			if got := selected(m); got != tt.want {
				t.Errorf("selected %q, want %q", got, tt.want)
			}
			if m.Matches() != tt.n {
				t.Errorf("%d matches, want %d", m.Matches(), tt.n)
			}
		})
	}
}

func TestChooseAndCancel(t *testing.T) {
	m := open(t, 40, 8, sample)
	m = typed(t, m, "tea")
	_, msgs := collect(t, m, func() tea.Cmd { _, cmd := m.Update(press("enter")); return cmd }())
	want := []tea.Msg{ChosenMsg{ID: m.ID(), Item: Item{Path: "tea.go"}}}
	if !slices.EqualFunc(msgs, want, func(a, b tea.Msg) bool { return a == b }) {
		t.Errorf("enter sent %v, want %v", msgs, want)
	}
	_, cmd := m.Update(press("esc"))
	if _, msgs = collect(t, m, cmd); len(msgs) != 1 || msgs[0] != (CancelMsg{ID: m.ID()}) {
		t.Errorf("esc sent %v, want a CancelMsg", msgs)
	}
	m = typed(t, m, "zzz")
	if _, cmd := m.Update(press("enter")); cmd != nil {
		t.Error("enter without a match sent something")
	}
}

// pending returns the messages cmd sends, without feeding them back.
func pending(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if b, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range b {
			out = append(out, pending(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// matchOf returns the matchMsg among msgs.
func matchOf(tb testing.TB, msgs []tea.Msg) matchMsg {
	tb.Helper()
	for _, msg := range msgs {
		if mm, ok := msg.(matchMsg); ok {
			return mm
		}
	}
	tb.Fatalf("no match in %v", msgs)
	return matchMsg{}
}

// TestMatchInCommand matches every query in a command: the last result
// stays until the new one arrives, and a result that another query
// overtook is dropped.
func TestMatchInCommand(t *testing.T) {
	m := open(t, 40, 8, sample, WithSyncLimit(0))
	m, first := m.Update(press("r"))
	if !m.Matching() || m.Matches() != len(sample) {
		t.Fatalf("matching %v with %d matches shown, want the last result while it runs", m.Matching(), m.Matches())
	}
	stale := matchOf(t, pending(first))
	m, second := m.Update(press("e"))
	m, _ = m.Update(stale)
	if !m.Matching() || m.Matches() != len(sample) {
		t.Fatalf("a stale result was shown: %d matches", m.Matches())
	}
	m, _ = m.Update(matchOf(t, pending(second)))
	if m.Matching() || m.Matches() != 7 || m.Query() != "re" {
		t.Errorf("matching %v, %d matches for %q, want 7 for re", m.Matching(), m.Matches(), m.Query())
	}
}

func TestNarrowsFromLastResult(t *testing.T) {
	m := open(t, 40, 8, sample)
	m = typed(t, m, "ren")
	if from := m.from("rend"); from != m.res {
		t.Error("rend doesn't look through the matches of ren")
	}
	if from := m.from("re"); from != nil {
		t.Error("re looks through the matches of ren")
	}
	m = typed(t, m, "d")
	want := ranked(t, m.corpus, "rend", nil)
	if got := pathsOf(m.corpus, m.res); !slices.Equal(got, want) {
		t.Errorf("narrowed to %v, want %v", got, want)
	}
}

func TestLoadError(t *testing.T) {
	boom := errors.New("boom\nand more")
	m := New(func(context.Context) (Listing, error) { return Listing{}, boom }, WithSize(40, 5))
	m.Focus()
	m = run(t, m, m.Init())
	if !errors.Is(m.Err(), boom) || m.Loading() {
		t.Fatalf("err %v, loading %v", m.Err(), m.Loading())
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Couldn't list the files: boom") {
		t.Errorf("view = %q, want the error", v)
	}
}

func TestErrorText(t *testing.T) {
	tests := []struct {
		name  string
		opts  []Option
		width int
		want  string
	}{
		{"default", nil, 40, "✗ Couldn't list the files: boom"},
		{"custom", []Option{WithErrorText(func(error) (string, string) { return "Can't reach GitHub", "r to retry" })}, 40, "✗ Can't reach GitHub · r to retry"},
		{"custom keeps the hint whole", []Option{WithErrorText(func(error) (string, string) { return "Can't reach GitHub", "r to retry" })}, 24, "✗ Can't re… · r to retry"},
		{"empty", []Option{WithErrorText(func(error) (string, string) { return "", "" })}, 40, ""},
		{"text with a dot", []Option{WithErrorText(func(error) (string, string) { return "GitHub says a · b", "" })}, 40, "✗ GitHub says a · b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			load := func(context.Context) (Listing, error) { return Listing{}, errors.New("boom\nand more") }
			m := New(load, append([]Option{WithSize(tt.width, 5)}, tt.opts...)...)
			m = run(t, m, m.Init())
			rows := strings.Split(ansi.Strip(m.View()), "\n")
			if got := strings.TrimRight(rows[1], " "); got != tt.want {
				t.Errorf("error row = %q, want %q", got, tt.want)
			}
			if tt.want == "" && strings.Contains(m.View(), "✗") {
				t.Errorf("View() = %q, want no error", m.View())
			}
		})
	}
}

// ASCII styles cut the error row with their own ellipsis, with a hint or
// without, however narrow the row.
func TestErrorASCII(t *testing.T) {
	st := DefaultStyles(true)
	st.ErrorGlyph, st.ErrorSeparator, st.ErrorEllipsis = "x", " - ", "..."
	load := func(context.Context) (Listing, error) { return Listing{}, errors.New("boom") }
	hint := WithErrorText(func(error) (string, string) { return "Can't reach GitHub", "r to retry" })
	tests := []struct {
		name  string
		opts  []Option
		width int
		want  string
	}{
		{"without a hint", nil, 24, "x Couldn't list the f..."},
		{"with a hint", []Option{hint}, 24, "x Can't ... - r to retry"},
		{"with a hint wider than the row", []Option{hint}, 12, " - r to r..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(load, append([]Option{WithSize(tt.width, 5), WithStyles(st)}, tt.opts...)...)
			m = run(t, m, m.Init())
			row := strings.Split(ansi.Strip(m.View()), "\n")[1]
			if got := strings.TrimRight(row, " "); got != tt.want {
				t.Errorf("error row = %q, want %q", got, tt.want)
			}
			for i := range len(row) {
				if row[i] >= 0x80 {
					t.Fatalf("error row %q has a byte beyond ASCII", row)
				}
			}
		})
	}
}

func TestCloseCancelsLoad(t *testing.T) {
	var ctx context.Context
	m := New(func(c context.Context) (Listing, error) {
		ctx = c
		<-c.Done()
		return Listing{}, c.Err()
	})
	cmd := m.Init()
	m.Close()
	_ = pending(cmd)
	if ctx == nil || ctx.Err() == nil {
		t.Error("Close didn't cancel the load")
	}
}

func TestTypingWhileLoading(t *testing.T) {
	m := New(loader(sample...), WithSize(40, 8))
	m.Focus()
	load := m.Init()
	m = typed(t, m, "tea")
	m = run(t, m, load)
	if got := selected(m); got != "tea.go" || m.Matches() != 2 {
		t.Errorf("selected %q of %d, want the query typed while loading matched", got, m.Matches())
	}
}

func TestReset(t *testing.T) {
	m := open(t, 40, 8, sample, WithRecent([]string{"tea.go"}))
	if got := selected(m); got != "tea.go" {
		t.Fatalf("selected %q, want the recent path first", got)
	}
	m = typed(t, m, "rend")
	m = run(t, m, m.Reset([]string{"README.md", "nil_renderer.go"}))
	if m.Query() != "" || selected(m) != "README.md" {
		t.Errorf("query %q, selected %q after Reset", m.Query(), selected(m))
	}
	m = typed(t, m, "rend")
	if got := selected(m); got != "nil_renderer.go" {
		t.Errorf("selected %q, want the recent match first", got)
	}
}

func TestSetQuery(t *testing.T) {
	m := open(t, 40, 8, sample)
	m = run(t, m, m.SetQuery("alt"))
	if got := selected(m); got != "examples/altscreen-toggle/main.go" {
		t.Errorf("selected %q", got)
	}
}

func TestPaste(t *testing.T) {
	m := open(t, 40, 8, sample)
	m, cmd := m.Update(tea.PasteMsg{Content: "nil"})
	m = run(t, m, cmd)
	if got := selected(m); got != "nil_renderer.go" {
		t.Errorf("selected %q after a paste", got)
	}
}

func TestBlurredIgnoresKeys(t *testing.T) {
	m := open(t, 40, 8, sample)
	m.Blur()
	m = typed(t, m, "tea")
	if m.Query() != "" || m.Focused() {
		t.Errorf("a blurred finder took %q", m.Query())
	}
}

func TestIgnoresOtherInstances(t *testing.T) {
	a := open(t, 40, 8, sample)
	b := New(loader("other.go"))
	for _, msg := range pending(b.Init()) {
		a2, cmd := a.Update(msg)
		if cmd != nil || a2.Total() != a.Total() {
			t.Fatal("finder reacted to another finder's message")
		}
	}
	if a.ID() == b.ID() {
		t.Fatal("two finders share an ID")
	}
}

func TestResizeKeepsSelectionInView(t *testing.T) {
	m := open(t, 40, 12, sample)
	m = keys(t, m, "down", "down", "down", "down", "down", "down")
	m.SetSize(40, 5)
	if m.sel < m.top || m.sel >= m.top+m.listHeight() {
		t.Errorf("selection %d outside rows %d to %d", m.sel, m.top, m.top+m.listHeight())
	}
	assertFits(t, m.View(), 40, 5)
}

// TestRetry checks that Retry loads the paths again once their load
// failed, and does nothing otherwise.
func TestRetry(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	m := New(func(context.Context) (Listing, error) {
		if fail.Load() {
			return Listing{}, errors.New("offline")
		}
		return Listing{Items: items("a.go", "b.go")}, nil
	}, WithSize(40, 5))
	m.Focus()
	m = run(t, m, m.Init())
	fail.Store(false)
	m = run(t, m, m.Retry())
	if m.Err() != nil || m.Loading() {
		t.Fatalf("after Retry: err %v, loading %v", m.Err(), m.Loading())
	}
	if v := ansi.Strip(m.View()); !strings.Contains(v, "a.go") {
		t.Errorf("view = %q, want the paths", v)
	}
	if cmd := m.Retry(); cmd != nil {
		t.Error("Retry with nothing failed returned a command")
	}
}
