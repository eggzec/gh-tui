package pager

import (
	"os"
	"testing"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/syntax/syntaxtest"
)

// TestMain times lexers by a clock that doesn't move, so files highlight
// however busy the machine is. The test of lexers that never finish uses
// the wall clock.
func TestMain(m *testing.M) {
	syntax.UseClock(syntaxtest.Stopped{})
	os.Exit(m.Run())
}
