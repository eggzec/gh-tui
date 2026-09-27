package markdown

import (
	"errors"
	"hash/maphash"
	"strings"
	"time"

	"github.com/alecthomas/chroma/v2"

	"github.com/eggzec/gh-tui/pkg/syntax"
)

// The code highlighted is at most maxHighlightedBytes long, in at most
// maxHighlightedLines lines of at most maxHighlightedLine bytes, and its
// lexer must finish within lexLimit; other code shows plain, as does code
// in a quote or deep in a list, which glamour renders on its own. A lexer's
// time can grow with the square of a line's length or worse: Java's takes
// seconds on 16 KiB of "a ".
const (
	maxHighlightedBytes = 4 << 10
	maxHighlightedLines = 400
	maxHighlightedLine  = 512
	lexLimit            = syntax.Limit
)

// A render highlights at most maxHighlightedBlocks blocks, lexing for at
// most renderLexLimit in all, since a comment can hold hundreds of blocks.
// The blocks after show plain.
const (
	maxHighlightedBlocks = 32
	renderLexLimit       = 100 * time.Millisecond
)

// lexer returns the lexer for code whose fence's info string starts with
// lang, if lang is highlighted and the code is short enough to be, or nil.
func lexer(lang, code string) chroma.Lexer {
	if len(code) > maxHighlightedBytes || strings.Count(code, "\n") >= maxHighlightedLines {
		return nil
	}
	for l := range strings.SplitSeq(code, "\n") {
		if len(l) > maxHighlightedLine {
			return nil
		}
	}
	return syntax.Lexer(lang)
}

// budget is what a render has left to highlight with, and whether it
// left a block plain only because a lexer that overran still ran, so the
// render is not worth keeping.
type budget struct {
	time   time.Duration
	blocks int
	busy   bool
}

func newBudget() *budget {
	return &budget{time: renderLexLimit, blocks: maxHighlightedBlocks}
}

// tokens returns the tokens of code, if l tokenises it quickly enough and
// b has room for it, and takes the time it took from b. It is the only
// way code reaches chroma: glamour's own highlighting is off.
func tokens(l chroma.Lexer, code string, b *budget) ([]chroma.Token, bool) {
	if b.blocks <= 0 || b.time <= 0 {
		return nil, false
	}
	// A lexer cut short by what the render had left isn't taken to
	// overrun, and may finish in another.
	toks, took, err := syntax.Tokenise(l, code, min(lexLimit, b.time))
	b.time -= took
	if err != nil {
		b.busy = b.busy || errors.Is(err, syntax.ErrBusy)
		return nil, false
	}
	b.blocks--
	return toks, true
}

// verdictKey names some code and the lexer for it, which highlight
// keeps its lines by.
type verdictKey struct {
	lexer string
	code  uint64
}

var verdictSeed = maphash.MakeSeed()
