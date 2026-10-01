package syntax

import (
	"sync/atomic"
	"time"
)

// Clock is what lexers are timed by: how long one ran, and when one ran
// past its limit.
type Clock interface {
	Now() time.Time
	// After returns a channel that receives once d has passed.
	After(d time.Duration) <-chan time.Time
}

// wallClock is the clock lexers are timed by unless a test sets another.
type wallClock struct{}

func (wallClock) Now() time.Time                         { return time.Now() }
func (wallClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

// clockBox holds a Clock, since an atomic.Value can't hold clocks of
// several types.
type clockBox struct{ Clock }

var clock atomic.Pointer[clockBox]

// now returns the clock lexers that start now are timed by.
func now() Clock {
	if b := clock.Load(); b != nil && b.Clock != nil {
		return b.Clock
	}
	return wallClock{}
}

// UseClock makes the lexers that start from now on timed by c, or by the
// wall clock if c is nil, until restore is called. It is for tests: a lexer
// timed by a clock that doesn't move never overruns, so code highlights
// however busy the machine is, while the limits still hold in the app. A
// lexer that runs already keeps the clock it started with.
func UseClock(c Clock) (restore func()) {
	old := clock.Swap(&clockBox{c})
	return func() { clock.Store(old) }
}
