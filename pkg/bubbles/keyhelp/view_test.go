package keyhelp

import (
	"strconv"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

func TestView(t *testing.T) {
	light := WithStyles(DefaultStyles(false))
	tests := []struct {
		name          string
		opts          []Option
		typed         string
		keys          []tea.Msg
		width, height int
	}{
		{name: "grouped", width: 80, height: 20},
		{name: "grouped light", opts: []Option{light}, width: 80, height: 20},
		{name: "fuzzy filtered", typed: "rld", width: 80, height: 10},
		{name: "capturing", keys: []tea.Msg{tab}, width: 80, height: 10},
		{name: "captured ctrl+r", keys: []tea.Msg{tab, ctrlR}, width: 80, height: 10},
		{name: "captured ctrl+r light", opts: []Option{light}, keys: []tea.Msg{tab, ctrlR}, width: 80, height: 10},
		{name: "key kept after capture", keys: []tea.Msg{tab, esc, tab}, width: 80, height: 10},
		{name: "no match", typed: "zzz", width: 80, height: 5},
		{name: "scrolled", keys: []tea.Msg{pgDown}, width: 80, height: 8},
		{name: "narrow", width: 40, height: 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := open(t, append(tt.opts, WithSize(tt.width, tt.height))...)
			m = typeText(t, m, tt.typed)
			m, _ = press(t, m, tt.keys...)
			v := m.View()
			assertFits(t, v, tt.width, tt.height)
			golden.RequireEqual(t, v)
		})
	}
}

// Disabled and typed rows are dimmed, and the rows that lose a key are
// marked in the color of how they lose it.
func TestViewMarks(t *testing.T) {
	s := DefaultStyles(true)
	typing := []Layer{
		{Source: "finder", Typing: true, Bindings: []key.Binding{bind("open", "enter")}},
		{Source: "list", Bindings: []key.Binding{bind("reload", "r")}},
	}
	m := open(t)
	v := m.View()
	for _, want := range []string{
		s.Conflict.Render(warnGlyph),
		s.Shadowed.Render(warnGlyph),
		s.Disabled.Render("merge"),
		s.Shadowed.Render(lossGlyph + " ctrl+r: refresh · app"),
		s.Conflict.Render(lossGlyph + " esc: cancel · pull requests"),
	} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks %q:\n%s", want, v)
		}
	}
	m = New(WithLayers(typing), WithSize(80, 10))
	v = m.View()
	for _, want := range []string{s.Disabled.Render("reload"), s.Typed.Render(lossGlyph + " r: typed in finder")} {
		if !strings.Contains(v, want) {
			t.Errorf("typed view lacks %q:\n%s", want, v)
		}
	}
}

// Every width renders exactly its width, at a height with every part and
// at the heights that drop parts.
func TestViewFits(t *testing.T) {
	for w := range 201 {
		for _, h := range []int{0, 1, 2, 12} {
			m := open(t, WithSize(w, h))
			assertFits(t, m.View(), w, h)
			m = typeText(t, m, "re")
			m, _ = press(t, m, tab, ctrlR)
			assertFits(t, m.View(), w, h)
			if t.Failed() {
				t.Fatalf("stopped at %s", strconv.Itoa(w)+"x"+strconv.Itoa(h))
			}
		}
	}
}

// Text from the layers can't break the layout.
func TestViewCleansText(t *testing.T) {
	m := New(WithLayers([]Layer{{Source: "two\nlines", Bindings: []key.Binding{bind("red \x1b[31mtext\x1b[m\tand tab", "x")}}}), WithSize(80, 5))
	v := ansi.Strip(m.View())
	if !strings.Contains(v, "red text and tab") || !strings.Contains(v, "two lines") {
		t.Errorf("the row isn't on one clean line:\n%s", v)
	}
}

// A help changed while blurred, as the parent resizes it or restyles it
// while it is closed, shows the same as one changed while open, once it
// opens, and a view of it before then is right too.
func TestBlurredHelpListsWhenShown(t *testing.T) {
	open := New(WithLayers(layers()), WithSize(60, 12))
	open.Focus()
	open.SetSize(80, 20)
	open.SetStyles(DefaultStyles(false))
	open.SetQuery("r")

	closed := New(WithLayers(layers()), WithSize(60, 12))
	closed.SetSize(80, 20)
	closed.SetStyles(DefaultStyles(false))
	closed.SetQuery("r")
	blurred := open
	blurred.Blur()
	if got, want := closed.View(), blurred.View(); got != want {
		t.Errorf("blurred view:\n%s\nwant:\n%s", got, want)
	}
	closed.Focus()
	if got, want := closed.View(), open.View(); got != want {
		t.Errorf("view once focused:\n%s\nwant:\n%s", got, want)
	}
}
