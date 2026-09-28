package watch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

var errPoll = errors.New("poll failed")

// source is a PollFunc that records when it was called. result decides what
// the nth call (starting at 0) returns; nil means unchanged.
type source struct {
	start  time.Time
	result func(n int) (Result, error)

	mu    sync.Mutex
	calls []time.Duration
}

func newSource(result func(n int) (Result, error)) *source {
	return &source{start: time.Now(), result: result}
}

func (s *source) poll(context.Context) (Result, error) {
	s.mu.Lock()
	n := len(s.calls)
	s.calls = append(s.calls, time.Since(s.start))
	s.mu.Unlock()
	if s.result == nil {
		return Result{}, nil
	}
	return s.result(n)
}

func (s *source) times() []time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

func (s *source) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.calls)
}

func changed(int) (Result, error) { return Result{Changed: true}, nil }

// run starts e and stops it when the test ends. The returned function stops
// it early and returns Run's error.
func run(t *testing.T, e *Engine) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	errc := make(chan error, 1)
	go func() { errc <- e.Run(ctx) }()
	stop = sync.OnceValue(func() error {
		cancel()
		return <-errc
	})
	t.Cleanup(func() { _ = stop() })
	return stop
}

func seconds(ss ...int) []time.Duration {
	ds := make([]time.Duration, len(ss))
	for i, s := range ss {
		ds[i] = time.Duration(s) * time.Second
	}
	return ds
}

func TestIntervals(t *testing.T) {
	tests := []struct {
		name   string
		opts   []Option
		result func(n int) (Result, error)
		want   []time.Duration
	}{
		{
			name: "default",
			opts: []Option{WithInterval(10 * time.Second)},
			want: seconds(10, 20, 30, 40, 50, 60),
		},
		{
			name: "hint raises interval",
			opts: []Option{WithInterval(10 * time.Second)},
			result: func(int) (Result, error) {
				return Result{Interval: 25 * time.Second}, nil
			},
			want: seconds(10, 35, 60),
		},
		{
			name: "minimum beats hint",
			opts: []Option{WithInterval(20 * time.Second), WithMinInterval(15 * time.Second)},
			result: func(int) (Result, error) {
				return Result{Interval: time.Second}, nil
			},
			want: seconds(20, 35, 50),
		},
		{
			name: "minimum beats default",
			opts: []Option{WithInterval(time.Second), WithMinInterval(30 * time.Second)},
			want: seconds(30, 60),
		},
		{
			name: "backoff doubles, caps and resets",
			opts: []Option{WithInterval(10 * time.Second), WithMaxBackoff(time.Minute)},
			result: func(n int) (Result, error) {
				if n < 5 {
					return Result{}, errPoll
				}
				return Result{}, nil
			},
			// Waits: 10, then 20, 40, 60 (capped), 60, 60 after each
			// failure, then back to 10 after the first success.
			want: seconds(10, 30, 70, 130, 190, 250, 260, 270),
		},
		{
			name: "a rate limit doesn't back off",
			opts: []Option{WithInterval(10 * time.Second), WithMaxBackoff(time.Minute)},
			result: func(n int) (Result, error) {
				switch n {
				case 0:
					return Result{}, fmt.Errorf("poll: %w", &core.RateLimitError{Reset: time.Now().Add(15 * time.Second)})
				case 1:
					return Result{}, fmt.Errorf("poll: %w", &core.RateLimitError{Reset: time.Now().Add(5 * time.Second)})
				}
				return Result{}, nil
			},
			// The next poll waits for the Reset, or the interval if that
			// is longer, and no failure grows it.
			want: seconds(10, 25, 35, 45, 55),
		},
		{
			name: "backoff never shortens a long interval",
			opts: []Option{WithInterval(2 * time.Minute), WithMaxBackoff(time.Minute)},
			result: func(int) (Result, error) {
				return Result{}, errPoll
			},
			want: seconds(120, 240),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				e := New(tt.opts...)
				src := newSource(tt.result)
				e.Subscribe("k", src.poll)
				run(t, e)

				synctest.Sleep(tt.want[len(tt.want)-1])
				if got := src.times(); !slices.Equal(got, tt.want) {
					t.Errorf("poll times = %v, want %v", got, tt.want)
				}
			})
		})
	}
}

// A poll runs as background work, so that the client doesn't send its
// requests again: the next poll comes anyway.
func TestPollIsBackground(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := New(WithInterval(10 * time.Second))
		var polls, background atomic.Int32
		e.Subscribe("k", func(ctx context.Context) (Result, error) {
			polls.Add(1)
			if obs.IsBackground(ctx) {
				background.Add(1)
			}
			return Result{}, nil
		})
		run(t, e)
		synctest.Sleep(30 * time.Second)
		if n, b := polls.Load(), background.Load(); n == 0 || b != n {
			t.Errorf("%d of %d polls ran as background work, want all", b, n)
		}
	})
}

func TestSubscribeSharesPoller(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := New(WithInterval(10 * time.Second))
		first, second := newSource(nil), newSource(nil)
		unsub1 := e.Subscribe("k", first.poll)
		run(t, e)
		unsub2 := e.Subscribe("k", second.poll)

		synctest.Sleep(30 * time.Second)
		if got := first.count(); got != 3 {
			t.Errorf("shared poller ran %d times in 3 intervals, want 3", got)
		}
		if got := second.count(); got != 0 {
			t.Errorf("second subscriber's func ran %d times, want 0", got)
		}

		// A repeated unsubscribe must not release the other subscriber's
		// reference.
		unsub1()
		unsub1()
		synctest.Sleep(10 * time.Second)
		if got := first.count(); got != 4 {
			t.Errorf("after one unsubscribe: %d polls, want 4", got)
		}

		unsub2()
		synctest.Sleep(time.Minute)
		if got := first.count(); got != 4 {
			t.Errorf("after last unsubscribe: %d polls, want 4", got)
		}
		unsub2()
	})
}

func TestResubscribeStartsNewPoller(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := New(WithInterval(10 * time.Second))
		run(t, e)
		old, fresh := newSource(nil), newSource(nil)
		e.Subscribe("k", old.poll)()
		e.Subscribe("k", fresh.poll)

		synctest.Sleep(10 * time.Second)
		if old.count() != 0 || fresh.count() != 1 {
			t.Errorf("polls: old %d, fresh %d; want 0, 1", old.count(), fresh.count())
		}
	})
}

func TestUnsubscribeCancelsPoll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := New(WithInterval(10 * time.Second))
		run(t, e)
		started := make(chan struct{})
		unsub := e.Subscribe("k", func(ctx context.Context) (Result, error) {
			close(started)
			<-ctx.Done()
			return Result{Changed: true}, nil
		})
		<-started
		unsub()
		synctest.Wait()
		select {
		case ev := <-e.Events():
			t.Errorf("got %v from a canceled poll", ev)
		default:
		}
	})
}

func TestEventsCoalesce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := New(WithInterval(10 * time.Second))
		a, b := newSource(changed), newSource(changed)
		e.Subscribe("a", a.poll)
		e.Subscribe("b", b.poll)
		run(t, e)

		// Nobody reads events, yet polling must carry on.
		synctest.Sleep(100 * time.Second)
		if a.count() != 10 || b.count() != 10 {
			t.Fatalf("polls with no consumer: a %d, b %d; want 10 each", a.count(), b.count())
		}

		// Both keys first changed at the same instant, so their order is
		// up to the scheduler.
		got := receiveAll(e)
		slices.SortFunc(got, func(x, y Event) int { return strings.Compare(x.Key, y.Key) })
		want := []Event{{Key: "a"}, {Key: "b"}}
		if !slices.Equal(got, want) {
			t.Errorf("events after burst = %v, want %v", got, want)
		}

		synctest.Sleep(10 * time.Second)
		if got := receiveAll(e); len(got) != 2 {
			t.Errorf("events after next change = %v, want one per key", got)
		}
	})
}

func TestCoalescedEventHasLatestError(t *testing.T) {
	results := []struct {
		res Result
		err error
	}{
		{Result{Changed: true}, nil},
		{Result{}, errPoll},
		{Result{Changed: true}, nil},
	}
	synctest.Test(t, func(t *testing.T) {
		e := New(WithInterval(10*time.Second), WithMaxBackoff(10*time.Second))
		src := newSource(func(n int) (Result, error) {
			r := results[min(n, len(results)-1)]
			return r.res, r.err
		})
		e.Subscribe("k", src.poll)
		run(t, e)

		synctest.Sleep(20 * time.Second)
		if got := receiveAll(e); len(got) != 1 || !errors.Is(got[0].Err, errPoll) {
			t.Errorf("after change then failure: %v, want one event with %v", got, errPoll)
		}

		synctest.Sleep(20 * time.Second)
		if got := receiveAll(e); len(got) != 1 || got[0].Err != nil {
			t.Errorf("after failure then change: %v, want one event without error", got)
		}
	})
}

// receiveAll returns the events that are ready now.
func receiveAll(e *Engine) []Event {
	var evs []Event
	for {
		synctest.Wait()
		select {
		case ev := <-e.Events():
			evs = append(evs, ev)
		default:
			return evs
		}
	}
}

func TestSetActive(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := New(WithInterval(10*time.Second), WithIdleMultiplier(3))
		a, b := newSource(nil), newSource(nil)
		e.Subscribe("a", a.poll)
		e.Subscribe("b", b.poll)
		run(t, e)

		synctest.Sleep(15 * time.Second)
		e.SetActive(false)
		e.SetActive(false)
		synctest.Sleep(70 * time.Second)
		e.SetActive(true)
		e.SetActive(true)
		synctest.Sleep(10 * time.Second)

		// The poll already scheduled for 20s keeps its time, the idle
		// interval of 30s applies after it, and activation at 85s polls at
		// once.
		want := seconds(10, 20, 50, 80, 85, 95)
		for name, src := range map[string]*source{"a": a, "b": b} {
			if got := src.times(); !slices.Equal(got, want) {
				t.Errorf("%s poll times = %v, want %v", name, got, want)
			}
		}
	})
}

func TestRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := New(WithInterval(time.Minute))
		src := newSource(nil)
		e.Subscribe("k", src.poll)
		e.Refresh("k") // before Run: polls as soon as Run starts
		run(t, e)

		synctest.Sleep(5 * time.Second)
		e.Refresh("k")
		e.Refresh("unknown")
		synctest.Sleep(time.Minute)

		want := seconds(0, 5, 65)
		if got := src.times(); !slices.Equal(got, want) {
			t.Errorf("poll times = %v, want %v", got, want)
		}
	})
}

// TestRunStops relies on synctest.Test, which fails the test if any goroutine
// started by the engine is still running when the test function returns.
func TestRunStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := New(WithInterval(10 * time.Second))
		e.Subscribe("idle", newSource(nil).poll)
		e.Subscribe("pending", newSource(changed).poll)
		blocked := make(chan struct{})
		var returned atomic.Bool
		e.Subscribe("blocked", func(ctx context.Context) (Result, error) {
			close(blocked)
			<-ctx.Done()
			returned.Store(true)
			return Result{}, ctx.Err()
		})
		stop := run(t, e)

		<-blocked
		if err := stop(); !errors.Is(err, context.Canceled) {
			t.Errorf("Run = %v, want %v", err, context.Canceled)
		}
		if !returned.Load() {
			t.Error("Run returned before the in-flight poll finished")
		}
		if _, ok := <-e.Events(); ok {
			t.Error("events channel still open after Run returned")
		}
		if err := e.Run(t.Context()); !errors.Is(err, ErrRan) {
			t.Errorf("second Run = %v, want %v", err, ErrRan)
		}

		// Subscribing to a stopped engine must not start a poller.
		e.Subscribe("late", newSource(nil).poll)
	})
}

func TestPublish(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := New(WithInterval(10 * time.Second))
		src := newSource(changed)
		e.Subscribe("polled", src.poll)
		e.Publish("early") // before Run: delivered once it runs
		stop := run(t, e)
		synctest.Wait()
		if got := receiveAll(e); !slices.Equal(got, []Event{{Key: "early"}}) {
			t.Errorf("events = %v, want the one published before Run", got)
		}

		// A key without a poller, and one whose poll found a change too,
		// each get one event.
		e.Publish("other")
		e.Publish("other")
		synctest.Sleep(10 * time.Second)
		e.Publish("polled")
		got := receiveAll(e)
		slices.SortFunc(got, func(x, y Event) int { return strings.Compare(x.Key, y.Key) })
		if want := []Event{{Key: "other"}, {Key: "polled"}}; !slices.Equal(got, want) {
			t.Errorf("events = %v, want %v", got, want)
		}

		_ = stop()
		e.Publish("late") // must not panic or block
	})
}
