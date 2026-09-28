package logview

import (
	"errors"
	"strconv"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	failed := WithFocusFailed(true)
	tests := []struct {
		name          string
		opts          []Option
		width, height int
		keys          []string
		search        string
		after         []string
		set           func(*Model)
	}{
		{name: "80 columns", width: 80, height: 24},
		{name: "80 columns failed", width: 80, height: 24, opts: []Option{failed}},
		{name: "140 columns failed", width: 140, height: 30, opts: []Option{failed, WithTimeMode(TimeRelative)}},
		{name: "collapsed", width: 80, height: 16, keys: []string{"*"}},
		{name: "times of day", width: 100, height: 12, opts: []Option{failed, WithTimeMode(TimeAbsolute)}},
		{name: "wrapped", width: 80, height: 24, opts: []Option{failed}, keys: []string{"s"}},
		{name: "scrolled sideways", width: 80, height: 12, opts: []Option{failed}, keys: []string{"l", "l"}},
		{name: "search", width: 80, height: 16, keys: []string{"*"}, search: "unused"},
		{name: "next match", width: 80, height: 16, keys: []string{"*"}, search: "unused", after: []string{"n"}},
		{name: "no matches", width: 80, height: 6, search: "kiwi"},
		{name: "search input", width: 80, height: 6, keys: []string{"/", "c", "a"}},
		{name: "warning", width: 80, height: 10, keys: []string{"*", "w"}},
		{name: "commands expanded", width: 80, height: 12, keys: []string{"j", "j", "j", "j", "j", "j", "j"},
			set: func(m *Model) { m.ExpandAll() }},
		{name: "light", width: 80, height: 12, opts: []Option{failed, WithStyles(DefaultStyles(false))}},
		{name: "no styles", width: 80, height: 12, opts: []Option{failed, WithStyles(Styles{})}},
		{name: "without line numbers", width: 80, height: 12, opts: []Option{failed, WithLineNumbers(false)}},
		{name: "blurred", width: 80, height: 8, set: func(m *Model) { m.Blur() }},
		{name: "live", width: 80, height: 8, keys: []string{"G"}, set: func(m *Model) {
			m.Append(Line{Text: "\x1b[32mstill running\x1b[0m"})
		}},
		{name: "narrow", width: 12, height: 6, opts: []Option{failed}},
		{name: "one row", width: 40, height: 1},
		{name: "loading", width: 40, height: 4, set: func(m *Model) { _ = m.SetLoading() }},
		{name: "failed", width: 40, height: 4,
			set: func(m *Model) { m.SetError(errors.New("410 Gone: the log expired\nmore")) }},
		{name: "empty log", width: 40, height: 4, set: func(m *Model) { m.SetLines(nil, nil) }},
		{name: "nothing", width: 40, height: 4, set: func(m *Model) { *m = New(WithSize(40, 4)) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, append(tt.opts, WithSize(tt.width, tt.height))...)
			if tt.set != nil {
				tt.set(&m)
			}
			m, _ = keys(t, m, tt.keys...)
			if tt.search != "" {
				m = find(t, m, tt.search)
			}
			m, _ = keys(t, m, tt.after...)
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

// Every size renders exactly its width and height, scrolled or wrapped,
// with times and line numbers.
func TestViewFits(t *testing.T) {
	for _, wrap := range []bool{false, true} {
		for _, w := range []int{1, 2, 3, 5, 10, 25, 40, 80, 140} {
			for _, h := range []int{1, 2, 3, 12} {
				t.Run(strconv.FormatBool(wrap)+"/"+strconv.Itoa(w)+"x"+strconv.Itoa(h), func(t *testing.T) {
					m := open(t, WithSize(w, h), WithWrap(wrap), WithTimeMode(TimeRelative), WithFocusFailed(true))
					m.Append(Line{Text: strings.Repeat("你好, 世界 👋🏽 é ", 20)})
					for _, k := range []string{"", "l", "l", "e", "j", "*", "G", "l", "t", "#", "*", "k"} {
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

// The view shows the text of every line it was given, whatever the colors
// and matches that split it.
func TestViewKeepsText(t *testing.T) {
	m := open(t, WithSize(400, 250), WithLineNumbers(false))
	m.ExpandAll()
	m = find(t, m, "e")
	got := strings.Split(plain(m), "\n")
	for i, r := range m.vis {
		row := &m.rows[r]
		if row.fold >= 0 {
			continue
		}
		// The cursor, the mark and a space come before the text.
		text := strings.TrimRight(ansi.Cut(got[i], 3+indent(row), 400), " ")
		if !strings.HasPrefix(row.text, text) || len(text) < min(len(strings.TrimRight(row.text, " ")), 300) {
			t.Errorf("row %d = %q, want %q", i, text, row.text)
		}
	}
}

// The log's colors come through, under the view's styles, and come back
// after a match hides them.
func TestViewColors(t *testing.T) {
	st := Styles{Match: lipgloss.NewStyle().Reverse(true), CurrentMatch: lipgloss.NewStyle().Reverse(true)}
	m := view(t, []Line{{Text: "\x1b[31mabc def\x1b[0m ghi"}}, WithSize(40, 2), WithStyles(st), WithLineNumbers(false))
	if v := m.View(); !strings.Contains(v, "\x1b[31mabc def\x1b[m ghi") {
		t.Errorf("colors lost: %q", v)
	}
	m = find(t, m, "c d")
	if v := m.View(); !strings.Contains(v, "\x1b[31mab\x1b[m\x1b[7mc d\x1b[m\x1b[31mef\x1b[m ghi") {
		t.Errorf("match over colors: %q", v)
	}
	// Scrolled into the red, the view starts with it.
	m = view(t, []Line{{Text: "\x1b[31m" + strings.Repeat("x", 60)}}, WithSize(40, 2), WithStyles(st), WithLineNumbers(false))
	m, _ = keys(t, m, "l")
	if v := m.View(); !strings.Contains(v, "\x1b[31mxxx") {
		t.Errorf("scrolled past the color: %q", v)
	}
}

// Error lines keep their own colors over the error style.
func TestViewErrorStyle(t *testing.T) {
	m := view(t, []Line{{Kind: Error, Text: "plain \x1b[1mbold\x1b[0m after"}}, WithSize(40, 2), WithLineNumbers(false))
	base := m.esc.kinds[Error].on
	v := m.View()
	if !strings.Contains(v, base+"plain \x1b[m"+base+"\x1b[1mbold\x1b[m"+base+" after") {
		t.Errorf("error line %q, want its bold over the error style %q", v, base)
	}
	if !strings.Contains(ansi.Strip(v), errorLineGlyph) {
		t.Errorf("error line without its mark: %q", ansi.Strip(v))
	}
}

func TestErrorText(t *testing.T) {
	offline := func(error) (string, string) { return "Can't reach GitHub", "r to retry" }
	tests := []struct {
		name string
		opts []Option
		want string
	}{
		{"default", nil, "✗ Couldn't load the log: 410 Gone"},
		{"custom", []Option{WithErrorText(offline)}, "✗ Can't reach GitHub · r to retry"},
		{"empty", []Option{WithErrorText(func(error) (string, string) { return "", "" })}, ""},
		{"text with a dot", []Option{WithErrorText(func(error) (string, string) { return "GitHub says a · b", "" })}, "✗ GitHub says a · b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(append([]Option{WithSize(60, 4)}, tt.opts...)...)
			m.SetError(errors.New("410 Gone\nmore"))
			first, _, _ := strings.Cut(ansi.Strip(m.View()), "\n")
			if got := strings.TrimRight(first, " "); got != tt.want {
				t.Errorf("first row = %q, want %q", got, tt.want)
			}
		})
	}
}

// The hint of a failed load stays whole at any size that can hold it,
// after the text or on a line of its own, and the text gives way to it.
func TestErrorKeepsTheHintWhole(t *testing.T) {
	const hint = "r to retry"
	say := func(error) (string, string) {
		return "GitHub says the token can't read this organization's repositories until SSO allows it", hint
	}
	for height := 2; height <= 6; height++ {
		for width := 1; width <= 120; width++ {
			m := New(WithSize(width, height), WithErrorText(say))
			m.SetError(errors.New("boom"))
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
		return "Can't reach GitHub", "r to retry"
	}))
	m.SetError(errors.New("boom"))
	for range 3 {
		_ = m.View()
	}
	if calls != 1 {
		t.Errorf("asked for the words %d times, want once", calls)
	}
}
