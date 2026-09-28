package obs

import (
	"slices"
	"testing"
	"testing/synctest"
	"time"
)

func TestThrottle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		th := NewThrottle(time.Minute)
		allowed := 0
		for range 100 {
			if ok, _ := th.Allow("disk"); ok {
				allowed++
			}
		}
		if allowed != 1 {
			t.Errorf("a burst of 100 let %d through, want 1", allowed)
		}
		if ok, _ := th.Allow("other"); !ok {
			t.Error("another key was held back by the first")
		}
		time.Sleep(time.Minute)
		ok, held := th.Allow("disk")
		if !ok || held != 99 {
			t.Errorf("after the interval Allow = %v, %d, want true, 99", ok, held)
		}
		if ok, _ := th.Allow("disk"); ok {
			t.Error("a record right after the one let through passed")
		}
	})
}

func TestThrottleNil(t *testing.T) {
	var th *Throttle
	for range 3 {
		if ok, held := th.Allow("k"); !ok || held != 0 {
			t.Fatalf("nil throttle Allow = %v, %d, want true, 0", ok, held)
		}
	}
}

func TestSuppressed(t *testing.T) {
	if got := Suppressed(0); got != nil {
		t.Errorf("Suppressed(0) = %v, want nil", got)
	}
	if got := Suppressed(3); !slices.Equal(got, []any{"suppressed", 3}) {
		t.Errorf("Suppressed(3) = %v", got)
	}
}
