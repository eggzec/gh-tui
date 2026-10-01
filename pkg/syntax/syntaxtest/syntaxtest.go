// Package syntaxtest holds clocks to time lexers by in tests, so that
// whether code highlights doesn't hang on how busy the machine is.
package syntaxtest

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/pkg/syntax"
)

// epoch is when the clocks here start.
var epoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// Stopped is a clock that never moves: lexing takes no time and no lexer
// runs past its limit, so each one finishes and its code highlights. A
// lexer that never finishes is waited for forever, so a test of one
// times it by the wall clock.
type Stopped struct{}

// Now returns the same time each time.
func (Stopped) Now() time.Time { return epoch }

// After returns a channel that never receives.
func (Stopped) After(time.Duration) <-chan time.Time { return nil }

// Stepping is a clock that moves on by Step each time it is read, and
// whose After never fires: each lexer that finishes, which reads it when
// it starts and when it ends, takes Step, however long it really took.
type Stepping struct {
	Step time.Duration
	n    atomic.Int64
}

// Now returns Step later than it did the last time.
func (c *Stepping) Now() time.Time {
	return epoch.Add(time.Duration(c.n.Add(1)) * c.Step)
}

// After returns a channel that never receives.
func (*Stepping) After(time.Duration) <-chan time.Time { return nil }

// Use times the lexers that start from now on by c, or by the wall clock
// if c is nil, until the test ends.
func Use(tb testing.TB, c syntax.Clock) {
	tb.Helper()
	tb.Cleanup(syntax.UseClock(c))
}
