package revalidate

import (
	"context"
	"slices"
	"sync"
	"time"
)

// window counts the requests of the last minute, so that no minute brings
// more than the budget, however the requests bunch up. A 304 is free
// against GitHub's primary rate limit, but not against its secondary
// limits, which count requests over time.
type window struct {
	mu   sync.Mutex
	sent []time.Time // oldest first
	// freed is closed, and replaced, when a request is given back, to
	// wake those waiting for one.
	freed chan struct{}
}

// take reserves a request at now if fewer than limit were sent in the
// minute before, and returns its time. Otherwise it returns how long to
// wait before trying again, and a channel that is closed if a request is
// given back or the limit rises before then.
func (w *window) take(now time.Time, limit int) (at time.Time, wait time.Duration, freed <-chan struct{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.freed == nil {
		w.freed = make(chan struct{})
	}
	i := 0
	for i < len(w.sent) && now.Sub(w.sent[i]) >= time.Minute {
		i++
	}
	w.sent = w.sent[i:]
	if len(w.sent) < limit {
		w.sent = append(w.sent, now)
		return now, 0, nil
	}
	return time.Time{}, w.sent[len(w.sent)-limit].Add(time.Minute).Sub(now), w.freed
}

// release gives back the request reserved at at, which wasn't sent.
func (w *window) release(at time.Time) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if i := slices.IndexFunc(w.sent, at.Equal); i >= 0 {
		w.sent = slices.Delete(w.sent, i, i+1)
	}
	w.wakeLocked()
}

// wake wakes those waiting for a request, such as when the limit rises.
func (w *window) wake() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.wakeLocked()
}

func (w *window) wakeLocked() {
	if w.freed != nil {
		close(w.freed)
		w.freed = nil
	}
}

// wait reserves a request under the limit that limit returns, waiting as
// long as it takes, and calls idle before each wait. It reports false if
// ctx is done first.
func (w *window) wait(ctx context.Context, limit func() int, idle func()) (time.Time, bool) {
	for {
		at, d, freed := w.take(time.Now(), limit())
		if d <= 0 {
			return at, true
		}
		idle()
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return time.Time{}, false
		case <-t.C:
		case <-freed:
			t.Stop()
		}
	}
}
