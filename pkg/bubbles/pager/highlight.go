package pager

import (
	"slices"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"

	"github.com/eggzec/gh-tui/pkg/syntax"
)

// Lines longer than maxLexedLine bytes show plain, and so does all the
// lexer hasn't reached within lexLimit. A lexer's time can grow with the
// square of a line's length or worse, as on a minified file, and with the
// size of the file.
const (
	maxLexedLine = 2 << 10
	lexLimit     = 2 * time.Second
)

// span is a run of one token type in a line, up to byte end.
type span struct {
	end int
	typ chroma.TokenType
}

// highlightMsg carries the tokens of the content with generation gen back
// to the pager with ID id.
type highlightMsg struct {
	id    int64
	gen   int
	spans [][]span
}

// lexerFor returns the lexer for a file named name that holds text, or nil
// when the text is best shown plain.
func lexerFor(name, text string) chroma.Lexer {
	return worthIt(syntax.File(name, text))
}

// lexerNamed returns the lexer with the name or alias lang, or nil.
func lexerNamed(lang string) chroma.Lexer {
	return worthIt(syntax.Lexer(lang))
}

// worthIt returns l ready to highlight with, or nil if it is plain text,
// which looks the same without the work.
func worthIt(l chroma.Lexer) chroma.Lexer {
	if l == nil || l.Config().Name == "plaintext" {
		return nil
	}
	return chroma.Coalesce(l)
}

// lexable returns lines as the text to lex, with the lines too long to
// lex left empty.
func lexable(lines []string) string {
	long := func(l string) bool { return len(l) > maxLexedLine }
	if !slices.ContainsFunc(lines, long) {
		return strings.Join(lines, "\n")
	}
	var b strings.Builder
	for i, l := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if !long(l) {
			b.WriteString(l)
		}
	}
	return b.String()
}

// spansOf splits toks, the tokens of up to n lines, into the spans of each
// line.
func spansOf(toks []chroma.Token, n int) [][]span {
	out := make([][]span, 0, n)
	var line []span
	pos := 0
	add := func(size int, typ chroma.TokenType) {
		pos += size
		if k := len(line) - 1; k >= 0 && line[k].typ == typ {
			line[k].end = pos
			return
		}
		line = append(line, span{end: pos, typ: typ})
	}
	for _, tok := range toks {
		v := tok.Value
		for {
			head, tail, found := strings.Cut(v, "\n")
			if head != "" {
				add(len(head), tok.Type)
			}
			if !found {
				break
			}
			out = append(out, line)
			line, pos, v = nil, 0, tail
		}
	}
	out = append(out, line)
	// Lexers may add a final newline, and with it an empty line.
	return out[:min(len(out), n)]
}
