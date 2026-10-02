package prompt

import (
	"strconv"
	"strings"
	"testing"
	"unicode"

	"charm.land/lipgloss/v2"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

const longComment = "This reproduces on main too. The config loader returns before " +
	"it reads the defaults, so every field ends up empty and the app starts " +
	"with no keys at all."

func TestView(t *testing.T) {
	tests := []struct {
		name          string
		opts          []Option
		width, height int
		blurred       bool
		typed         string
	}{
		{name: "placeholder", width: 60, height: 6,
			opts: []Option{WithTitle("Comment on #999"), WithPlaceholder("Write a comment in markdown…")}},
		{name: "typed", width: 60, height: 6, typed: "Thanks, **fixed** on main.",
			opts: []Option{WithTitle("Comment on #999")}},
		{name: "wraps long text", width: 40, height: 7, typed: longComment,
			opts: []Option{WithTitle("Comment on #999")}},
		{name: "scrolls when full", width: 40, height: 4, typed: longComment,
			opts: []Option{WithTitle("Comment on #999")}},
		{name: "single line", width: 60, height: SingleLineHeight,
			opts: []Option{WithMode(SingleLine), WithTitle("Labels of #999"), WithValue("bug, help wanted")}},
		{name: "single line scrolls sideways", width: 24, height: SingleLineHeight,
			opts: []Option{WithMode(SingleLine), WithTitle("Labels"), WithValue("bug, help wanted, good first issue")}},
		{name: "single line with extra rows", width: 40, height: 5,
			opts: []Option{WithMode(SingleLine), WithTitle("Labels"), WithPlaceholder("bug, ui")}},
		{name: "blurred", width: 60, height: 5, blurred: true,
			opts: []Option{WithTitle("Comment on #999"), WithValue("Draft")}},
		{name: "light", width: 60, height: 5,
			opts: []Option{WithTitle("Comment on #999"), WithValue("Draft"), WithStyles(DefaultStyles(false))}},
		{name: "long title is cut", width: 30, height: 4,
			opts: []Option{WithTitle("Comment on a very long issue title that can't fit")}},
		{name: "two rows drop the hint", width: 40, height: 2,
			opts: []Option{WithTitle("Comment"), WithValue("Draft")}},
		{name: "one row keeps the input", width: 40, height: 1,
			opts: []Option{WithTitle("Comment"), WithValue("Draft")}},
		{name: "narrower than the frame", width: 2, height: 3,
			opts: []Option{WithTitle("Comment"), WithValue("Draft")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(append(tt.opts, WithSize(tt.width, tt.height))...)
			if !tt.blurred {
				m.Focus()
			}
			m = typeText(t, m, tt.typed)
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

// Every size renders exactly its width and height, in both modes.
func TestViewFits(t *testing.T) {
	for _, mode := range []Mode{MultiLine, SingleLine} {
		for _, w := range []int{1, 2, 3, 10, 40, 80, 200} {
			for _, h := range []int{1, 2, 3, 4, 12} {
				t.Run(strconv.Itoa(int(mode))+"/"+strconv.Itoa(w)+"x"+strconv.Itoa(h), func(t *testing.T) {
					m := focused(t, WithMode(mode), WithTitle("Title"), WithSize(w, h))
					m.SetValue(longComment)
					assertFits(t, m.View(), w, h)
					m.SetSize(h*7, w%5+1)
					assertFits(t, m.View(), h*7, w%5+1)
				})
			}
		}
	}
}

func TestViewFollowsFocus(t *testing.T) {
	m := New(WithTitle("T"), WithSize(20, 3))
	blurred := m.View()
	m.Focus()
	if m.View() == blurred {
		t.Error("focus didn't change the view")
	}
	m.Blur()
	if m.View() != blurred {
		t.Error("blur didn't restore the view")
	}
}

func assertFits(t *testing.T, v string, width, height int) {
	t.Helper()
	lines := strings.Split(v, "\n")
	if len(lines) != height {
		t.Fatalf("view has %d lines, want %d:\n%s", len(lines), height, v)
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != width {
			t.Errorf("line %d is %d wide, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}

// A value set before the size scrolls again once the input has room.
func TestViewScrollsAgainOnResize(t *testing.T) {
	m := New(WithMode(SingleLine), WithValue("bug, help wanted"))
	m.SetSize(40, SingleLineHeight)
	if v := ansi.Strip(m.View()); !strings.Contains(v, "bug, help wanted") {
		t.Errorf("the value isn't in view after a resize:\n%s", v)
	}
}

// A prompt with an ASCII edge, separator and ellipsis is ASCII alone,
// however narrow.
func TestViewASCII(t *testing.T) {
	st := DefaultStyles(true)
	edge := lipgloss.Border{Left: "|"}
	st.Frame = st.Frame.Border(edge, false, false, false, true)
	st.BlurredFrame = st.BlurredFrame.Border(edge, false, false, false, true)
	st.Separator, st.Ellipsis = " - ", "..."
	for _, width := range []int{12, 30, 80} {
		m := New(WithStyles(st), WithTitle("Comment on the pull request"), WithValue("text"), WithSize(width, 6))
		m.Focus()
		if v := ansi.Strip(m.View()); strings.ContainsFunc(v, func(r rune) bool { return r > unicode.MaxASCII }) {
			t.Errorf("width %d: view isn't ASCII:\n%s", width, v)
		}
	}
}
