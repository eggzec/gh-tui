package syntax

import (
	"testing"

	"github.com/alecthomas/chroma/v2"
)

// name returns the name of l, or "" for none.
func name(l chroma.Lexer) string {
	if l == nil {
		return ""
	}
	return l.Config().Name
}

func TestLexer(t *testing.T) {
	for lang, want := range map[string]string{
		"go": "Go", "GO": "Go", "golang": "Go", "diff": "Diff", "sh": "Bash",
		// Lexers that never finish on some code, or hand it to one.
		"jsonata": "", "jungle": "", "markdown": "", "http": "", "postgresql": "",
		"no-such-lang": "",
	} {
		if got := name(Lexer(lang)); got != want {
			t.Errorf("Lexer(%q) is %q, want %q", lang, got, want)
		}
	}
}
