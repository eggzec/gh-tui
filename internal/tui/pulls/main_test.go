package pulls

import (
	"log/slog"
	"os"
	"testing"

	"github.com/eggzec/gh-tui/pkg/syntax"
	"github.com/eggzec/gh-tui/pkg/syntax/syntaxtest"
)

// TestMain drops what the section logs, which would otherwise go to the
// test output and garble benchmark results. It times lexers by a clock
// that doesn't move, so the code in comments highlights however busy the
// machine is, and the goldens hold.
func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	syntax.UseClock(syntaxtest.Stopped{})
	os.Exit(m.Run())
}
