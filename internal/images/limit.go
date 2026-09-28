package images

import (
	"context"
	"sync"
	"time"
)

// Requests to one host: at most burst at once, then perSecond a second,
// so a thread of many images doesn't hit a host all at once.
const (
	perSecond = 8
	burst     = 8
)

// limiter spaces the requests to each host, as a bucket of tokens per
// host. It is safe for concurrent use.
type limiter struct {
	mu    sync.Mutex
	hosts map[string]*tokens
}

type tokens struct {
	left float64
	last time.Time
}

// maxLimited is how many hosts the limiter tracks before it forgets them,
// all full again: there are few image hosts.
const maxLimited = 64

// wait returns when a request to host may be sent, or ctx's error if it is
// done first.
func (l *limiter) wait(ctx context.Context, host string) error {
	for {
		d := l.reserve(host, time.Now())
		if d == 0 {
			return nil
		}
		t := time.NewTimer(d)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		}
	}
}

// reserve takes a token of host at now, and returns 0, or how long until
// one is due if none is left.
func (l *limiter) reserve(host string, now time.Time) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hosts == nil || len(l.hosts) >= maxLimited {
		l.hosts = make(map[string]*tokens)
	}
	b, ok := l.hosts[host]
	if !ok {
		b = &tokens{left: burst, last: now}
		l.hosts[host] = b
	}
	b.left = min(burst, b.left+now.Sub(b.last).Seconds()*perSecond)
	b.last = now
	if b.left >= 1 {
		b.left--
		return 0
	}
	return time.Duration((1 - b.left) / perSecond * float64(time.Second))
}
