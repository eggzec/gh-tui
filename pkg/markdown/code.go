package markdown

import (
	"hash/maphash"
	"strings"
	"sync"
	"time"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// highlighted are the languages whose code is highlighted, by chroma's
// names for their lexers. A lexer is a set of regular expressions that
// runs on untrusted code in Update, and some never finish on some code,
// such as JSONata's and Jungle's on a lone backslash, so only common ones,
// checked on hostile code, run. Chroma can't be stopped once it starts.
// None of them hands code to a lexer the code names, as markdown's and
// PostgreSQL's do by a fence's language and HTTP's by a content type,
// which would reach every lexer.
var highlighted = map[string]bool{
	"Awk": true, "Bash": true, "Bash Session": true, "Batchfile": true,
	"C": true, "C#": true, "C++": true, "CMake": true, "CSS": true,
	"Clojure": true, "Dart": true, "Diff": true, "Docker": true,
	"Elixir": true, "Erlang": true, "Go": true, "GraphQL": true,
	"Groovy": true, "HCL": true, "HTML": true, "Haskell": true,
	"INI": true, "JSON": true, "Java": true, "JavaScript": true,
	"Julia": true, "Kotlin": true, "Lua": true, "Makefile": true,
	"MySQL": true, "Nix": true, "OCaml": true, "Objective-C": true,
	"PHP": true, "Perl": true, "PowerShell": true,
	"Protocol Buffer": true, "Python": true, "R": true, "Ruby": true,
	"Rust": true, "SCSS": true, "SQL": true, "Scala": true, "Swift": true,
	"TOML": true, "Terraform": true, "TypeScript": true, "XML": true,
	"YAML": true, "Zig": true, "plaintext": true, "react": true,
}

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
	lexLimit            = 50 * time.Millisecond
)

// A render highlights at most maxHighlightedBlocks blocks, lexing for at
// most renderLexLimit in all, since a comment can hold hundreds of blocks.
// The blocks after show plain.
const (
	maxHighlightedBlocks = 32
	renderLexLimit       = 100 * time.Millisecond
)

var (
	lexersOnce sync.Once
	// lexersByName holds the highlighted lexers by their names and
	// aliases, in lower case. Chroma's own lookup matches an unknown name
	// against every lexer's file patterns, which takes milliseconds.
	lexersByName map[string]chroma.Lexer
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
	lexersOnce.Do(func() {
		lexersByName = make(map[string]chroma.Lexer)
		for _, l := range lexers.GlobalLexerRegistry.Lexers {
			c := l.Config()
			if !highlighted[c.Name] {
				continue
			}
			lexersByName[strings.ToLower(c.Name)] = l
			for _, a := range c.Aliases {
				lexersByName[strings.ToLower(a)] = l
			}
		}
	})
	return lexersByName[strings.ToLower(lang)]
}

// budget is what a render has left to highlight with.
type budget struct {
	time   time.Duration
	blocks int
}

func newBudget() *budget {
	return &budget{time: renderLexLimit, blocks: maxHighlightedBlocks}
}

// tokens returns the tokens of code, if l tokenises it quickly enough and
// b has room for it, and takes the time it took from b. It is the only
// way code reaches chroma: glamour's own highlighting is off.
func tokens(l chroma.Lexer, code string, b *budget) ([]chroma.Token, bool) {
	k := verdictKey{lexer: l.Config().Name, code: maphash.String(verdictSeed, code)}
	v, known := verdictOf(k)
	if known && !v.done || b.blocks <= 0 || b.time <= 0 {
		return nil, false
	}
	limit := min(lexLimit, b.time)
	toks, v, ok := lex(l, code, limit)
	if !ok {
		return nil, false
	}
	b.time -= v.took
	// A lexer cut short by what the render had left may finish in
	// another.
	if v.done || limit == lexLimit {
		remember(k, v)
	}
	if !v.done {
		return nil, false
	}
	b.blocks--
	return toks, true
}

// verdict is whether a lexer finished on some code within lexLimit, and
// how long it took.
type verdict struct {
	done bool
	took time.Duration
}

type verdictKey struct {
	lexer string
	code  uint64
}

// maxVerdicts is how many verdicts are kept at most, a few bytes each.
const maxVerdicts = 4096

var (
	verdictSeed = maphash.MakeSeed()
	verdictsMu  sync.Mutex
	// verdicts keeps each verdict for as long as the process runs, so
	// code a lexer overran on shows plain at every width without running
	// it again.
	verdicts map[verdictKey]verdict
)

func verdictOf(k verdictKey) (verdict, bool) {
	verdictsMu.Lock()
	defer verdictsMu.Unlock()
	v, ok := verdicts[k]
	return v, ok
}

func remember(k verdictKey, v verdict) {
	verdictsMu.Lock()
	defer verdictsMu.Unlock()
	if verdicts == nil || len(verdicts) >= maxVerdicts {
		verdicts = make(map[verdictKey]verdict)
	}
	verdicts[k] = v
}

// lexing holds a token while a lexer runs. Chroma can't be stopped, so a
// lexer that overruns goes on in the background until it ends, which may
// be never; while it does, code shows plain rather than start another.
var lexing = make(chan struct{}, 1)

// lex runs l on code and returns its tokens if it finished within limit,
// or false for ok if a lexer that overran earlier still runs.
func lex(l chroma.Lexer, code string, limit time.Duration) (toks []chroma.Token, v verdict, ok bool) {
	select {
	case lexing <- struct{}{}:
	default:
		return nil, verdict{}, false
	}
	start := time.Now()
	type result struct {
		toks []chroma.Token
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var r result
		var it chroma.Iterator
		if it, r.err = l.Tokenise(nil, code); r.err == nil {
			r.toks = it.Tokens()
		}
		<-lexing
		done <- r
	}()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.toks, verdict{done: r.err == nil, took: time.Since(start)}, true
	case <-timer.C:
		return nil, verdict{took: limit}, true
	}
}
