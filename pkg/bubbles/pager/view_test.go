package pager

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

const markdown = "# Title\n\nSome *emphasis* and `code`.\n\n- one\n- two\n"

// colored is a program's output kept with its colors, one of which spans
// lines, and escapes that aren't colors.
const colored = "\x1b[1;32m=== RUN\x1b[0m   TestView\n" +
	"\x1b[31m--- FAIL: TestView (0.01s)\n" +
	"    view_test.go:42: got \x1b[1mred\x1b[22m, want 你好\x1b[0m\n" +
	"\x1b]0;title\a\x1b[2Jok \x1b[38;5;208mgithub.com/eggzec/gh-tui/pkg/bubbles/pager\x1b[m 0.4s\n"

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		file, text    string
		opts          []Option
		width, height int
		keys          []string
		filter        string
		search        string
		set           func(*Model)
	}{
		{name: "go", file: "main.go", text: goSource, width: 60, height: 12},
		{name: "markdown", file: "README.md", text: markdown, width: 40, height: 8},
		{name: "plain text", file: "notes.txt", text: "Just some notes.\n\tIndented with a tab.\n", width: 40, height: 4},
		{name: "light", file: "main.go", text: goSource, width: 60, height: 12,
			opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "no syntax colors", file: "main.go", text: goSource, width: 60, height: 12,
			opts: []Option{WithStyles(Styles{})}},
		{name: "without line numbers", file: "main.go", text: goSource, width: 60, height: 12,
			opts: []Option{WithLineNumbers(false)}},
		{name: "scrolled down", file: "lines.txt", text: numbered(40), width: 30, height: 8, keys: []string{"ctrl+d"}},
		{name: "at the end", file: "lines.txt", text: numbered(40), width: 30, height: 8, keys: []string{"G"}},
		{name: "scrolled sideways", file: "main.go", text: goSource, width: 40, height: 12, keys: []string{"l", "l", "l"}},
		// The scroll cuts a wide rune in half, which leaves a blank.
		{name: "cuts wide runes", file: "a.txt", text: "a你好世界\n", width: 6, height: 2,
			opts: []Option{WithLineNumbers(false)}, keys: []string{"l", "l"}},
		{name: "wrapped", file: "main.go", text: goSource, width: 40, height: 14, keys: []string{"-", "S"}},
		{name: "wrapped scrolled into a line", file: "main.go", text: goSource, width: 40, height: 6,
			keys: []string{"-", "S", "j", "j", "j", "j", "j"}},
		{name: "wraps wide runes", file: "a.txt", text: "abc你好世界\n", width: 5, height: 5,
			opts: []Option{WithLineNumbers(false), WithWrap(true)}},
		{name: "search", file: "main.go", text: goSource, width: 60, height: 12, search: "main"},
		{name: "next match", file: "main.go", text: goSource, width: 60, height: 12, search: "main", keys: []string{"n"}},
		{name: "search input", file: "main.go", text: goSource, width: 60, height: 12, keys: []string{"/", "f", "m"}},
		{name: "prompt", file: "main.go", text: goSource, width: 80, height: 12, keys: []string{"/", "f", "m"}},
		{name: "prompt light", file: "main.go", text: goSource, width: 80, height: 12, keys: []string{"/", "f", "m"},
			opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "not found", file: "main.go", text: goSource, width: 80, height: 12, search: "kiwi"},
		{name: "invalid pattern", file: "main.go", text: goSource, width: 80, height: 12, search: "fmt(",
			opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "inverted", file: "main.go", text: goSource, width: 80, height: 12, search: "!fmt", keys: []string{"n"}},
		{name: "inverted light", file: "main.go", text: goSource, width: 80, height: 12, search: "!fmt", keys: []string{"n"},
			opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "inverted without line numbers", file: "main.go", text: goSource, width: 80, height: 12, search: "!fmt",
			opts: []Option{WithLineNumbers(false)}},
		{name: "filtered", file: "main.go", text: goSource, width: 80, height: 8, filter: "fmt|func"},
		{name: "filtered light", file: "main.go", text: goSource, width: 80, height: 8, filter: "fmt|func",
			opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "filtered inverted", file: "main.go", text: goSource, width: 80, height: 8, filter: "!^\\s"},
		{name: "filtered search", file: "main.go", text: goSource, width: 80, height: 8, filter: "fmt|func",
			search: "main", keys: []string{"n"}},
		{name: "filtered wrapped", file: "main.go", text: goSource, width: 40, height: 8, filter: "fmt|func",
			opts: []Option{WithWrap(true)}},
		{name: "filter prompt", file: "main.go", text: goSource, width: 80, height: 8, keys: []string{"&", "f", "m"}},
		{name: "option", file: "main.go", text: goSource, width: 80, height: 8, keys: []string{"-"}},
		{name: "no such option", file: "main.go", text: goSource, width: 80, height: 8, keys: []string{"-", "x"}},
		{name: "squeezed", file: "main.go", text: "package main\n\n\n\nfunc main() {\n\n\n}\n", width: 80, height: 8,
			keys: []string{"-", "s"}},
		{name: "count", file: "lines.txt", text: numbered(40), width: 30, height: 8, keys: []string{"1", "2"}},
		{name: "went to a line", file: "lines.txt", text: numbered(40), width: 30, height: 8, keys: []string{"1", "2", "g"}},
		{name: "colors", file: "test.log", text: colored, width: 50, height: 5},
		{name: "colors light", file: "test.log", text: colored, width: 50, height: 5,
			opts: []Option{WithStyles(DefaultStyles(false))}},
		{name: "colors search", file: "test.log", text: colored, width: 50, height: 5, search: "red|FAIL", keys: []string{"n"}},
		{name: "colors filtered", file: "test.log", text: colored, width: 50, height: 5, filter: "view_test"},
		{name: "colors wrapped", file: "test.log", text: colored, width: 30, height: 8, opts: []Option{WithWrap(true)}},
		{name: "colors scrolled sideways", file: "test.log", text: colored, width: 30, height: 5, keys: []string{"l", "l"}},
		{name: "latin-1", file: "notes.txt", text: "Gr\xfc\xdfe aus K\xf6ln\n\x93Zitat\x94 \x96 5 \x80\n", width: 30, height: 3},
		{name: "invalid bytes", file: "notes.txt", text: "你好, w\xf6rld \xff\n", width: 30, height: 2},
		{name: "long name", file: "internal/some/very/deeply/nested/package/main.go", text: goSource,
			width: 40, height: 4},
		{name: "loading", width: 40, height: 4, set: func(m *Model) { _ = m.SetLoading("main.go") }},
		{name: "failed", width: 40, height: 4,
			set: func(m *Model) { m.SetError("main.go", errors.New("404 Not Found\nmore")) }},
		{name: "message", width: 40, height: 4,
			set: func(m *Model) { m.SetMessage("big.bin", "Too large to preview.") }},
		{name: "binary", file: "logo.png", text: "\x89PNG\r\n\x00\x00", width: 40, height: 4},
		{name: "empty file", file: "empty.go", text: "", width: 40, height: 4},
		{name: "nothing", width: 40, height: 4, set: func(*Model) {}},
		{name: "one row", file: "main.go", text: goSource, width: 40, height: 1},
		{name: "narrow", file: "main.go", text: goSource, width: 3, height: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m Model
			if tt.set != nil {
				m = New(append(tt.opts, WithSize(tt.width, tt.height))...)
				m.Focus()
				tt.set(&m)
			} else {
				m = open(t, tt.file, tt.text, append(tt.opts, WithSize(tt.width, tt.height))...)
			}
			if tt.filter != "" {
				m, _ = keys(t, m, "&")
				m = typeText(t, m, tt.filter)
				m, _ = keys(t, m, "enter")
			}
			if tt.search != "" {
				m, _ = keys(t, m, "/")
				m = typeText(t, m, tt.search)
				m, _ = keys(t, m, "enter")
			}
			m, _ = keys(t, m, tt.keys...)
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

func TestViewEmpty(t *testing.T) {
	for _, size := range [][2]int{{0, 5}, {40, 0}} {
		if v := New(WithSize(size[0], size[1])).View(); v != "" {
			t.Errorf("View() at %v = %q, want empty", size, v)
		}
	}
}

// Every size renders exactly its width and height, scrolled or wrapped.
func TestViewFits(t *testing.T) {
	text := goSource + strings.Repeat("你好, 世界 👋🏽 é ", 20) + "\n"
	for _, wrap := range []bool{false, true} {
		for _, w := range []int{1, 2, 3, 5, 10, 40, 80} {
			for _, h := range []int{1, 2, 3, 12} {
				t.Run(strconv.FormatBool(wrap)+"/"+strconv.Itoa(w)+"x"+strconv.Itoa(h), func(t *testing.T) {
					m := open(t, "main.go", text, WithSize(w, h), WithWrap(wrap))
					for _, k := range []string{"", "l", "l", "j", "G", "l"} {
						if k != "" {
							m, _ = keys(t, m, k)
						}
						assertFits(t, m.View(), w, h)
					}
				})
			}
		}
	}
}

// The view shows the text it was given, whatever the tokens and matches
// that split it.
func TestViewKeepsText(t *testing.T) {
	m := open(t, "main.go", goSource, WithSize(120, 20), WithLineNumbers(false))
	m, _ = keys(t, m, "/", "i", "enter")
	got := strings.Split(plain(m), "\n")
	for i, want := range strings.Split(strings.TrimSuffix(termtext.Clean(goSource, 4), "\n"), "\n") {
		if strings.TrimRight(got[i], " ") != want {
			t.Errorf("line %d = %q, want %q", i+1, got[i], want)
		}
	}
}

// A new syntax style applies without highlighting again.
func TestViewRestyles(t *testing.T) {
	m := open(t, "main.go", goSource, WithSize(60, 12))
	dark := m.View()
	m.SetStyles(DefaultStyles(false))
	if m.View() == dark || ansi.Strip(m.View()) != ansi.Strip(dark) {
		t.Error("the light styles didn't change only the colors")
	}
}

func TestViewWrapsMessages(t *testing.T) {
	m := New(WithSize(20, 4))
	m.SetMessage("big.go", "This diff is too large to show here.")
	v := m.View()
	assertFits(t, v, 20, 4)
	lines := strings.Split(ansi.Strip(v), "\n")
	if got := strings.TrimSpace(lines[0]) + " " + strings.TrimSpace(lines[1]); got != "This diff is too large to show here." {
		t.Errorf("message reads %q over two lines, want it whole", got)
	}
	// What doesn't fit the height is cut, and the status line stays.
	m.SetSize(8, 2)
	if v := m.View(); !strings.Contains(ansi.Strip(v), "big.go") {
		t.Errorf("status line lost: %q", ansi.Strip(v))
	}
}

func TestErrorText(t *testing.T) {
	forbidden := func(error) (string, string) { return "You don't have access to eggzec/x", "o to open on GitHub" }
	tests := []struct {
		name string
		opts []Option
		want string
	}{
		{"default", nil, "✗ Couldn't load: 404 Not Found"},
		{"custom", []Option{WithErrorText(forbidden)}, "✗ You don't have access to eggzec/x · o to open on GitHub"},
		{"empty", []Option{WithErrorText(func(error) (string, string) { return "", "" })}, ""},
		{"text with a dot", []Option{WithErrorText(func(error) (string, string) { return "GitHub says a · b", "" })}, "✗ GitHub says a · b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(append([]Option{WithSize(80, 4)}, tt.opts...)...)
			m.SetError("main.go", errors.New("404 Not Found\nmore"))
			lines := strings.Split(ansi.Strip(m.View()), "\n")
			if got := strings.TrimRight(lines[0], " "); got != tt.want {
				t.Errorf("first row = %q, want %q", got, tt.want)
			}
			if got := lines[len(lines)-1]; !strings.HasPrefix(got, "main.go") {
				t.Errorf("status line = %q, want the name", got)
			}
		})
	}
}

// The hint of a failed load stays whole at any size that can hold it,
// after the text or on a line of its own, and the text gives way to it.
func TestErrorKeepsTheHintWhole(t *testing.T) {
	const hint = "o to open on GitHub"
	say := func(error) (string, string) {
		return "GitHub says the token can't read this organization's repositories until SSO allows it", hint
	}
	for height := 2; height <= 6; height++ {
		for width := 1; width <= 120; width++ {
			m := New(WithSize(width, height), WithErrorText(say))
			m.SetError("main.go", errors.New("boom"))
			v := m.View()
			assertFits(t, v, width, height)
			if width < len(hint) {
				continue
			}
			lines := strings.Split(ansi.Strip(v), "\n")
			body := strings.Join(lines[:height-1], "\n")
			if !strings.Contains(body, hint) {
				t.Errorf("at %dx%d the hint is cut:\n%s", width, height, body)
			}
			if height > 2 && width > 20 && !strings.Contains(body, "✗ GitHub") {
				t.Errorf("at %dx%d the text is lost:\n%s", width, height, body)
			}
		}
	}
}

// The words of a failed load are asked for once, as it fails, not on
// every render.
func TestErrorTextWordedOnce(t *testing.T) {
	calls := 0
	m := New(WithSize(60, 4), WithErrorText(func(error) (string, string) {
		calls++
		return "Can't reach GitHub", ""
	}))
	m.SetError("main.go", errors.New("boom"))
	for range 3 {
		_ = m.View()
	}
	m.SetSize(40, 6)
	_ = m.View()
	if calls != 1 {
		t.Errorf("asked for the words %d times, want once", calls)
	}
}

// TestSetErrorText checks that new words for a failed load show at once.
func TestSetErrorText(t *testing.T) {
	m := New(WithSize(60, 4))
	m.SetError("main.go", errors.New("boom"))
	m.SetErrorText(func(error) (string, string) { return "Can't reach GitHub", "r to retry" })
	if v := ansi.Strip(m.View()); !strings.Contains(v, "Can't reach GitHub · r to retry") {
		t.Errorf("View() = %q, want the new words", v)
	}
}

// The status line ends a cut name with the ellipsis of the styles.
func TestViewEllipsis(t *testing.T) {
	st := DefaultStyles(true)
	st.Ellipsis = "..."
	name := strings.Repeat("a-very-long-directory/", 6) + "main.go"
	m := open(t, name, goSource, WithSize(40, 4), WithStyles(st))
	lines := strings.Split(plain(m), "\n")
	if last := lines[len(lines)-1]; !strings.Contains(last, "...") || strings.Contains(last, "…") {
		t.Errorf("status line %q, want the name cut with ...", last)
	}
}
