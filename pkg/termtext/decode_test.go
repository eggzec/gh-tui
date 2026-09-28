package termtext

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
)

func TestDecode(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{name: "ascii", in: "plain text\n", want: "plain text\n"},
		{name: "utf-8", in: "Grüße, 你好 👋\n", want: "Grüße, 你好 👋\n"},
		{name: "utf-8 bom", in: "\xef\xbb\xbfpackage main", want: "package main"},
		{name: "latin-1", in: "Gr\xfc\xdfe aus K\xf6ln, \xe0 bient\xf4t\n", want: "Grüße aus Köln, à bientôt\n"},
		{name: "windows-1252", in: "\x93quoted\x94 \x80 5 \x96 it\x92s\x85", want: "“quoted” € 5 – it’s…"},
		{name: "undefined in windows-1252", in: "caf\xe9 \x81\x8d\x8f\x90\x9d", want: "café <81><8D><8F><90><9D>"},
		{name: "mostly utf-8", in: "Grüße, 你好 \xff and \xfe", want: "Grüße, 你好 <FF> and <FE>"},
		{name: "cut utf-8", in: "你好世界 \xe4\xbd", want: "你好世界 <E4><BD>"},
		{name: "utf-16le", in: "\xff\xfeh\x00i\x00 \x00`O}Y=\xd8K\xdc\n\x00", want: "hi 你好👋\n"},
		{name: "utf-16be", in: "\xfe\xff\x00h\x00i\x00 O`Y}\xd8=\xdcK\x00\n", want: "hi 你好👋\n"},
		{name: "utf-16 odd byte", in: "\xff\xfeh\x00i", want: "h<69>"},
		{name: "utf-16 lone surrogate", in: "\xff\xfe=\xd8h\x00", want: "�h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Decode(tt.in)
			if got != tt.want {
				t.Errorf("Decode(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("Decode(%q) = %q, which isn't UTF-8", tt.in, got)
			}
		})
	}
}

// Decoded text keeps the widths of its characters: a Latin-1 letter is
// one cell, and CJK two.
func TestDecodeWidths(t *testing.T) {
	for _, tt := range []struct {
		in    string
		width int
	}{
		{"K\xf6ln", 4},
		{"你好 \xff", 9},
		{"\xff\xfe`O}Y", 4},
	} {
		if w := ansi.StringWidth(Clean(Decode(tt.in), 4)); w != tt.width {
			t.Errorf("%q is %d cells wide, want %d", tt.in, w, tt.width)
		}
	}
}

// Decode always returns valid UTF-8, and valid UTF-8 without a byte order
// mark as it is.
func FuzzDecode(f *testing.F) {
	for _, s := range []string{"plain", "Gr\xfc\xdfe", "你好\xff", "\xff\xfeh\x00", "\xfe\xff\x00h", "\xef\xbb\xbfx", "\x81\x9d"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := Decode(s)
		if !utf8.ValidString(got) {
			t.Fatalf("Decode(%q) = %q, which isn't UTF-8", s, got)
		}
		if utf8.ValidString(s) && !UTF16(s) && !strings.HasPrefix(s, bomUTF8) && got != s {
			t.Fatalf("Decode(%q) = %q, want it as it is", s, got)
		}
	})
}
