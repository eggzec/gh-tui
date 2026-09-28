package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
)

func TestLimitKeepsRequestsInFlightBounded(t *testing.T) {
	var inFlight, most atomic.Int64
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := inFlight.Add(1)
		defer inFlight.Add(-1)
		for {
			m := most.Load()
			if n <= m || most.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	var wg sync.WaitGroup
	errs := make(chan error, 50)
	for range 50 {
		wg.Go(func() {
			if _, err := c.Do(t.Context(), http.MethodGet, "ping", nil, nil); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("request failed: %v", err)
	}
	if m := most.Load(); m > int64(maxInFlight) || m < 2 {
		t.Errorf("up to %d requests in flight, want 2 to %d", m, maxInFlight)
	}
}

func TestLimitWaitEndsWithContext(t *testing.T) {
	_, stats := captureLog(t, 0)
	release := make(chan struct{})
	var arrived atomic.Int64
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		arrived.Add(1)
		<-release
		_, _ = w.Write([]byte(`{}`))
	}))
	var wg sync.WaitGroup
	defer wg.Wait()
	defer close(release)
	for range maxInFlight {
		wg.Go(func() { _, _ = c.Do(context.Background(), http.MethodGet, "held", nil, nil) })
	}
	for arrived.Load() < int64(maxInFlight) {
		time.Sleep(time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Do(ctx, http.MethodGet, "waiting", nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the context's", err)
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("returned after %v, want soon after the context ended", d)
	}
	if n := arrived.Load(); n != int64(maxInFlight) {
		t.Errorf("%d requests reached the server, want %d", n, maxInFlight)
	}
	if w := stats.Summary().Waits; w.Requests != 1 || w.MaxMS <= 0 {
		t.Errorf("waits = %+v, want 1", w)
	}
}

// bodies is a transport that answers each request with an empty body, or
// none if noBody is set.
type bodies struct {
	err    error
	noBody bool
}

func (b bodies) RoundTrip(*http.Request) (*http.Response, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.noBody {
		return &http.Response{StatusCode: http.StatusNoContent}, nil
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(""))}, nil
}

func TestLimitReleases(t *testing.T) {
	tests := []struct {
		name string
		// free frees one of two slots taken by lt, from the responses
		// that took them.
		free func(held []io.Closer)
		want bool
		// taken is how many slots are taken once a third request has
		// gone through or blocked.
		taken int
	}{{
		name:  "nothing",
		free:  func([]io.Closer) {},
		taken: 2,
	}, {
		name:  "body closed",
		free:  func(held []io.Closer) { _ = held[0].Close() },
		want:  true,
		taken: 1,
	}, {
		name: "body closed twice frees one",
		free: func(held []io.Closer) {
			_ = held[0].Close()
			_ = held[0].Close()
		},
		want:  true,
		taken: 1,
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				lt := newLimitTransport(bodies{}, 2, 0)
				held := make([]io.Closer, 0, 2)
				for range 2 {
					body, err := send(t.Context(), lt)
					if err != nil {
						t.Fatal(err)
					}
					held = append(held, body)
				}
				done := make(chan struct{})
				go func() {
					defer close(done)
					ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
					defer cancel()
					if body, err := send(ctx, lt); err == nil {
						_ = body.Close()
					}
				}()
				tt.free(held)
				// The third request takes a freed slot as soon as it is
				// given back, so count the slots only once it is done or
				// blocked.
				synctest.Wait()
				select {
				case <-done:
					if !tt.want {
						t.Error("a third request went through with two in flight")
					}
				default:
					if tt.want {
						t.Error("the third request still waits for a freed slot")
					}
				}
				if n := len(lt.slots); n != tt.taken {
					t.Errorf("%d slots taken, want %d", n, tt.taken)
				}
				time.Sleep(time.Hour)
				<-done
			})
		})
	}
}

func TestLimitReleasesOnError(t *testing.T) {
	lt := newLimitTransport(bodies{err: errors.New("refused")}, 1, 0)
	for range 3 {
		if _, err := send(t.Context(), lt); err == nil {
			t.Fatal("RoundTrip succeeded, want the transport's error")
		}
	}
	if n := len(lt.slots); n != 0 {
		t.Errorf("%d slots taken after failures, want none", n)
	}
}

// send sends a request through lt, and returns the body of its response.
func send(ctx context.Context, lt *limitTransport) (io.Closer, error) {
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/x", http.NoBody)
	resp, err := lt.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func TestLimitReleasesWithoutBody(t *testing.T) {
	lt := newLimitTransport(bodies{noBody: true}, 1, 0)
	for range 3 {
		body, err := send(t.Context(), lt)
		if err != nil {
			t.Fatal(err)
		}
		if body != nil {
			t.Fatal("the transport made up a body")
		}
	}
	if n := len(lt.slots); n != 0 {
		t.Errorf("%d slots taken after responses without a body, want none", n)
	}
}

func TestLimitKeepsSlotsForTheForeground(t *testing.T) {
	for name, mark := range map[string]func(context.Context) context.Context{
		"reads ahead":      obs.ForPrefetch,
		"background loops": obs.ForBackground,
	} {
		t.Run(name, func(t *testing.T) { testLimitKeepsSlotsForTheForeground(t, mark) })
	}
}

// testLimitKeepsSlotsForTheForeground fills the slots of the background
// with requests whose context mark marks, and checks that what the user
// waits for still goes at once.
func testLimitKeepsSlotsForTheForeground(t *testing.T, mark func(context.Context) context.Context) {
	t.Helper()
	synctest.Test(t, func(t *testing.T) {
		lt := newLimitTransport(bodies{}, maxInFlight, foregroundSlots)
		background := mark(t.Context())
		// The background takes all it may, and more waits.
		held := make([]io.Closer, 0, maxInFlight-foregroundSlots)
		for range maxInFlight - foregroundSlots {
			body, err := send(background, lt)
			if err != nil {
				t.Fatal(err)
			}
			held = append(held, body)
		}
		var waiting sync.WaitGroup
		for range 3 {
			waiting.Go(func() {
				if body, err := send(background, lt); err == nil {
					_ = body.Close()
				}
			})
		}
		synctest.Wait()
		if n := len(lt.slots); n != maxInFlight-foregroundSlots {
			t.Fatalf("the background holds %d slots, want %d", n, maxInFlight-foregroundSlots)
		}
		// What the user waits for goes at once.
		for range foregroundSlots {
			done := make(chan struct{})
			go func() {
				defer close(done)
				body, err := send(t.Context(), lt)
				if err != nil {
					t.Error(err)
					return
				}
				held = append(held, body)
			}()
			synctest.Wait()
			select {
			case <-done:
			default:
				t.Fatal("a foreground request waits while the background fills its share")
			}
		}
		for _, b := range held {
			_ = b.Close()
		}
		waiting.Wait()
		if n, m := len(lt.slots), len(lt.background); n != 0 || m != 0 {
			t.Errorf("%d slots and %d background slots taken at the end, want none", n, m)
		}
	})
}

// WithConcurrency sets how many requests are in flight at once, and a
// client without it, or given less than one, takes the default config's.
func TestWithConcurrency(t *testing.T) {
	for _, tt := range []struct {
		opts []Option
		want int
	}{
		{nil, maxInFlight},
		{[]Option{WithConcurrency(3)}, 3},
		{[]Option{WithConcurrency(0)}, maxInFlight},
	} {
		c, err := New(append([]Option{WithBaseURL("https://api.github.com/"), WithToken("t")}, tt.opts...)...)
		if err != nil {
			t.Fatal(err)
		}
		if got := cap(limiter(c).slots); got != tt.want {
			t.Errorf("%d options: %d slots, want %d", len(tt.opts), got, tt.want)
		}
		c.Close()
	}
}
