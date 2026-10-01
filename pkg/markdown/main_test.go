package markdown

import (
	"os"
	"testing"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/syntax/syntaxtest"
)

// TestMain times lexers by a clock that doesn't move, so code highlights
// however busy the machine is. A lexer timed by the wall clock may run
// past its limit on a busy one, and the code it lexed then stays plain
// for the rest of the run. The tests of the limits use the wall clock
// (wallClock).
func TestMain(m *testing.M) {
	syntax.UseClock(syntaxtest.Stopped{})
	os.Exit(m.Run())
}

// wallClock times lexers by the wall clock until the test ends, for a
// test of how lexers that never finish are cut short.
func wallClock(t *testing.T) {
	t.Helper()
	syntaxtest.Use(t, nil)
}
