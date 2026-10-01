package logview

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name, in, want string
		marks          []termtext.Style
	}{
		{name: "plain", in: "ok 1.2s", want: "ok 1.2s"},
		{name: "unicode", in: "✓ passed 你好", want: "✓ passed 你好"},
		{name: "crlf", in: "done\r\n", want: "done"},
		{name: "keeps colors", in: "\x1b[36;1mgo test\x1b[0m", want: "go test",
			marks: []termtext.Style{{Pos: 0, Seq: "\x1b[1;36m"}, {Pos: 7}}},
		{name: "splits a reset from what follows", in: "a\x1b[0;31mb", want: "ab",
			marks: []termtext.Style{{Pos: 1, Seq: "\x1b[31m"}}},
		{name: "an empty parameter resets", in: "\x1b[;1mb", want: "b",
			marks: []termtext.Style{{Pos: 0, Seq: "\x1b[1m"}}},
		{name: "a zero in a color is no reset", in: "\x1b[38;5;0mx\x1b[38;2;255;0;0my", want: "xy",
			marks: []termtext.Style{{Pos: 0, Seq: "\x1b[38;5;0m"}, {Pos: 1, Seq: "\x1b[38;2;255;0;0m"}}},
		{name: "colon colors", in: "\x1b[38:2::255:0:0mx", want: "x",
			marks: []termtext.Style{{Pos: 0, Seq: "\x1b[38;2;255;0;0m"}}},
		{name: "tabs", in: "a\tb\t\tc", want: "a       b               c"},
		{name: "tabs after wide runes", in: "你\tx", want: "你      x"},
		{name: "tabs after colors", in: "\x1b[1ma\x1b[m\tb", want: "a       b",
			marks: []termtext.Style{{Pos: 0, Seq: "\x1b[1m"}, {Pos: 1}}},
		{name: "carriage return overwrites", in: "10%\r50%\r100%", want: "100%"},
		{name: "invalid utf-8", in: "a\xffb", want: "a�b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, marks := sanitize(tt.in, 8)
			if got != tt.want {
				t.Errorf("text = %q, want %q", got, tt.want)
			}
			if !slices.Equal(marks, tt.marks) {
				t.Errorf("marks = %+v, want %+v", marks, tt.marks)
			}
		})
	}
}

// Nothing but SGR reaches the terminal: no cursor moves, screen clears,
// titles, hyperlinks, mode changes, device strings or control characters.
func TestSanitizeStripsSequences(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{name: "cursor moves", in: "a\x1b[2J\x1b[H\x1b[10;20Hb\x1b[3Ac", want: "abc"},
		{name: "erase line", in: "progress\x1b[2K\x1b[1Gdone", want: "progressdone"},
		{name: "private modes", in: "\x1b[?1049h\x1b[?25lx\x1b[?1000h", want: "x"},
		{name: "private sgr", in: "\x1b[>4;2mx", want: "x"},
		{name: "sgr with an intermediate", in: "\x1b[1 mx", want: "x"},
		{name: "window title with bel", in: "\x1b]0;pwned\x07x", want: "x"},
		{name: "window title with st", in: "\x1b]2;pwned\x1b\\x", want: "x"},
		{name: "hyperlink keeps its text", in: "\x1b]8;;https://evil.test\x1b\\link\x1b]8;;\x1b\\", want: "link"},
		{name: "clipboard write", in: "\x1b]52;c;cHduZWQ=\x07x", want: "x"},
		{name: "unterminated string", in: "x\x1b]0;never ends", want: "x"},
		{name: "dcs", in: "\x1bP+q544e\x1b\\x", want: "x"},
		{name: "apc pm sos", in: "\x1b_a\x1b\\\x1b^b\x1b\\\x1bXc\x1b\\x", want: "x"},
		{name: "two-byte escapes", in: "\x1bc\x1b7\x1b8\x1b(Bx", want: "x"},
		{name: "lone escape", in: "x\x1b", want: "x"},
		{name: "escape before a control", in: "x\x1b\ay", want: "xy"},
		{name: "cut csi", in: "x\x1b[31", want: "x"},
		{name: "csi cut by text", in: "\x1b[31é", want: "é"},
		{name: "c0 controls", in: "a\a\bb\x00c\x7fd\ve\ff", want: "abcdef"},
		{name: "c1 controls", in: "a\u009b31mb\u0085c\u009d0;x\u009c", want: "a\ufffd31mb\ufffdc\ufffd0;x\ufffd"},
		{name: "bidi controls", in: "a\u202eexe.txt\u202c \u2066b\u2069\u200fc", want: "a\ufffdexe.txt\ufffd \ufffdb\ufffd\ufffdc"},
		{name: "image placeholder", in: "\U0010EEEE\u0305\u0305x", want: "\ufffd\u0305\u0305x"},
		{name: "invisible format", in: "a\u2060b\ufeffc\U000E0041d", want: "abcd"},
		{name: "joiners kept", in: "\U0001F469\u200d\U0001F4BB", want: "\U0001F469\u200d\U0001F4BB"},
		{name: "newline", in: "a\nb", want: "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, marks := sanitize(tt.in, 8)
			if got != tt.want {
				t.Errorf("text = %q, want %q", got, tt.want)
			}
			if len(marks) > 0 {
				t.Errorf("kept %+v, want no marks", marks)
			}
		})
	}
}

// The view writes the log's colors and nothing else of what it sent.
func TestSanitizeView(t *testing.T) {
	lines := []Line{
		{Text: "\x1b]0;pwned\x07\x1b[2J\x1b[31mred\x1b[0m plain \x1b[?1049h"},
		{Text: "\x1b]8;;https://evil.test\x1b\\link\x1b]8;;\x1b\\\x1b[6n"},
	}
	m := view(t, lines, WithSize(40, 3), WithStyles(Styles{}), WithLineNumbers(false))
	v := m.View()
	for _, bad := range []string{"\x1b]", "\x1b[2J", "\x1b[?", "\x1b[6n", "\a", "evil"} {
		if strings.Contains(v, bad) {
			t.Errorf("view holds %q: %q", bad, v)
		}
	}
	if !strings.Contains(v, "\x1b[31mred") {
		t.Errorf("view lost the red: %q", v)
	}
	if got := strings.Fields(ansi.Strip(v)); !slices.Equal(got[:4], []string{"▌", "red", "plain", "link"}) {
		t.Errorf("view reads %q", got)
	}
	assertFits(t, v, 40, 3)
}

// However many colors a log holds, a frame averages at most 72 bytes a
// cell: sequences too long to keep are dropped, the colors since the last
// reset fold into one, and each change writes only what changes. The
// costliest content measured, two opposite styles at every cell, takes
// about 65; 72 leaves room above it. It is an average over the frame, not
// a bound on each cell.
func TestSanitizeViewAverageBytesBounded(t *testing.T) {
	const width, height = 80, 10
	huge := "\x1b[38;2;" + strings.Repeat("1", 60<<10) + "m"
	rows := make([]Line, 0, height*2)
	oppositeRows := make([]Line, 0, height*2)
	for range height * 2 {
		rows = append(rows, Line{Text: padded(width)})
		oppositeRows = append(oppositeRows, Line{Text: opposite(width)})
	}
	for _, lines := range [][]Line{
		{{Text: strings.Repeat("\x1b[1mx", 200_000)}},
		{{Text: strings.Repeat("\x1b[1mx\x1b[3my\x1b[22;23m", 50_000)}},
		{{Text: strings.Repeat(huge+"x", 50)}},
		{{Text: padded(width * height * 2)}},
		rows,
		{{Text: opposite(width * height * 2)}},
		oppositeRows,
	} {
		m := view(t, lines, WithSize(width, height), WithLineNumbers(false))
		start := time.Now()
		v := m.View()
		if d := time.Since(start); d > time.Second {
			t.Errorf("a frame took %v", d)
		}
		if limit := 72 * width * height; len(v) > limit {
			t.Errorf("a frame is %d bytes, more than an average of 72 a cell (%d)", len(v), limit)
		}
		assertFits(t, v, width, height)
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
