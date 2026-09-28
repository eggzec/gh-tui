package termtext

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

// styles returns the styles that the SGR sequences seqs, each at the
// position after it, fold into.
func styles(seqs ...string) []Style {
	var s Styler
	for i, q := range seqs {
		s.Add(i, q)
	}
	return s.Styles()
}

func TestStyler(t *testing.T) {
	tests := []struct {
		name string
		seqs []string
		want []Style
	}{
		{name: "color", seqs: []string{"\x1b[36;1m"}, want: []Style{{0, "\x1b[1;36m"}}},
		{name: "reset", seqs: []string{"\x1b[31m", "\x1b[0m"}, want: []Style{{0, "\x1b[31m"}, {1, ""}}},
		{name: "empty reset", seqs: []string{"\x1b[1m", "\x1b[m"}, want: []Style{{0, "\x1b[1m"}, {1, ""}}},
		{name: "reset first", seqs: []string{"\x1b[1m", "\x1b[0;31m"}, want: []Style{{0, "\x1b[1m"}, {1, "\x1b[31m"}}},
		{name: "empty parameter", seqs: []string{"\x1b[3m", "\x1b[;1m"}, want: []Style{{0, "\x1b[3m"}, {1, "\x1b[1m"}}},
		{name: "styles add up", seqs: []string{"\x1b[1m", "\x1b[31m", "\x1b[4m", "\x1b[44m"},
			want: []Style{{0, "\x1b[1m"}, {1, "\x1b[1;31m"}, {2, "\x1b[1;4;31m"}, {3, "\x1b[1;4;31;44m"}}},
		{name: "offs", seqs: []string{"\x1b[1;2;3;4;7;9;53m", "\x1b[22;23;24;27;29;55m"},
			want: []Style{{0, "\x1b[1;2;3;4;7;9;53m"}, {1, ""}}},
		{name: "a color replaces the last", seqs: []string{"\x1b[31m", "\x1b[32m", "\x1b[39m"},
			want: []Style{{0, "\x1b[31m"}, {1, "\x1b[32m"}, {2, ""}}},
		{name: "extended colors", seqs: []string{"\x1b[38;5;208;48;2;1;2;3;58;5;1m"},
			want: []Style{{0, "\x1b[38;5;208;48;2;1;2;3;58;5;1m"}}},
		{name: "a zero in a color is no reset", seqs: []string{"\x1b[1m", "\x1b[38;5;0m"},
			want: []Style{{0, "\x1b[1m"}, {1, "\x1b[1;38;5;0m"}}},
		{name: "colon colors", seqs: []string{"\x1b[38:2::255:0:0m"}, want: []Style{{0, "\x1b[38:2::255:0:0m"}}},
		{name: "bright colors", seqs: []string{"\x1b[91;103m"}, want: []Style{{0, "\x1b[91;103m"}}},
		{name: "underline styles", seqs: []string{"\x1b[4:3m", "\x1b[4:0m"}, want: []Style{{0, "\x1b[4m"}, {1, ""}}},
		{name: "a cut color drops the rest", seqs: []string{"\x1b[1;38;5m"}, want: []Style{{0, "\x1b[1m"}}},
		// Text that hides or flashes can make a file read other than it
		// is.
		{name: "blink and conceal", seqs: []string{"\x1b[5m", "\x1b[6m", "\x1b[8m", "\x1b[1;5;6;8m"}, want: []Style{{3, "\x1b[1m"}}},
		{name: "unknown parameters", seqs: []string{"\x1b[10;26;50;60;73m"}},
		{name: "nothing changes", seqs: []string{"\x1b[1m", "\x1b[1m", "\x1b[1;1m"}, want: []Style{{0, "\x1b[1m"}}},
		{name: "a change undone", seqs: []string{"\x1b[1m", "\x1b[3m", "\x1b[23m"},
			want: []Style{{0, "\x1b[1m"}, {1, "\x1b[1;3m"}, {2, "\x1b[1m"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := styles(tt.seqs...); !slices.Equal(got, tt.want) {
				t.Errorf("styles = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Sequences at one position fold into one style, or none where they
// leave the style as it was.
func TestStylerSamePosition(t *testing.T) {
	var s Styler
	s.Add(0, "\x1b[1m")
	for range 1000 {
		s.Add(5, "\x1b[3m")
		s.Add(5, "\x1b[23m")
	}
	if got, want := s.Styles(), []Style{{0, "\x1b[1m"}}; !slices.Equal(got, want) {
		t.Errorf("styles = %+v, want %+v", got, want)
	}
}

// However many sequences text holds, the style of any position is one
// short sequence.
func TestStyleBounded(t *testing.T) {
	var s Styler
	for i := range 10_000 {
		s.Add(i, fmt.Sprintf("\x1b[%dm", 1+i%9))
		s.Add(i, fmt.Sprintf("\x1b[38;2;%d;%d;%dm", i%256, i/256%256, i%7))
	}
	for _, st := range s.Styles() {
		if len(st.Seq) > 64 {
			t.Fatalf("style %q is %d bytes", st.Seq, len(st.Seq))
		}
	}
}

func TestEscape(t *testing.T) {
	tests := []struct {
		name, in string
		n        int
		sgr      bool
	}{
		{name: "sgr", in: "\x1b[31mx", n: 5, sgr: true},
		{name: "reset", in: "\x1b[mx", n: 3, sgr: true},
		{name: "colon color", in: "\x1b[38:2::1:2:3mx", n: 14, sgr: true},
		{name: "cursor move", in: "\x1b[10;20Hx", n: 8},
		{name: "private sgr", in: "\x1b[>4;2mx", n: 7},
		{name: "sgr with an intermediate", in: "\x1b[1 mx", n: 5},
		{name: "cut csi", in: "\x1b[31", n: 4},
		{name: "csi cut by a newline", in: "\x1b[31\nx", n: 4},
		{name: "title with bel", in: "\x1b]0;t\ax", n: 6},
		{name: "title with st", in: "\x1b]0;t\x1b\\x", n: 7},
		{name: "unterminated title", in: "\x1b]0;t", n: 5},
		{name: "title ends at a newline", in: "\x1b]0;t\nnext", n: 5},
		{name: "two bytes", in: "\x1bcx", n: 2},
		{name: "charset", in: "\x1b(Bx", n: 3},
		{name: "lone escape", in: "\x1b", n: 1},
		{name: "sgr of 64 bytes", in: "\x1b[" + strings.Repeat("1;", 31) + "22m", n: 67, sgr: true},
		{name: "sgr of more than 64 bytes", in: "\x1b[" + strings.Repeat("1;", 32) + "1mx", n: 68},
		{name: "sgr of more than 32 parameters", in: "\x1b[" + strings.Repeat(";", 32) + "mx", n: 35},
		{name: "a huge sgr", in: "\x1b[38;2;" + strings.Repeat("1", 60_000) + "mx", n: 60_008},
	}
	for _, tt := range tests {
		if n, sgr := Escape(tt.in); n != tt.n || sgr != tt.sgr {
			t.Errorf("%s: Escape(%q) = %d, %v, want %d, %v", tt.name, tt.in, n, sgr, tt.n, tt.sgr)
		}
	}
}

func TestHasSGR(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"plain", false},
		{"a\x1b[31mred", true},
		{"a\x1b[mreset", true},
		{"\x1b[2J\x1b[Hmoves", false},
		{"\x1b]0;t\a\x1b[?25l", false},
		{"\x1b[>4;2m", false},
		{"\x1b[2J then \x1b[1m", true},
		{"cut \x1b[31", false},
		{"lone \x1b", false},
	}
	for _, tt := range tests {
		if got := HasSGR(tt.in); got != tt.want {
			t.Errorf("HasSGR(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestCleanStyled(t *testing.T) {
	tests := []struct {
		name, in, want string
		styles         []Style
	}{
		{name: "plain", in: "a\tb\r\nc", want: "a   b\nc"},
		{name: "colors", in: "\x1b[1;32mok\x1b[0m done\n\x1b[31mfail", want: "ok done\nfail",
			styles: []Style{{Pos: 0, Seq: "\x1b[1;32m"}, {Pos: 2}, {Pos: 8, Seq: "\x1b[31m"}}},
		{name: "tabs after colors", in: "\x1b[1ma\x1b[m\tb", want: "a   b",
			styles: []Style{{Pos: 0, Seq: "\x1b[1m"}, {Pos: 1}}},
		{name: "wide runes keep their bytes", in: "你\x1b[31m好", want: "你好",
			styles: []Style{{Pos: 3, Seq: "\x1b[31m"}}},
		// What moves the cursor, clears, titles, links or asks the
		// terminal goes whole, and its text stays.
		{name: "hostile sequences", in: "a\x1b[2J\x1b[H\x1b[10;20Hb\x1b[3A\x1b[?1049h\x1b]0;pwned\a\x1b]52;c;eA==\x1b\\" +
			"\x1b]8;;https://evil.test\x1b\\link\x1b]8;;\x1b\\\x1bP+q\x1b\\\x1b[6n\x1bc\x1b[>4;2mc", want: "ablinkc"},
		{name: "a title never ended stops at the line", in: "a\x1b]0;never\nb", want: "a\nb"},
		{name: "c1 and invalid utf-8", in: "a\u009b31mb\x9bc\xffd", want: "a\ufffd31mb\ufffdc\ufffdd"},
		{name: "bidi controls", in: "\x1b[1ma\u202eb", want: "a\ufffdb", styles: []Style{{Pos: 0, Seq: "\x1b[1m"}}},
		{name: "lone escape", in: "x\x1b", want: "x"},
		{name: "blink and conceal", in: "\x1b[5mblink \x1b[8mhidden\x1b[m", want: "blink hidden"},
		{name: "long sgr", in: "\x1b[38;2;" + strings.Repeat("1", 100) + "mx", want: "x"},
		{name: "nothing to clean", in: "plain\nlines", want: "plain\nlines"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, styles := CleanStyled(tt.in, 4)
			if got != tt.want {
				t.Errorf("text = %q, want %q", got, tt.want)
			}
			if !slices.Equal(styles, tt.styles) {
				t.Errorf("styles = %+v, want %+v", styles, tt.styles)
			}
		})
	}
}
