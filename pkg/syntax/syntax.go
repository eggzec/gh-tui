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

var (
	lexersOnce sync.Once
	// lexersByName holds the highlighted lexers by their names and
	// aliases, in lower case. Chroma's own lookup matches an unknown name
	// against every lexer's file patterns, which takes milliseconds.
	lexersByName map[string]chroma.Lexer
)

func load() {
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
}

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
