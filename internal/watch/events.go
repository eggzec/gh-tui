package watch

import (
	"context"
	"slices"
)

// Event reports that the data behind Key may have changed and should be
// refetched. Err is set when the latest poll failed.
//
// Events are coalesced: while an event for a key waits to be received, later
// polls of that key update it instead of queueing another, so a burst of
// changes costs the consumer one refetch.
type Event struct {
	Key string
	Err error
}

type pending struct {
	ev  Event
	seq uint64
}

// publish queues ev for its key, or merges it into the event already waiting
// there. It never blocks on the consumer.
func (e *Engine) publish(p *poller, ev Event) {
	e.mu.Lock()
	// A poll that finished after its key was unsubscribed must not revive it.
	if e.pollers[p.key] != p || e.stopped {
		e.mu.Unlock()
		return
	}
	e.seq++
	if q, ok := e.pending[ev.Key]; ok {
		q.ev, q.seq = ev, e.seq
	} else {
		e.pending[ev.Key] = &pending{ev: ev, seq: e.seq}
		e.queue = append(e.queue, ev.Key)
	}
	e.mu.Unlock()

	select {
	case e.notify <- struct{}{}:
	default:
	}
}

// drop discards the pending event for key. The caller holds e.mu.
func (e *Engine) drop(key string) {
	if _, ok := e.pending[key]; !ok {
		return
	}
	delete(e.pending, key)
	e.queue = slices.DeleteFunc(e.queue, func(k string) bool { return k == key })
}

func (e *Engine) head() (pending, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if len(e.queue) == 0 {
		return pending{}, false
	}
	return *e.pending[e.queue[0]], true
}

// delivered removes the event for key unless a newer poll merged into it while
// it was being sent, in which case it stays queued and is sent again.
func (e *Engine) delivered(key string, seq uint64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if q, ok := e.pending[key]; ok && q.seq == seq {
		e.drop(key)
	}
}

// dispatch hands pending events to the consumer, oldest first, until ctx is
// done. It is the only goroutine that sends on e.events, so pollers never
// wait for a slow consumer.
func (e *Engine) dispatch(ctx context.Context) {
	for {
		q, ok := e.head()
		if !ok {
			select {
			case <-e.notify:
				continue
			case <-ctx.Done():
				return
			}
		}
		select {
		case e.events <- q.ev:
			e.delivered(q.ev.Key, q.seq)
		case <-e.notify:
			// Something was published; reread the head so the consumer gets
			// the latest state of the key.
		case <-ctx.Done():
			return
		}
	}
}
