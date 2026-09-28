package images

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestLimiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var l limiter
		start := time.Now()
		for range burst {
			if err := l.wait(t.Context(), "a.test"); err != nil {
				t.Fatal(err)
			}
		}
		if d := time.Since(start); d != 0 {
			t.Errorf("a burst of %d waited %v", burst, d)
		}
		// Another host has its own bucket.
		if err := l.wait(t.Context(), "b.test"); err != nil || time.Since(start) != 0 {
			t.Errorf("another host waited %v (%v)", time.Since(start), err)
		}
		// Then perSecond a second.
		for range perSecond {
			if err := l.wait(t.Context(), "a.test"); err != nil {
				t.Fatal(err)
			}
		}
		if d := time.Since(start); d != time.Second {
			t.Errorf("%d more after the burst took %v, want 1s", perSecond, d)
		}
		// A caller that gives up stops waiting.
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()
		if err := l.wait(ctx, "a.test"); !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("err = %v, want DeadlineExceeded", err)
		}
	})
}
