package files

import (
	"os"
	"testing"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/syntax/syntaxtest"
)

// TestMain times lexers by a clock that doesn't move, so previews
// highlight however busy the machine is, and the goldens hold.
func TestMain(m *testing.M) {
	syntax.UseClock(syntaxtest.Stopped{})
	os.Exit(m.Run())
}
