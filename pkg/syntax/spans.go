package syntax

import (
	"slices"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"
)

// Lines longer than MaxLexedLine bytes show plain, and so does all the
// lexer hasn't reached within LexLimit. A lexer's time can grow with the
// square of a line's length or worse, as on a minified file, and with the
// size of the file.
const (
	MaxLexedLine = 2 << 10
	LexLimit     = 2 * time.Second
)

// Span is a run of one token type in a line, up to byte End.
type Span struct {
	End  int
	Type chroma.TokenType
}

// Lexable returns lines as the text to lex, with the lines too long to
// lex left empty.
func Lexable(lines []string) string {
	long := func(l string) bool { return len(l) > MaxLexedLine }
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

// Spans splits toks, the tokens of up to n lines, into the spans of each
// line.
func Spans(toks []chroma.Token, n int) [][]Span {
	out := make([][]Span, 0, n)
	var line []Span
	pos := 0
	add := func(size int, typ chroma.TokenType) {
		pos += size
		if k := len(line) - 1; k >= 0 && line[k].Type == typ {
			line[k].End = pos
			return
		}
		line = append(line, Span{End: pos, Type: typ})
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

// Ready returns l ready to highlight with, or nil if l is nil or plain
// text, which looks the same without the work.
func Ready(l chroma.Lexer) chroma.Lexer {
	if l == nil || l.Config().Name == "plaintext" {
		return nil
	}
	return chroma.Coalesce(l)
}
