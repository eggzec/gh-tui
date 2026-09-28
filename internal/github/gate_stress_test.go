package github

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// TestGateStress sends many requests at once, of every class and against
// several resources, whose quotas run out and refill a little late, with
// secondary limits now and then, and deadlines and cancellations on the
// way. It pins what the gate promises whatever happens: no request comes
// back after its deadline, nor is held past the sanity cap of maxWindow,
// every request held is let go or failed, nothing is left held or
// counted once all came back, the changes are told of with no lock held,
// and the limits that come recall requests waiting for a slot, but never
// one that was sent.
func TestGateStress(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		gateStats(t)
		h := &hub{limit: 20, window: 10 * time.Second, late: 500 * time.Millisecond, delay: 20 * time.Millisecond}
		// A notify that reads the status would deadlock if it were
		// called while the client holds its lock.
		var c *Client
		var told atomic.Int64
		c, err := New(WithBaseURL("https://api.github.com/"), WithToken("t"),
			WithHTTPClient(&http.Client{Transport: unrecalled{base: h, t: t}}),
			WithRateNotify(func() {
				_ = c.RateStatus()
				told.Add(1)
			}))
		if err != nil {
			t.Fatal(err)
		}
		sends := []func(context.Context) error{
			func(ctx context.Context) error {
				_, err := c.Get(ctx, "repos/o/r", Conditional{}, nil)
				return err
			},
			func(ctx context.Context) error {
				_, err := c.Do(ctx, http.MethodPost, "repos/o/r/issues", map[string]string{"title": "t"}, nil)
				return err
			},
			func(ctx context.Context) error {
				_, err := c.Get(obs.ForBackground(ctx), "repos/o/r/pulls", Conditional{}, nil)
				return err
			},
			func(ctx context.Context) error {
				_, err := c.Get(obs.ForBackground(ctx), "search/issues?q=x", Conditional{}, nil)
				return err
			},
			func(ctx context.Context) error {
				_, err := c.Get(obs.ForPrefetch(ctx), "search/issues?q=y", Conditional{}, nil)
				return err
			},
			func(ctx context.Context) error {
				return c.Query(obs.ForBackground(ctx), "query Q { x }", nil, nil)
			},
			func(ctx context.Context) error { return c.Query(ctx, "query R { x }", nil, nil) },
		}

		var wg sync.WaitGroup
		for range 64 {
			wg.Go(func() {
				for range 30 {
					time.Sleep(time.Duration(rand.IntN(2000)) * time.Millisecond)
					if rand.IntN(40) == 0 {
						h.secondary([]string{"", "2", "30"}[rand.IntN(3)])
					}
					ctx, cancel := t.Context(), context.CancelFunc(func() {})
					var canceling *time.Timer
					switch rand.IntN(6) {
					case 0, 1:
						ctx, cancel = context.WithTimeout(t.Context(), time.Duration(rand.IntN(30_000))*time.Millisecond)
					case 2:
						ctx, cancel = context.WithCancel(t.Context())
						canceling = time.AfterFunc(time.Duration(rand.IntN(5000))*time.Millisecond, cancel)
					}
					start := time.Now()
					err := sends[rand.IntN(len(sends))](ctx)
					if dl, ok := ctx.Deadline(); ok && time.Now().After(dl) {
						t.Errorf("a request came back %v after its deadline", time.Since(dl))
					}
					// A hold that lasts longer than the cap fails at the
					// first instant it does, which the timer of its queue
					// fires at (limit.expiry), so a request that the gate
					// failed comes back at most a nanosecond past the cap.
					// One that it let go before the cap then waits for its
					// turn after those let go before it (a stagger of up to
					// maxStagger each), for a slot, and for its answer,
					// which take well under a second here; a timer that
					// never fired would hold it minutes longer, until a
					// scout answered or the limit lifted.
					_, refused := errors.AsType[*Error](err)
					_, queried := errors.AsType[*GraphQLError](err)
					answered := refused || queried
					d := time.Since(start)
					if errors.Is(err, core.ErrRateLimited) && !answered && d > maxWindow+time.Nanosecond {
						t.Errorf("the gate failed a request after %v, past the cap of %v", d, maxWindow)
					}
					if d > maxWindow+time.Second {
						t.Errorf("a request came back after %v, past the cap of %v and its turn", d, maxWindow)
					}
					if canceling != nil {
						canceling.Stop()
					}
					cancel()
				}
			})
		}
		wg.Wait()
		defer c.Close()
		if told.Load() == 0 {
			t.Error("never told of a change")
		}

		c.budget.mu.Lock()
		defer c.budget.mu.Unlock()
		if n := len(c.budget.gate.queues); n != 0 {
			t.Errorf("%d queues left, want none", n)
		}
		if n := len(c.budget.pending); n != 0 {
			t.Errorf("%d reservations left, want none", n)
		}
		if g := c.budget.gate; g.scout != 0 || g.lifting {
			t.Errorf("a scout %d, lifting %v, left after all came back", g.scout, g.lifting)
		}
		// Whether a limit comes while requests wait for a slot is up to
		// the draw, and a run now and then has none, so recalls are left
		// to TestGateRecall.
		sum := obs.Default().Summary().RateLimit
		var held, released int64
		for _, r := range sum.Resources {
			held += r.Held
			released += r.Released
		}
		if held == 0 || released == 0 {
			t.Errorf("rate limit stats = %+v, want requests held and let go", sum)
		}
	})
}
