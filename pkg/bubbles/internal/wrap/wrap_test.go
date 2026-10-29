package wrap

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestAdvance(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		i, cols  int
		wantJ    int
		wantUsed int
	}{
		{name: "ascii run", s: "abcdef", cols: 3, wantJ: 3, wantUsed: 3},
		{name: "fits", s: "abc", cols: 10, wantJ: 3, wantUsed: 3},
		{name: "from the middle", s: "abcdef", i: 2, cols: 2, wantJ: 4, wantUsed: 2},
		{name: "wide rune cut", s: "a你好", cols: 4, wantJ: 4, wantUsed: 3},
		{name: "wide runes fit", s: "a你好", cols: 5, wantJ: 7, wantUsed: 5},
		{name: "combining mark", s: "éx", cols: 1, wantJ: 3, wantUsed: 1},
		{name: "cols 0", s: "abc", cols: 0, wantJ: 0, wantUsed: 0},
		{name: "empty", s: "", cols: 4, wantJ: 0, wantUsed: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, used := Advance(tt.s, tt.i, tt.cols)
			if j != tt.wantJ || used != tt.wantUsed {
				t.Errorf("Advance(%q, %d, %d) = %d, %d; want %d, %d",
					tt.s, tt.i, tt.cols, j, used, tt.wantJ, tt.wantUsed)
			}
		})
	}
}

func TestNextRow(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		i, cols  int
		wantJ    int
		wantUsed int
	}{
		{name: "ascii", s: "abcdef", cols: 4, wantJ: 4, wantUsed: 4},
		{name: "wide grapheme wider than cols", s: "你a", cols: 1, wantJ: 3, wantUsed: 2},
		{name: "cols 0 still takes one", s: "abc", cols: 0, wantJ: 1, wantUsed: 1},
		{name: "from the middle", s: "abcdef", i: 3, cols: 2, wantJ: 5, wantUsed: 2},
		{name: "empty", s: "", cols: 3, wantJ: 0, wantUsed: 0},
		{name: "at the end", s: "ab", i: 2, cols: 3, wantJ: 2, wantUsed: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, used := NextRow(tt.s, tt.i, tt.cols)
			if j != tt.wantJ || used != tt.wantUsed {
				t.Errorf("NextRow(%q, %d, %d) = %d, %d; want %d, %d",
					tt.s, tt.i, tt.cols, j, used, tt.wantJ, tt.wantUsed)
			}
		})
	}
}

func TestLeftEdge(t *testing.T) {
	tests := []struct {
		s       string
		left    int
		wantA   int
		wantPad int
	}{
		{s: "abcdef", left: 0, wantA: 0},
		{s: "abcdef", left: 2, wantA: 2},
		{s: "a你好", left: 2, wantA: 4, wantPad: 1},
		{s: "ab", left: 5, wantA: 2},
	}
	for _, tt := range tests {
		a, pad := LeftEdge(tt.s, tt.left)
		if a != tt.wantA || pad != tt.wantPad {
			t.Errorf("LeftEdge(%q, %d) = %d, %d; want %d, %d", tt.s, tt.left, a, pad, tt.wantA, tt.wantPad)
		}
	}
}

// TestAdvanceAgreesWithTruncate checks that the walker cuts plain text where
// ansi.Truncate does.
func TestAdvanceAgreesWithTruncate(t *testing.T) {
	for _, s := range []string{"", "hello world", "a你好b", "éx你y", "👨‍👩‍👧 family", "日本語のテキスト"} {
		for cols := range ansi.StringWidth(s) + 3 {
			j, used := Advance(s, 0, cols)
			want := ansi.Truncate(s, cols, "")
			if s[:j] != want {
				t.Errorf("Advance(%q, 0, %d) cut at %q; Truncate gives %q", s, cols, s[:j], want)
			}
			if w := ansi.StringWidth(s[:j]); w != used {
				t.Errorf("Advance(%q, 0, %d) used %d; the cut %q is %d wide", s, cols, used, s[:j], w)
			}
		}
	}
}
