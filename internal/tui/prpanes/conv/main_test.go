package conv

import (
	"os"
	"testing"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/syntax/syntaxtest"
)

// TestMain times lexers by a clock that doesn't move, so code highlights
// however busy the machine is: a lexer that overran would leave its block
// plain, which the markdown renderer draws wrapped and with no code bar.
func TestMain(m *testing.M) {
	syntax.UseClock(syntaxtest.Stopped{})
	os.Exit(m.Run())
}
