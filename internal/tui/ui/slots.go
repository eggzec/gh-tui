package ui

import (
	"context"
	"sync"
)

// Slots bounds how many reads ahead are in flight at once, across every
// [Ahead] that shares it ([Ahead.Share]), so that the pages and kinds that
// read ahead together keep to prefetch.parallel. Create it with
// [NewSlots]. It is safe for concurrent use.
type Slots struct {
	mu   sync.Mutex
	size int
	used int
	// waiting are the reads waiting for a slot, first come first served.
	// Each waits on a channel of its own, made where it waits.
	waiting []chan struct{}
}

// NewSlots returns slots for n reads at once, at least one.
func NewSlots(n int) *Slots {
	return &Slots{size: max(n, 1)}
}

// SetSize lets n reads run at once, at least one, from now on: more start
// at once if n grew, and if it shrank, the reads in flight go on and no
// more start until fewer than n are.
func (s *Slots) SetSize(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.size = max(n, 1)
	for s.used < s.size && len(s.waiting) > 0 {
		s.used++
		s.wake()
	}
}

// acquire takes a slot, waiting for one until ctx is done.
func (s *Slots) acquire(ctx context.Context) error {
	s.mu.Lock()
	if s.used < s.size && len(s.waiting) == 0 {
		s.used++
		s.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	s.waiting = append(s.waiting, ch)
	s.mu.Unlock()
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		defer s.mu.Unlock()
		for i, w := range s.waiting {
			if w == ch {
				s.waiting = append(s.waiting[:i], s.waiting[i+1:]...)
				return ctx.Err()
			}
		}
		// The slot was handed over meanwhile, so it goes to the next.
		s.handOn()
		return ctx.Err()
	}
}

// release gives a slot back, to the first read waiting if there is room.
func (s *Slots) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handOn()
}

// handOn passes a slot that is given back on to the first read waiting,
// unless the slots shrank meanwhile. s.mu must be held.
func (s *Slots) handOn() {
	if s.used <= s.size && len(s.waiting) > 0 {
		s.wake()
		return
	}
	s.used--
}

// wake hands a slot to the first read waiting. s.mu must be held.
func (s *Slots) wake() {
	close(s.waiting[0])
	s.waiting = s.waiting[1:]
}
