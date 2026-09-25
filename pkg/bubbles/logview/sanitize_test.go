package logview

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name, in, want string
		marks          []mark
	}{
		{name: "plain", in: "ok 1.2s", want: "ok 1.2s"},
		{name: "unicode", in: "✓ passed 你好", want: "✓ passed 你好"},
		{name: "crlf", in: "done\r\n", want: "done"},
		{name: "keeps colors", in: "\x1b[36;1mgo test\x1b[0m", want: "go test",
			marks: []mark{{pos: 0, seq: "\x1b[36;1m"}, {pos: 7, reset: true}}},
		{name: "splits a reset from what follows", in: "a\x1b[0;31mb", want: "ab",
			marks: []mark{{pos: 1, seq: "\x1b[31m", reset: true}}},
		{name: "an empty parameter resets", in: "\x1b[;1mb", want: "b",
			marks: []mark{{pos: 0, seq: "\x1b[1m", reset: true}}},
		{name: "a zero in a color is no reset", in: "\x1b[38;5;0mx\x1b[38;2;255;0;0my", want: "xy",
			marks: []mark{{pos: 0, seq: "\x1b[38;5;0m"}, {pos: 1, seq: "\x1b[38;2;255;0;0m"}}},
		{name: "colon colors", in: "\x1b[38:2::255:0:0mx", want: "x",
			marks: []mark{{pos: 0, seq: "\x1b[38:2::255:0:0m"}}},
		{name: "tabs", in: "a\tb\t\tc", want: "a       b               c"},
		{name: "tabs after wide runes", in: "你\tx", want: "你      x"},
		{name: "tabs after colors", in: "\x1b[1ma\x1b[m\tb", want: "a       b",
			marks: []mark{{pos: 0, seq: "\x1b[1m"}, {pos: 1, reset: true}}},
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
		{name: "c1 controls", in: "a\u009b31mb\u0085c\u009d0;x\u009c", want: "a31mbc0;x"},
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
