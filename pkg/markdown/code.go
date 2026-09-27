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
// lexer must finish within lexLimit; other code shows plain. A lexer's
// time can grow with the square of a line's length or worse: Java's takes
// seconds on 16 KiB of "a ".
const (
	maxHighlightedBytes = 4 << 10
	maxHighlightedLines = 400
	maxHighlightedLine  = 512
	lexLimit            = 50 * time.Millisecond
)

// A render highlights at most maxHighlightedBlocks blocks, lexing for at
// most renderLexLimit in all, since a comment can hold hundreds of blocks
// and glamour lexes each highlighted one again. The blocks after show
// plain.
const (
	maxHighlightedBlocks = 32
	renderLexLimit       = 100 * time.Millisecond
)

// plain is the language that shows code as it is. Without a language,
// chroma would guess one from the code.
const plain = "text"

var (
	lexersOnce sync.Once
	// lexersByName holds the highlighted lexers by their names and
	// aliases, in lower case. Chroma's own lookup matches an unknown name
	// against every lexer's file patterns, which takes milliseconds.
	lexersByName map[string]chroma.Lexer
)

// lexer returns the lexer chroma finds for lang if it is highlighted, or
// nil.
func lexer(lang string) chroma.Lexer {
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
	if lexersByName[strings.ToLower(lang)] == nil {
		return nil
	}
	// The lexer glamour will use, which a name is known to find quickly.
	if l := lexers.Get(lang); l != nil && highlighted[l.Config().Name] {
		return l
	}
	return nil
}

// budget is what a render has left to highlight with.
type budget struct {
	time   time.Duration
	blocks int
}

func newBudget() *budget {
	return &budget{time: renderLexLimit, blocks: maxHighlightedBlocks}
}

// highlightAs returns the language to highlight code in, from the first
// word of its fence's info string: lang, or plain.
func highlightAs(lang, code string, b *budget) string {
	if b.blocks <= 0 || b.time <= 0 {
		return plain
	}
	if len(code) > maxHighlightedBytes || strings.Count(code, "\n") >= maxHighlightedLines {
		return plain
	}
	for l := range strings.SplitSeq(code, "\n") {
		if len(l) > maxHighlightedLine {
			return plain
		}
	}
	l := lexer(lang)
	if l == nil || !check(l, code, b) {
		return plain
	}
	return lang
}

// check reports whether l tokenises code quickly enough to highlight it,
// and takes what glamour will spend lexing it again from b.
func check(l chroma.Lexer, code string, b *budget) bool {
	k := verdictKey{lexer: l.Config().Name, code: maphash.String(verdictSeed, code)}
	v, ok := verdictOf(k)
	if !ok {
		limit := min(lexLimit, b.time/2)
		v, ok = finishes(l, code, limit)
		if !ok {
			return false
		}
		b.time -= v.took
		// A lexer cut short by what the render had left may finish in
		// another.
		if v.done || limit == lexLimit {
			remember(k, v)
		}
	}
	if !v.done {
		return false
	}
	b.time -= v.took
	b.blocks--
	return true
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
	// it again, and code it finished on is lexed once by glamour alone.
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

// finishes runs l on code and reports whether it finished within limit,
// or false for ok if a lexer that overran earlier still runs.
func finishes(l chroma.Lexer, code string, limit time.Duration) (v verdict, ok bool) {
	select {
	case lexing <- struct{}{}:
	default:
		return verdict{}, false
	}
	start := time.Now()
	done := make(chan bool, 1)
	go func() {
		ok := tokenise(l, code)
		<-lexing
		done <- ok
	}()
	timer := time.NewTimer(limit)
	defer timer.Stop()
	select {
	case ok := <-done:
		return verdict{done: ok, took: time.Since(start)}, true
	case <-timer.C:
		return verdict{took: limit}, true
	}
}

// tokenise runs l on code to the end and reports whether it could.
func tokenise(l chroma.Lexer, code string) bool {
	it, err := l.Tokenise(nil, code)
	if err != nil {
		return false
	}
	t := it()
	for t != chroma.EOF {
		t = it()
	}
	return true
}

// withLang returns the fenced block src with the info string of its
// opening fence set to the language that highlights it, and closed. Code
// longer than maxHighlightedLines shows as blocks of that many lines,
// since glamour's time grows with the square of a block's length.
func withLang(lang, src string, b *budget) string {
	lines := strings.Split(src, "\n")
	f, ok := openFence(lines[0])
	if !ok {
		return src
	}
	fence := lines[0][:len(lines[0])-len(strings.TrimLeft(lines[0], " "))] + strings.Repeat(string(f.char), f.n)
	body := lines[1:]
	if n := len(body); n > 0 && f.closedBy(body[n-1]) {
		body = body[:n-1]
	}
	lang = highlightAs(lang, strings.Join(body, "\n"), b)
	var out strings.Builder
	for out.Len() == 0 || len(body) > 0 {
		n := min(len(body), maxHighlightedLines)
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(fence + lang + "\n")
		for _, l := range body[:n] {
			out.WriteString(l + "\n")
		}
		out.WriteString(fence)
		body = body[n:]
	}
	return out.String()
}
