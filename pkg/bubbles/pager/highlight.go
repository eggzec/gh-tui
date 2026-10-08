package pager

import (
	"github.com/alecthomas/chroma/v2"

	"github.com/eggzec/gh-tui/pkg/syntax"
)

// Lines longer than maxLexedLine bytes show plain, and so does all the
// lexer hasn't reached within lexLimit.
const (
	maxLexedLine = syntax.MaxLexedLine
	lexLimit     = syntax.LexLimit
)

// highlightMsg carries the tokens of the content with generation gen back
// to the pager with ID id.
type highlightMsg struct {
	id    int64
	gen   int
	spans [][]syntax.Span
}

// lexerFor returns the lexer for a file named name that holds text, or nil
// when the text is best shown plain.
func lexerFor(name, text string) chroma.Lexer {
	return syntax.Ready(syntax.File(name, text))
}

// lexerNamed returns the lexer with the name or alias lang, or nil.
func lexerNamed(lang string) chroma.Lexer {
	return syntax.Ready(syntax.Lexer(lang))
}
