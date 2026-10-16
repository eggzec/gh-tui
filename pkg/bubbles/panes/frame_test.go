package panes

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// visible makes escape sequences readable in a golden file.
func visible(s string) string { return strings.ReplaceAll(s, "\x1b", "␛") }

func TestFrame(t *testing.T) {
	st := DefaultStyles(true)
	files := []string{"8 files · +67 −9 · 2 viewed", "8 · +67 −9 · 2 viewed", "8 · +67 −9 · 2✓", "8 · +67 −9"}
	tests := []struct {
		name string
		c    Content
		w, h int
	}{
		{"title and info", Content{Title: "6 Files", Info: files, Body: "src/calc.go\nsrc/calc_test.go"}, 40, 5},
		{"info form 2", Content{Title: "6 Files", Info: files}, 29, 3},
		{"info form 3", Content{Title: "6 Files", Info: files}, 26, 3},
		{"info form 4", Content{Title: "6 Files", Info: files}, 22, 3},
		{"info dropped whole", Content{Title: "6 Files", Info: files}, 18, 3},
		{"info needs a dash", Content{Title: "6 Files", Info: []string{"abcd"}}, 19, 3},
		{"info with a dash", Content{Title: "6 Files", Info: []string{"abcd"}}, 20, 3},
		{"title cut", Content{Title: "2 Conversation", Info: []string{"26 of 26 shown"}}, 14, 3},
		{"title cut leaves no info", Content{Title: "2 Conversation", Info: []string{"x"}}, 16, 3},
		{"title gone in a tiny box", Content{Title: "2 Conversation"}, 5, 3},
		{"no title", Content{Info: []string{"info"}}, 20, 3},
		{"body cut and padded", Content{Title: "1 Description", Body: "short\na line that is far too wide for this box\n\ttabbed"}, 24, 5},
		{"body rows dropped", Content{Title: "x", Body: "1\n2\n3\n4"}, 12, 4},
		{"wide characters", Content{Title: "3 漢字のタイトル", Info: []string{"日本語"}, Body: "漢字漢字漢字漢字漢字漢字\n🙂🙂🙂"}, 24, 5},
		{
			"hyperlinks",
			Content{
				Title: "\x1b]8;;https://example.com\x1b\\4 Linked title\x1b]8;;\x1b\\",
				Body:  "see \x1b]8;;https://example.com/a-long-url\x1b\\a link that is cut off here\x1b]8;;\x1b\\ end",
			},
			24, 4,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := st.Frame(tt.c, tt.w, tt.h)
			lines := strings.Split(out, "\n")
			if len(lines) != tt.h {
				t.Fatalf("%d lines, want %d", len(lines), tt.h)
			}
			for i, ln := range lines {
				if got := ansi.StringWidth(ln); got != tt.w {
					t.Errorf("line %d is %d cells, want %d: %q", i, got, tt.w, ansi.Strip(ln))
				}
				if linkOpen(ln) {
					t.Errorf("line %d leaves a hyperlink open: %q", i, visible(ln))
				}
			}
			golden.RequireEqual(t, ansi.Strip(out))
		})
	}
}

func TestFrameFocus(t *testing.T) {
	st := DefaultStyles(true)
	c := Content{Title: "3 Merge", Info: []string{"5 blockers"}, Body: "feat/calc → main"}
	focused, blurred := c, c
	focused.Focus = true
	f, b := st.Frame(focused, 30, 4), st.Frame(blurred, 30, 4)
	if ansi.Strip(f) != ansi.Strip(b) {
		t.Error("focus must change colours only")
	}
	if f == b {
		t.Fatal("a focused frame must look different from a blurred one")
	}
	golden.RequireEqual(t, visible(f+"\n\n"+b))
}

func TestFrameNoRoom(t *testing.T) {
	st := DefaultStyles(true)
	for _, sz := range [][2]int{{0, 0}, {1, 5}, {5, 1}, {1, 1}, {2, 2}, {3, 3}} {
		out := st.Frame(Content{Title: "x", Info: []string{"y"}, Body: "z"}, sz[0], sz[1])
		lines := strings.Split(out, "\n")
		if sz[1] > 0 && len(lines) != sz[1] {
			t.Errorf("%v: %d lines", sz, len(lines))
		}
		for _, ln := range lines {
			if sz[1] > 0 && ansi.StringWidth(ln) != sz[0] {
				t.Errorf("%v: line of %d cells", sz, ansi.StringWidth(ln))
			}
		}
	}
}

func TestCutClosesLinks(t *testing.T) {
	st := DefaultStyles(true)
	in := "\x1b]8;;https://example.com\x1b\\abcdefghij\x1b]8;;\x1b\\"
	if !linkOpen(ansi.Truncate(in, 4, "…")) {
		t.Skip("Truncate closes hyperlinks itself; cut has nothing to add")
	}
	if linkOpen(st.cut(in, 4)) {
		t.Error("cut left the hyperlink open")
	}
}

func TestExpandTabs(t *testing.T) {
	for in, want := range map[string]string{
		"a\tb":               "a   b",
		"abcd\te":            "abcd    e",
		"\tx":                "    x",
		"漢\tx":               "漢  x",
		"plain":              "plain",
		"\x1b[1ma\x1b[0m\tb": "\x1b[1ma\x1b[0m   b",
	} {
		if got := expandTabs(in); got != want {
			t.Errorf("expandTabs(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLinkOpen(t *testing.T) {
	for in, want := range map[string]bool{
		"plain":                      false,
		"\x1b]8;;http://x\x1b\\text": true,
		"\x1b]8;;http://x\x1b\\text\x1b]8;;\x1b\\": false,
		"\x1b]8;id=1;http://x\a":                   true,
		"\x1b]8;;\a":                               false,
	} {
		if got := linkOpen(in); got != want {
			t.Errorf("linkOpen(%q) = %v, want %v", in, got, want)
		}
	}
}
