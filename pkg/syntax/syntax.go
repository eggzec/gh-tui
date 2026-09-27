// Package syntax is the gate code passes to reach chroma's lexers. A lexer
// is a set of regular expressions that runs on untrusted code, some never
// finish on some code, such as JSONata's and Jungle's on a lone
// backslash, and chroma can't stop one once it starts. So only common
// lexers, checked on hostile code, are ever looked up, none is guessed
// from the code, and they run one at a time within a time limit.
package syntax

import (
	"iter"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// highlighted are the languages whose code is highlighted, by chroma's
// names for their lexers. None of them hands code to a lexer the code
// names, as markdown's and PostgreSQL's do by a fence's language and
// HTTP's by a content type, which would reach every lexer.
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

// markdown is the name of chroma's lexer for markdown, which hands a
// fence's code to the lexer its language names. A file is often markdown,
// so files in it are highlighted, with the fences confined to the
// highlighted languages.
const markdown = "markdown"

var (
	lexersOnce sync.Once
	// lexersByName holds the highlighted lexers by their names and
	// aliases, in lower case. Chroma's own lookup matches an unknown name
	// against every lexer's file patterns, which takes milliseconds.
	lexersByName map[string]chroma.Lexer
	// confined is markdown's lexer, handing fences to the highlighted
	// lexers only, or nil.
	confined chroma.Lexer
)

func load() {
	lexersOnce.Do(func() {
		lexersByName = make(map[string]chroma.Lexer)
		fences := chroma.NewLexerRegistry()
		for _, l := range lexers.GlobalLexerRegistry.Lexers {
			c := l.Config()
			if !highlighted[c.Name] {
				continue
			}
			lexersByName[strings.ToLower(c.Name)] = l
			for _, a := range c.Aliases {
				lexersByName[strings.ToLower(a)] = l
			}
			fences.Register(kept{l})
		}
		// Chroma keeps one lexer for markdown, so this confines it for
		// everyone who uses it.
		if l := lexers.GlobalLexerRegistry.Get(markdown); l != nil && l.Config().Name == markdown {
			l.SetRegistry(fences)
			confined = l
		}
	})
}

// kept is a lexer that keeps the registry it has when another takes it.
type kept struct{ chroma.Lexer }

func (k kept) SetRegistry(*chroma.LexerRegistry) chroma.Lexer { return k }

// Names returns the names of the lexers whose code is highlighted.
func Names() iter.Seq[string] {
	return maps.Keys(highlighted)
}

// Lexer returns the lexer named name, or with name for an alias, ignoring
// case, if its code is highlighted, or nil.
func Lexer(name string) chroma.Lexer {
	load()
	return lexersByName[strings.ToLower(name)]
}

// File returns the lexer for a file named filename that holds text, by
// its name or extension, or else by the interpreter its shebang line
// names, if its code is highlighted, or nil. It never guesses from the
// code, and a file whose name picks a lexer that isn't highlighted has
// none, rather than the next best.
func File(filename, text string) chroma.Lexer {
	load()
	l := lexers.Match(filepath.Base(filename))
	switch {
	case l == nil:
		first, _, _ := strings.Cut(text, "\n")
		return shebang(first)
	case l.Config().Name == markdown:
		return confined
	case highlighted[l.Config().Name]:
		return l
	}
	return nil
}

// shebang returns the lexer for a script whose first line is line, such
// as "#!/usr/bin/env python3", by the interpreter it names, or nil.
func shebang(line string) chroma.Lexer {
	rest, ok := strings.CutPrefix(line, "#!")
	if !ok {
		return nil
	}
	args := strings.Fields(rest)
	if len(args) > 0 && filepath.Base(args[0]) == "env" {
		args = slices.DeleteFunc(args[1:], func(a string) bool { return strings.HasPrefix(a, "-") })
	}
	if len(args) == 0 {
		return nil
	}
	name := filepath.Base(args[0])
	if l := Lexer(name); l != nil {
		return l
	}
	// A versioned interpreter, such as python3.12.
	return Lexer(strings.TrimRight(name, "0123456789."))
}
