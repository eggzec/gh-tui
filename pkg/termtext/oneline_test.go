package termtext

import (
	"testing"
	"unicode/utf8"
)

func TestOneLine(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"plain", "plain", "plain"},
		{"lines and tabs", "a\nb\tc", "a b c"},
		{"escape sequences", "a\r\n\x1b[31mb\x1b[2J\x1b]0;title\ac", "a bc"},
		{"links", "a\x1b]8;;https://evil.test\x1b\\b\x1b]8;;\x1b\\c", "abc"},
		{"c1 controls", "a\u009b2Jb\u009d0;t\u0007c", "a 2Jb 0;t c"},
		// A byte of C1 is invalid UTF-8, which parsers of escape sequences
		// may read as the control.
		{"c1 bytes", "a\x9b2Jb\x9d0;t\x07c", "a\ufffd2Jb\ufffd0;t c"},
		{"c1 bytes after a lead byte", "\xe2\x9b2Jb", "\ufffd2Jb"},
		{"osc 8 after a lead byte", "\xc3\xa9\xe2\x9d8;;https://e\x9cX", "\u00e9\ufffd8;;https://e\ufffdX"},
		{"invalid utf-8", "a\xff\xfeb", "a\ufffdb"},
		{"continuation byte", "a\x80b", "a\ufffdb"},
		{"unfinished lead byte", "a\xe2b", "a\ufffdb"},
		{"unfinished at the end", "x\xe2\x80", "x\ufffd"},
		{"bidi controls", "CI \u202efdp.exe\u2066x\u2069", "CI fdp.exex"},
		{"other format characters", "a\u200bb\ufeffc", "abc"},
		{"joiners stay", "👩\u200d💻", "👩\u200d💻"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OneLine(tt.in); got != tt.want {
				t.Errorf("OneLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// OneLine never returns what a terminal could read as a control: invalid
// UTF-8, C0 or C1 controls, ESC, or format characters but the joiners.
func FuzzOneLine(f *testing.F) {
	for _, s := range []string{
		"plain", "a\x1b[2Jb", "a\x9b2Jb", "\xe2\x9b2Jb", "\xc3\xa9\xe2\x9d8;;https://e\x9cX",
		"a\x80b", "a\xe2b", "x\xe2\x80", "\u202eexe\u2066", "\x1b]8;;https://evil.test\x1b\\x\x1b]8;;\x1b\\",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		got := OneLine(s)
		if !utf8.ValidString(got) {
			t.Fatalf("OneLine(%q) = %q, which is invalid UTF-8", s, got)
		}
		for _, r := range got {
			if isControl(r) || isHidden(r) {
				t.Fatalf("OneLine(%q) = %q, which holds %U", s, got, r)
			}
		}
	})
}
