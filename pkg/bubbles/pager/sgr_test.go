package pager

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/pkg/termtext"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

func TestStyleLines(t *testing.T) {
	red, bold, boldRed := "\x1b[31m", "\x1b[1m", "\x1b[1;31m"
	tests := []struct {
		name string
		text string
		want [][]termtext.Style
	}{
		{name: "a line of its own", text: "a" + red + "b\x1b[0mc\nd",
			want: [][]termtext.Style{{{Pos: 1, Seq: red}, {Pos: 2}}, nil}},
		{name: "colors carry over lines", text: red + "a\nb\n" + bold + "c\x1b[0m\nd",
			want: [][]termtext.Style{{{Seq: red}}, {{Seq: red}}, {{Seq: boldRed}, {Pos: 1}}, nil}},
		{name: "a reset carries what follows it", text: bold + "a\x1b[0;31mb\nc",
			want: [][]termtext.Style{{{Seq: bold}, {Pos: 1, Seq: red}}, {{Seq: red}}}},
		{name: "a color at the end of a line", text: "a" + red + "\nb",
			want: [][]termtext.Style{{{Pos: 1, Seq: red}}, {{Seq: red}}}},
		{name: "a color at the start of a line", text: "a\n" + red + "b",
			want: [][]termtext.Style{nil, {{Seq: red}}}},
		{name: "a carried color and one of its own", text: red + "a\nb" + bold + "c",
			want: [][]termtext.Style{{{Seq: red}}, {{Seq: red}, {Pos: 1, Seq: boldRed}}}},
		{name: "empty lines", text: red + "\n\n\x1b[m",
			want: [][]termtext.Style{{{Seq: red}}, {{Seq: red}}, {{}}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, styles := termtext.CleanStyled(tt.text, 4)
			got := styleLines(strings.Split(text, "\n"), styles)
			if !slices.EqualFunc(got, tt.want, slices.Equal) {
				t.Errorf("styleLines = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A line takes over one style from the lines before it, however many
// colors they held.
func TestStyleLinesCarryOne(t *testing.T) {
	text, styles := termtext.CleanStyled(strings.Repeat("\x1b[1m\x1b[31mab\x1b[3m\n\n", 100), 4)
	for i, l := range styleLines(strings.Split(text, "\n"), styles) {
		if len(l) > 2 {
			t.Fatalf("line %d has %d styles: %+v", i+1, len(l), l)
		}
	}
}

// However many colors the content holds, a frame averages at most 72
// bytes a cell: sequences too long to keep are dropped, the colors before
// a row fold into one style, and each change writes only what changes.
// The costliest content measured, two opposite styles at every cell, takes
// about 65; 72 leaves room above it. It is an average over the frame, not
// a bound on each cell: one cell may take more where others take less.
func TestColorsAverageBytesBounded(t *testing.T) {
	const width, height = 120, 40
	huge := "\x1b[38;2;" + strings.Repeat("1", 60<<10) + "m"
	tests := []struct {
		name, text string
	}{
		{name: "one long line never reset", text: strings.Repeat("\x1b[1mx", 200_000)},
		{name: "a color for each cell", text: strings.Repeat("\x1b[1mx\x1b[3my\x1b[22;23mz", 70_000)},
		{name: "many colors at one place", text: strings.Repeat("\x1b[1m\x1b[3m\x1b[22;23m", 100_000) + "x"},
		{name: "huge sequences", text: strings.Repeat(huge+"x", 50)},
		{name: "many lines never reset", text: strings.Repeat("\x1b[1m\x1b[4m\x1b[31mline\n", 50_000)},
		{name: "padded colors at every cell", text: padded(width * height * 2)},
		{name: "padded colors on every row", text: strings.Repeat(padded(width)+"\n", height*2)},
		{name: "opposite styles at every cell", text: opposite(width * height * 2)},
		{name: "opposite styles on every row", text: strings.Repeat(opposite(width)+"\n", height*2)},
	}
	for _, tt := range tests {
		for _, wrap := range []bool{false, true} {
			m := open(t, "out.log", tt.text, WithSize(width, height), WithWrap(wrap), WithLineNumbers(false))
			m, _ = keys(t, m, "G")
			start := time.Now()
			v := m.View()
			if d := time.Since(start); d > time.Second {
				t.Errorf("%s, wrap %v: a frame took %v", tt.name, wrap, d)
			}
			if limit := 72 * width * height; len(v) > limit {
				t.Errorf("%s, wrap %v: a frame is %d bytes, more than an average of 72 a cell (%d)", tt.name, wrap, len(v), limit)
			}
			assertFits(t, v, width, height)
		}
	}
}

// Content can't hide or flash its text: blink and conceal are dropped.
func TestColorsNoBlinkOrConceal(t *testing.T) {
	m := open(t, "out.log", "\x1b[5mblink\x1b[m \x1b[6mrapid\x1b[m \x1b[8mhidden\x1b[m \x1b[1;8;31mred\n", WithSize(40, 3))
	v := m.View()
	for _, bad := range []string{"\x1b[5", "\x1b[6", "\x1b[8", ";5m", ";6m", ";8m", ";5;", ";6;", ";8;"} {
		if strings.Contains(v, bad) {
			t.Errorf("view holds %q: %q", bad, v)
		}
	}
	if !strings.Contains(v, "\x1b[1;31mred") || !strings.Contains(plain(m), "blink rapid hidden red") {
		t.Errorf("view lost the text or its colors: %q", v)
	}
}

// Colored content shows its colors and nothing else of the escapes it
// holds: no cursor moves, clears, titles, links or queries.
func TestColorsHostile(t *testing.T) {
	text := "\x1b[31mred\x1b[0m " + termtexttest.Hostile + "\n\x1b[2J\x1b[H\x1b[?1049h\x1b]52;c;eA==\a\x1b[6n\x1b[1mbold\n" +
		"\x1b]0;never ended\nnext \x1b[38;2;1;2;3mrgb\x1b[m\n"
	for _, wrap := range []bool{false, true} {
		m := open(t, "out.log", text, WithSize(40, 8), WithWrap(wrap))
		v := m.View()
		termtexttest.AssertClean(t, v, 40)
		for _, want := range []string{"\x1b[31mred", "\x1b[1mbold", "38;2;1;2;3mrgb"} {
			if !strings.Contains(v, want) {
				t.Errorf("wrap %v: view lost %q: %q", wrap, want, v)
			}
		}
		for _, bad := range []string{"\x1b[2J", "\x1b[H", "\x1b[?", "\x1b[6n", "\x1b]0", "\x1b]52", "never ended"} {
			if strings.Contains(v, bad) {
				t.Errorf("wrap %v: view holds %q: %q", wrap, bad, v)
			}
		}
		if !strings.Contains(plain(m), "next rgb") {
			t.Errorf("wrap %v: a title never ended hid the next line:\n%s", wrap, plain(m))
		}
	}
}

// Searches and filters read the text without its colors, and the matches
// show over the right columns.
func TestColorsSearch(t *testing.T) {
	text := "\x1b[1;32mok\x1b[0m  \x1b[31mfa\x1b[1mil-ed\x1b[0m 你好 fail\nplain fail\n"
	m := open(t, "test.log", text, WithSize(40, 4))
	if m.sgr == nil {
		t.Fatal("the colors weren't found")
	}
	m, _ = typeSearch(t, m, "fail")
	if m.Matches() != 3 {
		t.Fatalf("%d matches of fail, want 3", m.Matches())
	}
	v := m.View()
	// The first match starts inside a color and spans another; it hides
	// both, and the colors come back after it.
	cur := "\x1b[m" + m.esc.current.on + "fail"
	if !strings.Contains(v, cur) {
		t.Errorf("view doesn't show %q over fail: %q", cur, v)
	}
	// Bold and red over the text style, in one sequence after a reset.
	if !strings.Contains(v, "fail\x1b[0;1;31m-ed") {
		t.Errorf("the colors don't come back after the match: %q", v)
	}
	if got := strings.Count(v, m.esc.match.on+"fail"); got != 2 {
		t.Errorf("%d other matches shown, want 2: %q", got, v)
	}
	m, _ = enterAll(t, m, "&", "ok", "enter")
	if m.Shown() != 1 {
		t.Errorf("filter ok shows %d lines, want 1", m.Shown())
	}
}

// Colored content isn't highlighted, since it has colors of its own.
func TestColorsNotHighlighted(t *testing.T) {
	m := New(WithSize(40, 4))
	if cmd := m.SetContent("main.go", "\x1b[32mpackage\x1b[m main\n"); cmd != nil {
		t.Error("colored content is highlighted")
	}
	if cmd := m.SetContent("main.go", "package main\n"); cmd == nil {
		t.Error("plain Go isn't highlighted")
	}
	if m.sgr != nil {
		t.Error("plain content kept the colors of the content before it")
	}
}

// padded is n cells of text crafted to make a frame as large as it can:
// every cell changes all three colors, which come in the colon form and
// padded with zeros, as long as a kept sequence may be, and every other
// cell turns every attribute on, and the next turns them off.
func padded(n int) string {
	var b strings.Builder
	pad := func(v int) string { return fmt.Sprintf("%017d", v%256) }
	for i := range n {
		if i%2 == 0 {
			b.WriteString("\x1b[1;2;3;4;7;9;21;53m")
		} else {
			b.WriteString("\x1b[22;23;24;27;29;55m")
		}
		for _, kind := range []string{"38", "48", "58"} {
			b.WriteString("\x1b[" + kind + ":2::" + pad(i) + ":" + pad(i/256) + ":" + pad(i+1) + "m")
		}
		b.WriteString("x")
	}
	return b.String()
}

// opposite is n cells of text that alternate two opposite styles, as
// costly to change between as kept sequences allow: every attribute on
// and off, two underline styles, and three true colors of three-digit
// values, which no color of the other style shares.
func opposite(n int) string {
	styles := [2]string{
		"\x1b[1;2;3;4:3;7;9;53m\x1b[38;2;255;254;253;48;2;252;251;250;58;2;249;248;247m",
		"\x1b[22;23;27;29;55;4:5m\x1b[38;2;246;245;244;48;2;243;242;241;58;2;240;239;238m",
	}
	var b strings.Builder
	for i := range n {
		b.WriteString(styles[i%2] + "x")
	}
	return b.String()
}
