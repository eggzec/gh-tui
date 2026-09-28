package obs

import (
	"sync"
	"time"
)

// Throttle lets through at most one record of each key every so often,
// and counts the ones it holds back, so that a burst of the same event,
// such as a full disk failing every write, logs once rather than once per
// event. Keys name kinds of events, such as a resource and a reason, so
// that there are few of them.
//
// A nil *Throttle lets every record through. A Throttle is safe for
// concurrent use.
type Throttle struct {
	every time.Duration

	mu   sync.Mutex
	keys map[string]*throttled
}

// throttled is what a Throttle knows of one key: when it last let a
// record of it through, and how many it held back since.
type throttled struct {
	at   time.Time
	held int
}

// NewThrottle returns a throttle that lets through one record of each key
// every every.
func NewThrottle(every time.Duration) *Throttle {
	return &Throttle{every: every, keys: make(map[string]*throttled)}
}

// Allow reports whether a record of key may be logged now: the first one,
// and then the first once every has passed since the last it let through.
// With true, it returns how many records of key it held back since then,
// for the record to say as suppressed. The caller drops a record held
// back, or logs it at debug level only, so suppressed counts the records
// missing at the usual level, not those missing from the log.
func (t *Throttle) Allow(key string) (ok bool, held int) {
	if t == nil {
		return true, 0
	}
	now := time.Now()
	t.mu.Lock()
	defer t.mu.Unlock()
	k := t.keys[key]
	if k == nil {
		t.keys[key] = &throttled{at: now}
		return true, 0
	}
	if now.Sub(k.at) < t.every {
		k.held++
		return false, 0
	}
	held = k.held
	k.at, k.held = now, 0
	return true, held
}

// Suppressed returns the attributes that say how many records like this
// one a Throttle held back before it, from its level: none if it held none
// back, so that the usual record stays as it was. Some callers log the
// records held back at debug level, where they still show.
func Suppressed(held int) []any {
	if held == 0 {
		return nil
	}
	return []any{"suppressed", held}
}
