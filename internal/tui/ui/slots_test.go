package ui

import (
	"context"
	"testing"
	"testing/synctest"
)

// TestSlotsShrink checks that slots that shrink with reads in flight let
// those go on, and start no more until fewer than the new size are.
func TestSlotsShrink(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewSlots(3)
		for range 3 {
			if err := s.acquire(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
		s.SetSize(1)
		got := make(chan struct{})
		go func() {
			if s.acquire(t.Context()) == nil {
				close(got)
			}
		}()
		synctest.Wait()
		// Two reads end, which leaves one in flight: the size.
		s.release()
		s.release()
		synctest.Wait()
		select {
		case <-got:
			t.Fatal("a read started with the slots full")
		default:
		}
		s.release()
		synctest.Wait()
		select {
		case <-got:
		default:
			t.Fatal("no read started once a slot was free")
		}
	})
}

// TestSlotsHandedOverToCancelled checks that a slot handed to a read
// whose wait ended meanwhile goes on to the next read waiting.
func TestSlotsHandedOverToCancelled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewSlots(1)
		if err := s.acquire(t.Context()); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(t.Context())
		first := make(chan error, 1)
		go func() { first <- s.acquire(ctx) }()
		synctest.Wait()
		second := make(chan error, 1)
		go func() { second <- s.acquire(t.Context()) }()
		synctest.Wait()
		// The slot is handed to the first waiter and its wait ends at
		// once, before it sees the slot.
		s.mu.Lock()
		s.handOn()
		cancel()
		s.mu.Unlock()
		synctest.Wait()
		if err := <-first; err == nil {
			// It took the slot, so it gives it back.
			s.release()
		}
		synctest.Wait()
		select {
		case err := <-second:
			if err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatal("the slot didn't go on to the next read waiting")
		}
	})
}
