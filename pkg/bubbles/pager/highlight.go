package pager

import (
	"context"
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// analyseBytes is how much of a file without a known name is read to guess
// its syntax, for example from a shebang line.
const analyseBytes = 4096

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

// lexerFor returns the lexer for a file name, or one guessed from the text,
// or nil when the text is best shown plain.
func lexerFor(name, text string) chroma.Lexer {
	l := lexers.Match(name)
	if l == nil {
		l = lexers.Analyse(text[:min(len(text), analyseBytes)])
	}
	// Plain text looks the same without the work.
	if l == nil || l == lexers.Fallback || l.Config().Name == "plaintext" {
		return nil
	}
	return chroma.Coalesce(l)
}

// highlight splits the tokens of text into the spans of its n lines. It
// stops early when ctx is cancelled.
func highlight(ctx context.Context, lexer chroma.Lexer, text string, n int) ([][]span, error) {
	it, err := lexer.Tokenise(nil, text)
	if err != nil {
		return nil, fmt.Errorf("highlight: %w", err)
	}
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
	count := 0
	for tok := it(); tok != chroma.EOF; tok = it() {
		count++
		if count%4096 == 0 && ctx.Err() != nil {
			return nil, fmt.Errorf("highlight: %w", ctx.Err())
		}
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
	return out[:min(len(out), n)], nil
}
