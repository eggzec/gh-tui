package markdown

import "github.com/eggzec/gh-tui/pkg/syntax"

// highlighted and lexing are the gate's: the languages whose code is
// highlighted, and the token a lexer holds while it runs.
var (
	highlighted = func() map[string]bool {
		m := make(map[string]bool)
		for n := range syntax.Names() {
			m[n] = true
		}
		return m
	}()
	lexing = syntax.Lexing()
)
