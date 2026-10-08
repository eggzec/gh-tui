package syntax

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
)

func TestSpansSplitTokensIntoLines(t *testing.T) {
	toks := []chroma.Token{
		{Type: chroma.Keyword, Value: "var"},
		{Type: chroma.Text, Value: " "},
		{Type: chroma.Text, Value: "x\n"},
		{Type: chroma.Comment, Value: "// a\n// b\n"},
	}
	got := Spans(toks, 3)
	want := [][]Span{
		{{3, chroma.Keyword}, {5, chroma.Text}},
		{{4, chroma.Comment}},
		{{4, chroma.Comment}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if len(got[i]) != len(want[i]) {
			t.Fatalf("line %d = %v, want %v", i, got[i], want[i])
		}
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Errorf("line %d span %d = %v, want %v", i, j, got[i][j], want[i][j])
			}
		}
	}
}

func TestLexableBlanksLongLines(t *testing.T) {
	long := strings.Repeat("x", MaxLexedLine+1)
	if got := Lexable([]string{"a", long, "b"}); got != "a\n\nb" {
		t.Errorf("Lexable = %q, want the long line left empty", got)
	}
	if got := Lexable([]string{"a", "b"}); got != "a\nb" {
		t.Errorf("Lexable = %q, want the lines joined", got)
	}
}
