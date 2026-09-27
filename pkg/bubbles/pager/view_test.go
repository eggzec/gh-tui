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

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		file, text    string
		opts          []Option
		width, height int
		keys          []string
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
		{name: "scrolled down", file: "lines.txt", text: numbered(40), width: 30, height: 8, keys: []string{"d"}},
		{name: "at the end", file: "lines.txt", text: numbered(40), width: 30, height: 8, keys: []string{"G"}},
		{name: "scrolled sideways", file: "main.go", text: goSource, width: 40, height: 12, keys: []string{"l", "l", "l"}},
		// The scroll cuts a wide rune in half, which leaves a blank.
		{name: "cuts wide runes", file: "a.txt", text: "a你好世界\n", width: 6, height: 2,
			opts: []Option{WithLineNumbers(false)}, keys: []string{"l", "l"}},
		{name: "wrapped", file: "main.go", text: goSource, width: 40, height: 14, keys: []string{"w"}},
		{name: "wrapped scrolled into a line", file: "main.go", text: goSource, width: 40, height: 6,
			keys: []string{"w", "j", "j", "j", "j", "j"}},
		{name: "wraps wide runes", file: "a.txt", text: "abc你好世界\n", width: 5, height: 5,
			opts: []Option{WithLineNumbers(false), WithWrap(true)}},
		{name: "search", file: "main.go", text: goSource, width: 60, height: 12, search: "main"},
		{name: "next match", file: "main.go", text: goSource, width: 60, height: 12, search: "main", keys: []string{"n"}},
		{name: "no matches", file: "main.go", text: goSource, width: 60, height: 12, search: "kiwi"},
		{name: "search input", file: "main.go", text: goSource, width: 60, height: 12, keys: []string{"/", "f", "m"}},
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
