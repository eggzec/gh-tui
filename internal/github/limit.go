package github

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
)

// foregroundSlots of the requests in flight (WithConcurrency) are kept for
// what the user waits for: reads ahead (obs.IsPrefetch) and the requests
// of background loops (obs.IsBackground) may take the others only, so
// that they never make a read the user asked for wait, even when many
// that a rate limit held are let go at once. GitHub's secondary rate
// limits frown on many requests at once, and a burst gains nothing past a
// few: they only queue behind each other on the connection.
const foregroundSlots = 2

// limitTransport lets at most cap(slots) requests through at once, and of
// those at most cap(background) reads ahead and requests of background
// loops. A request takes its slots
// before it is sent, waiting as long as its context allows, and gives them
// back when the body of its response is closed, or at once if it failed.
type limitTransport struct {
	base       http.RoundTripper
	slots      chan struct{}
	background chan struct{}
}

func newLimitTransport(base http.RoundTripper, n, foreground int) *limitTransport {
	return &limitTransport{base: base, slots: make(chan struct{}, n), background: make(chan struct{}, max(n-foreground, 1))}
}

// RoundTrip sends req once a slot is free, unless the gate recalled it
// meanwhile (markSent).
func (t *limitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	release, err := t.acquire(req)
	if err == nil {
		// Under the slot, so that once it is marked sent, nothing but its
		// context stops it going out.
		if err = markSent(req.Context()); err != nil {
			release()
		}
	}
	if err != nil {
		// A RoundTripper closes the body of the request, even on error.
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, err
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		release()
		return nil, err
	}
	if resp.Body == nil {
		// Nothing will be closed to give the slots back.
		release()
		return resp, nil
	}
	resp.Body = &limitedBody{ReadCloser: resp.Body, release: release}
	return resp, nil
}

// acquire takes the slots of req, waiting until they are free or the
// context of req is done, and returns what gives them back. A read ahead
// or a request of a background loop takes one of the background slots
// first, so it never holds a shared slot while it waits. A wait is counted
// and logged.
func (t *limitTransport) acquire(req *http.Request) (release func(), err error) {
	ctx := req.Context()
	background := obs.IsPrefetch(ctx) || obs.IsBackground(ctx)
	start := time.Now()
	var waited bool
	if background {
		waited, err = take(ctx, t.background)
	}
	if err == nil {
		var w bool
		w, err = take(ctx, t.slots)
		waited = waited || w
		if err != nil && background {
			<-t.background
		}
	}
	if waited {
		d := time.Since(start)
		obs.CountHTTPWait(d)
		if obs.Enabled(ctx, slog.LevelDebug) {
			attrs := []any{"span", "http", "method", req.Method, "waited_ms", obs.Millis(d),
				"max_in_flight", cap(t.slots), "background", background}
			if err != nil {
				attrs = append(attrs, "err", err.Error())
			}
			slog.DebugContext(ctx, "http wait", attrs...)
		}
	}
	if err != nil {
		return nil, err
	}
	return func() {
		<-t.slots
		if background {
			<-t.background
		}
	}, nil
}

// take takes a slot of ch, waiting until one is free or ctx is done, and
// reports whether it waited.
func take(ctx context.Context, ch chan struct{}) (waited bool, err error) {
	select {
	case ch <- struct{}{}:
		return false, nil
	default:
	}
	select {
	case ch <- struct{}{}:
		return true, nil
	case <-ctx.Done():
		return true, ctx.Err()
	}
}

// limitedBody gives the slot of its request back when it is first closed.
type limitedBody struct {
	io.ReadCloser
	release func()
	once    sync.Once
}

func (b *limitedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}
