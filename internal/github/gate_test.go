package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// hub is a fake GitHub for the tests of the gate. Each resource has a
// quota of limit requests a window, which refills late after the reset
// of the window, as GitHub may, since it keeps its resets in whole
// seconds. It answers GET /rate_limit with the quotas, unless noProbe is
// set, and each request after delay, or the delay of its path in delays.
// It keeps when each request came and how many were in flight at most.
type hub struct {
	limit   int
	window  time.Duration
	late    time.Duration
	delay   time.Duration
	delays  map[string]time.Duration
	noProbe bool

	mu     sync.Mutex
	quotas map[string]*hubQuota
	log    []hubRequest
	// secondaries are the Retry-After of the secondary limits that the
	// next requests meet, "" for one that only its message tells.
	secondaries []string
	// fail is how many of the next requests, but for the probes, get no
	// answer.
	fail int
	// inFlight and background count the requests being answered, all and
	// those of the background, and maxInFlight and maxBackground the most
	// at once.
	inFlight, background, maxInFlight, maxBackground int
}

type hubQuota struct {
	remaining int
	reset     time.Time
}

// hubRequest is a request that came to a hub, at at.
type hubRequest struct {
	path string
	at   time.Time
}

// quota returns the quota of resource, refilled if its window passed.
// h.mu must be held.
func (h *hub) quota(resource string, now time.Time) *hubQuota {
	if h.quotas == nil {
		h.quotas = make(map[string]*hubQuota)
	}
	q := h.quotas[resource]
	if q == nil || !now.Before(q.reset.Add(h.late)) {
		q = &hubQuota{remaining: h.limit, reset: now.Truncate(time.Second).Add(h.window)}
		h.quotas[resource] = q
	}
	return q
}

// spend spends the quota of resource until reset.
func (h *hub) spend(resource string, reset time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	q := h.quota(resource, time.Now())
	q.remaining, q.reset = 0, reset
}

// requests returns the requests that came, but for the probes, and the
// probes.
func (h *hub) requests() (sent, probes []hubRequest) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.log {
		if r.path == "rate_limit" {
			probes = append(probes, r)
		} else {
			sent = append(sent, r)
		}
	}
	return sent, probes
}

func (h *hub) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	now := time.Now()
	path := strings.TrimPrefix(req.URL.Path, "/")
	background := obs.IsPrefetch(req.Context()) || obs.IsBackground(req.Context())
	h.mu.Lock()
	h.log = append(h.log, hubRequest{path, now})
	if h.fail > 0 && path != "rate_limit" {
		h.fail--
		h.mu.Unlock()
		return nil, errors.New("connection reset")
	}
	delay, ok := h.delays[path]
	if !ok {
		delay = h.delay
	}
	h.inFlight++
	h.maxInFlight = max(h.maxInFlight, h.inFlight)
	if background {
		h.background++
		h.maxBackground = max(h.maxBackground, h.background)
	}
	status, header, body := h.answer(path, now)
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.inFlight--
		if background {
			h.background--
		}
	}()
	if err := sleep(req.Context(), delay); err != nil {
		return nil, err
	}
	header.Set("X-GitHub-Request-Id", "ABCD:1234")
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

// answer returns what h answers to a request of path at now. h.mu must be
// held.
func (h *hub) answer(path string, now time.Time) (status int, header http.Header, body string) {
	header = make(http.Header)
	if path == "rate_limit" {
		if h.noProbe {
			return http.StatusNotFound, header, `{"message": "Not Found"}`
		}
		resources := make(map[string]map[string]int64)
		for resource := range h.quotas {
			q := h.quota(resource, now)
			resources[resource] = map[string]int64{"limit": int64(h.limit), "remaining": int64(q.remaining), "reset": q.reset.Unix()}
		}
		b, _ := json.Marshal(map[string]any{"resources": resources})
		return http.StatusOK, header, string(b)
	}
	resource := resourceCore
	switch {
	case path == "graphql":
		resource = resourceGraphQL
	case strings.HasPrefix(path, "search/"):
		resource = resourceSearch
	}
	q := h.quota(resource, now)
	status, body = http.StatusOK, `{}`
	switch {
	case len(h.secondaries) > 0:
		status, body = http.StatusForbidden, `{"message": "You have exceeded a secondary rate limit."}`
		if h.secondaries[0] != "" {
			header.Set("Retry-After", h.secondaries[0])
		}
		h.secondaries = h.secondaries[1:]
	case q.remaining == 0:
		status, body = http.StatusForbidden, `{"message": "API rate limit exceeded"}`
	default:
		q.remaining--
	}
	for k, v := range quotaHeader(resource, h.limit, q.remaining, q.reset) {
		header.Set(k, v)
	}
	return status, header, body
}

// newHubClient returns a client whose requests h answers, and whose
// releases are staggered by jitter.
func newHubClient(t *testing.T, h *hub, jitter float64) *Client {
	t.Helper()
	c, err := New(WithBaseURL("https://api.github.com/"), WithToken("t"), WithHTTPClient(&http.Client{Transport: h}))
	if err != nil {
		t.Fatal(err)
	}
	c.budget.gate.jitter = func() float64 { return jitter }
	return c
}

// gateStats makes empty stats the default ones for the test, and returns
// them.
func gateStats(t *testing.T) *obs.Stats {
	t.Helper()
	s := obs.NewStats()
	prev := obs.SetDefault(s)
	t.Cleanup(func() { obs.SetDefault(prev) })
	return s
}

// spendCore has c learn that core is spent until reset, which lifts a
// guard later.
func spendCore(t *testing.T, c *Client, h *hub, reset time.Time) {
	t.Helper()
	h.spend(resourceCore, reset)
	if _, err := c.Get(t.Context(), "user", Conditional{}, nil); !errors.Is(err, core.ErrRateLimited) {
		t.Fatalf("Get of a spent quota = %v, want a rate limit", err)
	}
}

// watchdog panics if t still runs after a few seconds: a gate that sets a
// timer to fire at once, again and again, never lets the fake clock of
// synctest move, and the test would hang until the timeout of go test.
func watchdog(t *testing.T) {
	t.Helper()
	name := t.Name()
	timer := time.AfterFunc(10*time.Second, func() { panic(name + " still runs after 10s: the gate loops") })
	t.Cleanup(func() { timer.Stop() })
}

// goAsync runs send in the background, and returns where its error
// arrives, once send is held or on its way.
func goAsync(send func() error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- send() }()
	synctest.Wait()
	return done
}

// TestGateReleaseTiming pins the release of a held request:
// nothing is sent before the reset and its guard, then the probe
// confirms the new window, and the request goes a stagger of 2ms to 10ms
// after it.
func TestGateReleaseTiming(t *testing.T) {
	for _, jitter := range []float64{0, 0.5, 0.999} {
		t.Run(fmt.Sprint(jitter), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := &hub{limit: 5, window: time.Minute}
				c := newHubClient(t, h, jitter)
				reset := time.Now().Add(10 * time.Second)
				spendCore(t, c, h, reset)

				if _, err := c.Get(obs.ForBackground(t.Context()), "repos/o/r", Conditional{}, nil); err != nil {
					t.Fatalf("held Get = %v", err)
				}
				release := reset.Add(minGuard)
				stagger := minStagger + time.Duration(jitter*float64(maxStagger-minStagger))
				sent, probes := h.requests()
				if len(probes) != 1 || !probes[0].at.Equal(release) {
					t.Errorf("probes %v, want one at %v", probes, release)
				}
				if len(sent) != 2 || !sent[1].at.Equal(release.Add(stagger)) {
					t.Fatalf("sent %v, want the held request at %v", sent, release.Add(stagger))
				}
				if stagger < minStagger || stagger > maxStagger {
					t.Errorf("stagger %v, want within [%v, %v]", stagger, minStagger, maxStagger)
				}
			})
		})
	}
}

// TestGateStagger pins the order and spacing of the releases:
// the requests held are let go by class, then as they came, each
// 2ms to 10ms after the one before, whatever the jitter.
func TestGateStagger(t *testing.T) {
	for _, jitter := range []float64{0, 0.999} {
		t.Run(fmt.Sprint(jitter), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := &hub{limit: 100, window: time.Minute}
				c := newHubClient(t, h, jitter)
				// The foreground waits up to 2s, so the reset is 1s away.
				spendCore(t, c, h, time.Now().Add(time.Second))

				var want []string
				dones := make([]<-chan error, 0, 50)
				for i := range 50 {
					var send func() error
					path := fmt.Sprintf("repos/o/r/issues/%d", i)
					switch i % 3 {
					case 0:
						send = func() error {
							_, err := c.Get(obs.ForBackground(t.Context()), path, Conditional{}, nil)
							return err
						}
					case 1:
						send = func() error {
							_, err := c.Do(t.Context(), http.MethodPost, path, nil, nil)
							return err
						}
					case 2:
						send = func() error {
							_, err := c.Get(t.Context(), path, Conditional{}, nil)
							return err
						}
					}
					dones = append(dones, goAsync(send))
				}
				for _, class := range []int{2, 1, 0} {
					for i := class; i < 50; i += 3 {
						want = append(want, fmt.Sprintf("repos/o/r/issues/%d", i))
					}
				}
				for _, done := range dones {
					if err := <-done; err != nil {
						t.Fatal(err)
					}
				}

				sent, _ := h.requests()
				sent = sent[1:]
				for i, r := range sent {
					if r.path != want[i] {
						t.Fatalf("request %d sent is %s, want %s", i, r.path, want[i])
					}
					if i == 0 {
						continue
					}
					if gap := r.at.Sub(sent[i-1].at); gap < minStagger || gap > maxStagger {
						t.Errorf("request %d sent %v after the one before, want within [%v, %v]", i, gap, minStagger, maxStagger)
					}
				}
			})
		})
	}
}

// TestGateCancelWhileHeld pins cancellation: a held
// request whose context ends leaves the queue at once with the context's
// error, never a rate limit, and counts as dropped.
func TestGateCancelWhileHeld(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := gateStats(t)
		h := &hub{limit: 5, window: time.Minute}
		c := newHubClient(t, h, 0)
		spendCore(t, c, h, time.Now().Add(10*time.Minute))

		ctx, cancel := context.WithCancel(obs.ForBackground(t.Context()))
		done := goAsync(func() error {
			_, err := c.Get(ctx, "repos/o/r", Conditional{}, nil)
			return err
		})
		if n := coreQuota(t, c.RateStatus()).Held; n != 1 {
			t.Fatalf("%d held, want 1", n)
		}
		start := time.Now()
		cancel()
		err := <-done
		if !errors.Is(err, context.Canceled) || errors.Is(err, core.ErrRateLimited) {
			t.Errorf("err = %v, want context.Canceled and no rate limit", err)
		}
		if core.KindOf(err) != core.Canceled {
			t.Errorf("kind = %v, want canceled", core.KindOf(err))
		}
		if d := time.Since(start); d != 0 {
			t.Errorf("returned after %v, want at once", d)
		}
		if n := coreQuota(t, c.RateStatus()).Held; n != 0 {
			t.Errorf("%d held after the cancellation, want none", n)
		}
		c.budget.mu.Lock()
		queues := len(c.budget.gate.queues)
		c.budget.mu.Unlock()
		if queues != 0 {
			t.Errorf("%d queues after the cancellation, want none", queues)
		}
		if sum := stats.Summary().RateLimit; len(sum.Resources) != 1 || sum.Resources[0].Dropped != 1 {
			t.Errorf("rate limit stats = %+v, want 1 dropped", sum)
		}
		if sent, _ := h.requests(); len(sent) != 1 {
			t.Errorf("sent %v, want only the first request", sent)
		}
	})
}

// TestGateDeadline pins the deadline rule: a request
// that couldn't be sent 50ms before its deadline fails at once with the
// release as its reset, and nothing is sent; one with time to spare is
// held and sent.
func TestGateDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5, window: time.Minute}
		c := newHubClient(t, h, 0)
		reset := time.Now().Add(10 * time.Second)
		spendCore(t, c, h, reset)
		release := reset.Add(minGuard)

		ctx, cancel := context.WithDeadline(obs.ForBackground(t.Context()), release.Add(deadlineMargin-time.Millisecond))
		defer cancel()
		start := time.Now()
		_, err := c.Get(ctx, "repos/o/r", Conditional{}, nil)
		if got := limitedUntil(t, err); !got.Equal(release) {
			t.Errorf("Reset = %v, want the release %v", got, release)
		}
		if d := time.Since(start); d != 0 {
			t.Errorf("failed after %v, want at once", d)
		}

		ctx, cancel = context.WithDeadline(obs.ForBackground(t.Context()), release.Add(deadlineMargin+maxStagger))
		defer cancel()
		if _, err := c.Get(ctx, "repos/o/r", Conditional{}, nil); err != nil {
			t.Errorf("Get with time to spare = %v", err)
		}
		if sent, _ := h.requests(); len(sent) != 2 {
			t.Errorf("sent %v, want the first request and the one with time to spare", sent)
		}
	})
}

// TestGateForegroundFailsFast pins what the user waits for:
// a read or a change of a quota that refills in half an hour
// fails at once, with the release as its reset, and nothing is sent; one
// that refills within foregroundWait waits and succeeds.
func TestGateForegroundFailsFast(t *testing.T) {
	sends := map[string]func(context.Context, *Client) error{
		"read": func(ctx context.Context, c *Client) error {
			_, err := c.Get(ctx, "repos/o/r", Conditional{}, nil)
			return err
		},
		"mutation": func(ctx context.Context, c *Client) error {
			_, err := c.Do(ctx, http.MethodPost, "repos/o/r/issues", map[string]string{"title": "t"}, nil)
			return err
		},
	}
	for name, send := range sends {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := &hub{limit: 5, window: time.Minute}
				c := newHubClient(t, h, 0)
				reset := time.Now().Add(30 * time.Minute)
				spendCore(t, c, h, reset)

				err := send(t.Context(), c)
				if !errors.Is(err, core.ErrRateLimited) || core.KindOf(err) != core.RateLimited {
					t.Fatalf("err = %v, want a rate limit", err)
				}
				if got := limitedUntil(t, err); !got.Equal(reset.Add(minGuard)) {
					t.Errorf("Reset = %v, want the release %v", got, reset.Add(minGuard))
				}
				if sent, _ := h.requests(); len(sent) != 1 {
					t.Errorf("sent %v, want only the first request", sent)
				}
			})
		})
		t.Run(name+" waits", func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				h := &hub{limit: 5, window: time.Minute}
				c := newHubClient(t, h, 0)
				reset := time.Now().Add(time.Second)
				spendCore(t, c, h, reset)
				time.Sleep(500 * time.Millisecond)

				if err := send(t.Context(), c); err != nil {
					t.Fatalf("err = %v, want it sent once the limit lifted", err)
				}
				if want := reset.Add(minGuard + minStagger); !time.Now().Equal(want) {
					t.Errorf("sent at %v, want %v", time.Now(), want)
				}
			})
		})
	}
}

// TestGatePrefetchReserve pins the share kept from reads ahead, the
// share under which the status bar warns too (core.LowQuotaShare): once
// a read ahead would leave less than it, it fails at once and isn't held,
// while a read the user waits for goes.
func TestGatePrefetchReserve(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := gateStats(t)
		h := &hub{limit: 100, window: time.Hour}
		c := newHubClient(t, h, 0)
		h.mu.Lock()
		h.quota(resourceCore, time.Now()).remaining = 100 * core.LowQuotaShare / 100
		h.mu.Unlock()
		if _, err := c.Get(t.Context(), "user", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}

		_, err := c.Get(obs.ForPrefetch(t.Context()), "repos/o/r", Conditional{}, nil)
		if !errors.Is(err, core.ErrRateLimited) {
			t.Errorf("read ahead = %v, want a rate limit", err)
		}
		if n := coreQuota(t, c.RateStatus()).Held; n != 0 {
			t.Errorf("%d held, want none", n)
		}
		if _, err := c.Get(t.Context(), "repos/o/r", Conditional{}, nil); err != nil {
			t.Errorf("foreground read = %v, want it sent", err)
		}
		if sent, _ := h.requests(); len(sent) != 2 {
			t.Errorf("sent %v, want the foreground reads only", sent)
		}
		if got := stats.Summary().RateLimit.FailedFast["reserve"]; got != 1 {
			t.Errorf("failed fast for the reserve %d times, want 1", got)
		}
	})
}

// TestGateEarlyLimit pins a limit that outlasts its guard:
// the probe finds the old window, so the guard doubles and the
// request stays held until the next probe finds the new one; nothing is
// sent in between, and nothing loops.
func TestGateEarlyLimit(t *testing.T) {
	watchdog(t)
	synctest.Test(t, func(t *testing.T) {
		stats := gateStats(t)
		h := &hub{limit: 5, window: time.Minute, late: 1500 * time.Millisecond}
		c := newHubClient(t, h, 0)
		reset := time.Now().Add(10 * time.Second)
		spendCore(t, c, h, reset)

		if _, err := c.Get(obs.ForBackground(t.Context()), "repos/o/r", Conditional{}, nil); err != nil {
			t.Fatalf("held Get = %v", err)
		}
		sent, probes := h.requests()
		if want := []time.Time{reset.Add(minGuard), reset.Add(minGuard + 2*minGuard)}; len(probes) != 2 ||
			!probes[0].at.Equal(want[0]) || !probes[1].at.Equal(want[1]) {
			t.Errorf("probes %v, want them at %v", probes, want)
		}
		if len(sent) != 2 || !sent[1].at.Equal(reset.Add(3*minGuard+minStagger)) {
			t.Errorf("sent %v, want the held request after the second probe", sent)
		}
		if g := c.budget.quotas[resourceCore].guard; g != minGuard {
			t.Errorf("guard in the new window = %v, want %v", g, minGuard)
		}
		if sum := stats.Summary().RateLimit; sum.Early != 1 || sum.Probes != 2 {
			t.Errorf("rate limit stats = %+v, want 1 early limit and 2 probes", sum)
		}
	})
}

// TestGateScout pins what happens when the probe can't tell:
// the first request held goes alone, and when it finds the old window,
// the guard doubles and the others stay held until the next one finds
// the new window.
func TestGateScout(t *testing.T) {
	watchdog(t)
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5, window: time.Minute, late: 1500 * time.Millisecond, noProbe: true}
		c := newHubClient(t, h, 0)
		reset := time.Now().Add(10 * time.Second)
		spendCore(t, c, h, reset)

		dones := make([]<-chan error, 0, 3)
		for range 3 {
			dones = append(dones, goAsync(func() error {
				_, err := c.Get(obs.ForBackground(t.Context()), "repos/o/r", Conditional{}, nil)
				return err
			}))
		}
		if err := <-dones[0]; !errors.Is(err, core.ErrRateLimited) {
			t.Errorf("the first held = %v, want the rate limit it found", err)
		}
		for _, done := range dones[1:] {
			if err := <-done; err != nil {
				t.Errorf("held Get = %v", err)
			}
		}
		sent, probes := h.requests()
		scout := reset.Add(minGuard + minStagger)
		next := scout.Add(2*minGuard + minStagger)
		want := []time.Time{scout, next, next.Add(minStagger)}
		if len(sent) != 4 || !sent[1].at.Equal(want[0]) || !sent[2].at.Equal(want[1]) || !sent[3].at.Equal(want[2]) {
			t.Errorf("sent %v, want the held ones at %v", sent, want)
		}
		if len(probes) != 1 {
			t.Errorf("probes %v, want the one that failed", probes)
		}
	})
}

// TestGateSlots pins how a release meets the limit of requests in flight:
// of 100 background requests let go at once, never
// more than 8 are in flight, nor more than 6 of the background, and a
// read the user waits for that comes meanwhile is sent at once.
func TestGateSlots(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5000, window: time.Hour, delay: 50 * time.Millisecond}
		c := newHubClient(t, h, 0.5)
		reset := time.Now().Add(10 * time.Second)
		spendCore(t, c, h, reset)

		var wg sync.WaitGroup
		for i := range 100 {
			wg.Go(func() {
				if _, err := c.Get(obs.ForBackground(t.Context()), "repos/o/r/issues/"+strconv.Itoa(i), Conditional{}, nil); err != nil {
					t.Error(err)
				}
			})
		}
		time.Sleep(time.Until(reset.Add(minGuard + 100*time.Millisecond)))
		asked := time.Now()
		if _, err := c.Get(t.Context(), "user", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}
		wg.Wait()

		sent, _ := h.requests()
		for _, r := range sent {
			if r.path == "user" && !r.at.Before(asked) && !r.at.Equal(asked) {
				t.Errorf("the foreground read was sent at %v, want at once, %v", r.at, asked)
			}
		}
		if h.maxInFlight > maxInFlight || h.maxBackground > maxInFlight-foregroundSlots {
			t.Errorf("%d in flight at most, %d of the background, want %d and %d",
				h.maxInFlight, h.maxBackground, maxInFlight, maxInFlight-foregroundSlots)
		}
	})
}

// TestGateWaitsForAnswers pins what the user waits for when only the
// requests in flight make the quota short (coordinator's Q1): a read
// waits for their answers, up to foregroundWait, and goes once one
// frees the quota; if none comes in time, it fails with the end of the
// window.
func TestGateWaitsForAnswers(t *testing.T) {
	watchdog(t)
	synctest.Test(t, func(t *testing.T) {
		reset := time.Now().Add(time.Hour)
		a := answers{"x": make(chan answer, 1), "y": make(chan answer, 1)}
		c := newAnswered(t, a)
		a["x"] <- answer{header: quotaHeader(resourceCore, 5000, 1, reset)}
		if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}
		inFlight := getAsync(c, "x")
		waiting := getAsync(c, "y")
		if n := coreQuota(t, c.RateStatus()).Held; n != 1 {
			t.Fatalf("%d held, want the read that the one in flight makes short", n)
		}
		time.Sleep(time.Second)
		// A 304 costs nothing, so what GitHub reported left covers the read.
		a["x"] <- answer{status: http.StatusNotModified, header: quotaHeader(resourceCore, 5000, 1, reset)}
		if err := <-inFlight; err != nil {
			t.Fatal(err)
		}
		a["y"] <- answer{header: quotaHeader(resourceCore, 5000, 0, reset)}
		if err := <-waiting; err != nil {
			t.Errorf("the read that waited = %v, want it sent", err)
		}

		// None comes in time.
		time.Sleep(time.Until(reset.Add(time.Hour)))
		a["x"] <- answer{header: quotaHeader(resourceCore, 5000, 1, reset.Add(2*time.Hour))}
		if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}
		inFlight = getAsync(c, "x")
		start := time.Now()
		_, err := c.Get(t.Context(), "y", Conditional{}, nil)
		if got, want := limitedUntil(t, err), reset.Add(2*time.Hour+minGuard); !got.Equal(want) {
			t.Errorf("Reset = %v, want the end of the window, %v", got, want)
		}
		if d := time.Since(start); d != foregroundWait+time.Nanosecond {
			t.Errorf("failed after %v, want just after %v", d, foregroundWait)
		}
		a["x"] <- answer{header: quotaHeader(resourceCore, 5000, 0, reset.Add(2*time.Hour))}
		<-inFlight
	})
}

// secondary has h refuse the next requests with secondary limits, whose
// Retry-After each of retryAfter is.
func (h *hub) secondary(retryAfter ...string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.secondaries = append(h.secondaries, retryAfter...)
}

// TestGateSecondary pins secondary limits: one holds
// every resource; what the user waits for waits up to 10s for it and
// fails at once otherwise; once it lifts, the first request held goes
// alone, and the rest a stagger after its answer.
func TestGateSecondary(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5000, window: time.Hour}
		c := newHubClient(t, h, 0)
		h.secondary("30")
		start := time.Now()
		_, err := c.Get(t.Context(), "user", Conditional{}, nil)
		lifts := start.Add(30 * time.Second)
		if got := limitedUntil(t, err); !got.Equal(lifts) {
			t.Errorf("Reset = %v, want %v", got, lifts)
		}
		if got := c.budget.secondaryUntil(); !got.Equal(lifts) {
			t.Errorf("secondaryUntil = %v, want %v", got, lifts)
		}

		// The foreground doesn't wait 30s.
		_, err = c.Get(t.Context(), "repos/o/r", Conditional{}, nil)
		if got := limitedUntil(t, err); !got.Equal(lifts) {
			t.Errorf("Reset of a foreground read = %v, want %v", got, lifts)
		}
		// Every resource is held, not only the one refused.
		background := obs.ForBackground(t.Context())
		search := goAsync(func() error {
			_, err := c.Get(background, "search/issues?q=x", Conditional{}, nil)
			return err
		})
		graphql := goAsync(func() error { return c.Query(background, "query Q { x }", nil, nil) })
		time.Sleep(21 * time.Second)
		// It waits 9s.
		read := goAsync(func() error {
			_, err := c.Get(t.Context(), "repos/o/r/pulls", Conditional{}, nil)
			return err
		})
		for _, done := range []<-chan error{read, search, graphql} {
			if err := <-done; err != nil {
				t.Errorf("held request = %v", err)
			}
		}

		sent, _ := h.requests()
		want := []hubRequest{
			{"user", start},
			{"repos/o/r/pulls", lifts.Add(minStagger)},
			{"search/issues", lifts.Add(2 * minStagger)},
			{"graphql", lifts.Add(3 * minStagger)},
		}
		if len(sent) != len(want) {
			t.Fatalf("sent %v, want %v", sent, want)
		}
		for i, r := range sent {
			if r.path != want[i].path || !r.at.Equal(want[i].at) {
				t.Errorf("request %d = %v, want %v", i, r, want[i])
			}
		}
	})
}

// TestGateSecondaryBackoff pins how long a secondary limit that doesn't
// say lasts: a minute, doubling each time one comes again,
// up to 15 minutes, until a request succeeds.
func TestGateSecondaryBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5000, window: time.Hour}
		c := newHubClient(t, h, 0)
		refused := func(wait time.Duration) {
			t.Helper()
			h.secondary("")
			start := time.Now()
			_, err := c.Get(t.Context(), "user", Conditional{}, nil)
			got := limitedUntil(t, err)
			if want := start.Add(wait); !got.Equal(want) {
				t.Errorf("Reset = %v, want %v later", got, wait)
			}
			time.Sleep(time.Until(got))
		}
		for _, wait := range []time.Duration{1, 2, 4, 8, 15, 15} {
			refused(wait * time.Minute)
		}
		if _, err := c.Get(t.Context(), "user", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}
		refused(time.Minute)
	})
}

// TestGateRetriesSecondaryOnce pins how retry and the gate share a short
// secondary limit: a read the user waits for is sent
// again once, which the gate holds until the limit lifts, so it waits
// once, 3s and a stagger, not twice.
func TestGateRetriesSecondaryOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := gateStats(t)
		h := &hub{limit: 5000, window: time.Hour}
		c := newHubClient(t, h, 0)
		h.secondary("3")
		if _, err := c.Get(t.Context(), "repos/o/r", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}
		sent, _ := h.requests()
		if len(sent) != 2 || sent[1].at.Sub(sent[0].at) != 3*time.Second+minStagger {
			t.Errorf("sent %v, want twice, 3s and a stagger apart", sent)
		}
		if got := stats.Summary().Retries[retryLimit]; got != 1 {
			t.Errorf("sent again %d times for the secondary limit, want 1", got)
		}
	})
}

// TestGateGraphQLSecondaryBackoff pins that a query refused as
// RATE_LIMITED in a 200 isn't a success (coordinator's Q3): a secondary
// limit of GraphQL that doesn't say lasts a minute, then two, as REST's,
// until a query succeeds.
func TestGateGraphQLSecondaryBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"graphql": make(chan answer, 1)}
		c := newAnswered(t, a)
		limited := answer{body: `{"data": null, "errors": [{"type": "RATE_LIMITED", "message": "You have exceeded a secondary rate limit."}]}`}
		for _, wait := range []time.Duration{1, 2, 4} {
			a["graphql"] <- limited
			start := time.Now()
			got := limitedUntil(t, c.Query(t.Context(), "query Q { x }", nil, nil))
			if want := start.Add(wait * time.Minute); !got.Equal(want) {
				t.Errorf("Reset = %v, want %dm later", got, int64(wait))
			}
			time.Sleep(time.Until(got))
		}
		a["graphql"] <- answer{body: `{"data": {"x": 1}}`}
		if err := c.Query(t.Context(), "query Q { x }", nil, nil); err != nil {
			t.Fatal(err)
		}
		a["graphql"] <- limited
		start := time.Now()
		if got := limitedUntil(t, c.Query(t.Context(), "query Q { x }", nil, nil)); !got.Equal(start.Add(time.Minute)) {
			t.Errorf("Reset after a success = %v, want a minute later", got)
		}
	})
}

// TestGateCap pins the sanity cap: a request held for a
// limit that then moves past maxWindow from when it was held fails at
// once, with a rate limit, rather than wait longer.
func TestGateCap(t *testing.T) {
	watchdog(t)
	synctest.Test(t, func(t *testing.T) {
		stats := gateStats(t)
		h := &hub{limit: 5, window: time.Minute}
		c := newHubClient(t, h, 0)
		spendCore(t, c, h, time.Now().Add(55*time.Minute))
		start := time.Now()
		done := goAsync(func() error {
			_, err := c.Get(obs.ForBackground(t.Context()), "repos/o/r", Conditional{}, nil)
			return err
		})
		time.Sleep(30 * time.Minute)
		h.secondary(strconv.Itoa(int((45 * time.Minute).Seconds())))
		_, _ = c.Get(t.Context(), "search/issues?q=x", Conditional{}, nil)
		err := <-done
		if !errors.Is(err, core.ErrRateLimited) {
			t.Errorf("held Get = %v, want a rate limit", err)
		}
		if d := time.Since(start); d != 30*time.Minute {
			t.Errorf("failed after %v, want at once when the limit moved, 30m", d)
		}
		if got := stats.Summary().RateLimit.FailedFast["cap"]; got != 1 {
			t.Errorf("failed for the cap %d times, want 1", got)
		}
	})
}

// TestGateWaitsForScout pins how long what the user waits for waits for
// the answer that tells whether a limit lifted: a read that comes while a
// scout is out, since no probe could tell, fails once it waited
// foregroundWait for a spent quota, or maxSecondaryWait for a secondary
// limit, however long the scout takes.
func TestGateWaitsForScout(t *testing.T) {
	for _, secondary := range []bool{false, true} {
		t.Run(fmt.Sprint("secondary=", secondary), func(t *testing.T) {
			watchdog(t)
			synctest.Test(t, func(t *testing.T) {
				h := &hub{limit: 5, window: time.Minute, noProbe: true,
					delays: map[string]time.Duration{"repos/o/r/scout": 20 * time.Second}}
				c := newHubClient(t, h, 0)
				background := obs.ForBackground(t.Context())
				lifts, wait := time.Now().Add(10*time.Second), maxSecondaryWait
				if secondary {
					h.secondary("10")
					if _, err := c.Get(background, "user", Conditional{}, nil); !errors.Is(err, core.ErrRateLimited) {
						t.Fatalf("Get = %v, want the secondary limit", err)
					}
				} else {
					spendCore(t, c, h, lifts)
					lifts, wait = lifts.Add(minGuard), foregroundWait
				}
				scout := goAsync(func() error {
					_, err := c.Get(background, "repos/o/r/scout", Conditional{}, nil)
					return err
				})
				time.Sleep(time.Until(lifts.Add(100 * time.Millisecond)))
				if sent, _ := h.requests(); sent[len(sent)-1].path != "repos/o/r/scout" {
					t.Fatalf("sent %v, want the scout last", sent)
				}

				start := time.Now()
				_, err := c.Get(t.Context(), "repos/o/r", Conditional{}, nil)
				if !errors.Is(err, core.ErrRateLimited) {
					t.Errorf("read = %v, want a rate limit", err)
				}
				if d := time.Since(start); d != wait+time.Nanosecond {
					t.Errorf("failed after %v, want just after %v", d, wait)
				}
				if err := <-scout; err != nil {
					t.Errorf("scout = %v", err)
				}
			})
		})
	}
}

// TestGateForegroundAheadOfBackground pins that what the user waits for
// never waits behind the background: a read let go while 400 requests of
// the background are being staggered goes a stagger after it is let go,
// not after them.
func TestGateForegroundAheadOfBackground(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const jitter = 0.999
		h := &hub{limit: 1000, window: time.Minute}
		c := newHubClient(t, h, jitter)
		reset := time.Now().Add(10 * time.Second)
		spendCore(t, c, h, reset)
		h.spend(resourceSearch, reset.Add(2*time.Second))
		if _, err := c.Get(t.Context(), "search/issues?q=x", Conditional{}, nil); !errors.Is(err, core.ErrRateLimited) {
			t.Fatalf("Get of a spent quota = %v, want a rate limit", err)
		}

		var wg sync.WaitGroup
		for i := range 400 {
			wg.Go(func() {
				if _, err := c.Get(obs.ForBackground(t.Context()), "repos/o/r/issues/"+strconv.Itoa(i), Conditional{}, nil); err != nil {
					t.Error(err)
				}
			})
		}
		// Core is let go, and its requests are staggered for 4s.
		time.Sleep(time.Until(reset.Add(minGuard + 50*time.Millisecond)))
		if _, err := c.Get(t.Context(), "search/issues?q=y", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}
		release := reset.Add(2*time.Second + minGuard)
		stagger := minStagger + time.Duration(jitter*float64(maxStagger-minStagger))
		if want := release.Add(stagger); !time.Now().Equal(want) {
			t.Errorf("the read was sent at %v, want %v", time.Now(), want)
		}
		wg.Wait()
	})
}

// TestGateScoutFails pins a scout that gets no answer: the next request
// held goes in its place, rather than the queue wait for good.
func TestGateScoutFails(t *testing.T) {
	watchdog(t)
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5, window: time.Minute, noProbe: true}
		c := newHubClient(t, h, 0)
		reset := time.Now().Add(10 * time.Second)
		spendCore(t, c, h, reset)
		h.mu.Lock()
		h.fail = 1
		h.mu.Unlock()

		background := obs.ForBackground(t.Context())
		first := goAsync(func() error {
			_, err := c.Get(background, "repos/o/r/1", Conditional{}, nil)
			return err
		})
		second := goAsync(func() error {
			_, err := c.Get(background, "repos/o/r/2", Conditional{}, nil)
			return err
		})
		if err := <-first; !errors.Is(err, core.ErrOffline) {
			t.Errorf("the scout = %v, want no answer", err)
		}
		if err := <-second; err != nil {
			t.Errorf("the next held = %v, want it sent", err)
		}
		scout := reset.Add(minGuard + minStagger)
		if sent, _ := h.requests(); len(sent) != 3 || !sent[2].at.Equal(scout.Add(minStagger)) {
			t.Errorf("sent %v, want the next held at %v", sent, scout.Add(minStagger))
		}
	})
}

// TestGateScoutPastTooLate pins the hand-off of the scout when no probe
// can tell whether a limit lifted: the first request held, whose deadline
// is too close for its turn, fails, and the next goes in its place, rather
// than the queue wait with no scout out.
func TestGateScoutPastTooLate(t *testing.T) {
	watchdog(t)
	synctest.Test(t, func(t *testing.T) {
		stats := gateStats(t)
		h := &hub{limit: 5, window: time.Minute, noProbe: true}
		c := newHubClient(t, h, 0)
		reset := time.Now().Add(10 * time.Second)
		spendCore(t, c, h, reset)
		release := reset.Add(minGuard)

		background := obs.ForBackground(t.Context())
		// Held, since it may be sent by the release, but not a stagger later.
		ctx, cancel := context.WithDeadline(background, release.Add(deadlineMargin+minStagger/2))
		defer cancel()
		first := goAsync(func() error {
			_, err := c.Get(ctx, "repos/o/r/first", Conditional{}, nil)
			return err
		})
		dones := make([]<-chan error, 0, 2)
		for _, path := range []string{"repos/o/r/scout", "repos/o/r/next"} {
			dones = append(dones, goAsync(func() error {
				_, err := c.Get(background, path, Conditional{}, nil)
				return err
			}))
		}
		if err := <-first; !errors.Is(err, core.ErrRateLimited) {
			t.Errorf("the first held = %v, want a rate limit for its deadline", err)
		}
		for _, done := range dones {
			if err := <-done; err != nil {
				t.Errorf("held Get = %v, want it sent", err)
			}
		}
		sent, _ := h.requests()
		want := []hubRequest{{"repos/o/r/scout", release.Add(minStagger)}, {"repos/o/r/next", release.Add(2 * minStagger)}}
		if len(sent) != 3 || sent[1] != want[0] || sent[2] != want[1] {
			t.Errorf("sent %v, want the scout and then the next, %v", sent, want)
		}
		if got := stats.Summary().RateLimit.FailedFast["deadline"]; got != 1 {
			t.Errorf("failed for the deadline %d times, want 1", got)
		}
	})
}

// TestGateNoTimerAfterClose pins that once the client is closed, the gate
// neither sets a timer for a queue that comes after, nor sets again the
// timer of one it stopped.
func TestGateNoTimerAfterClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5, window: time.Minute}
		c := newHubClient(t, h, 0)
		spendCore(t, c, h, time.Now().Add(10*time.Minute))
		ctx, cancel := context.WithCancel(obs.ForBackground(t.Context()))
		defer cancel()
		get := func(path string) <-chan error {
			return goAsync(func() error {
				_, err := c.Get(ctx, path, Conditional{}, nil)
				return err
			})
		}
		dones := make([]<-chan error, 0, 3)
		dones = append(dones, get("repos/o/r/1"))
		c.Close()

		// Another held pumps the queue, which would set its timer again.
		dones = append(dones, get("repos/o/r/2"))
		h.spend(resourceSearch, time.Now().Add(10*time.Minute))
		if _, err := c.Get(t.Context(), "search/issues?q=x", Conditional{}, nil); !errors.Is(err, core.ErrRateLimited) {
			t.Fatalf("Get of a spent quota = %v, want a rate limit", err)
		}
		dones = append(dones, get("search/issues?q=y"))
		c.budget.mu.Lock()
		coreTimer, search := c.budget.gate.queues[resourceCore].timer, c.budget.gate.queues[resourceSearch]
		c.budget.mu.Unlock()
		if coreTimer.Stop() {
			t.Error("the timer of core was set again after Close")
		}
		if search == nil || search.timer != nil {
			t.Errorf("the queue of search after Close = %+v, want one held and no timer", search)
		}
		cancel()
		for _, done := range dones {
			<-done
		}
	})
}

// TestGateExpiryFails pins that when a request held can wait no longer
// (limit.expiry), it is too late for it then (hold.tooLate): otherwise the
// timer set for that instant would find it may still wait, and set itself
// for the same instant again, for good.
func TestGateExpiryFails(t *testing.T) {
	now := time.Now()
	for _, l := range []limit{{inFlight: true}, {inFlight: true, secondary: true}, {until: now.Add(2 * maxWindow)}} {
		for _, k := range []class{classForeground, classMutation, classBackground} {
			h := &hold{class: k, at: now}
			at := l.expiry([]*hold{h})
			if why := h.tooLate(l, at); why == "" {
				t.Errorf("a %v request held for a limit (in flight %t, secondary %t) may still wait at its expiry, %v after it was held",
					k, l.inFlight, l.secondary, at.Sub(now))
			}
		}
	}
}

// TestGateDropAfterLetGo pins a request whose context ends as it is let
// go: dropping it hands back its reservation, so that it is settled rather
// than taken from the quota for good.
func TestGateDropAfterLetGo(t *testing.T) {
	b := newBudget("api.github.com", "/", "/graphql")
	now := time.Now()
	h := &hold{class: classBackground, resource: resourceCore, cost: 1, at: now}
	b.mu.Lock()
	b.enqueue(h, now)
	b.letGo(b.gate.queues[resourceCore], h, now)
	b.mu.Unlock()
	if r := b.drop(h); r == nil || r != h.r {
		t.Fatalf("drop = %v, want the reservation %v", r, h.r)
	}
	b.forget(h.r, false)
	if len(b.pending) != 0 {
		t.Errorf("%d pending, want none", len(b.pending))
	}
}

// TestGateLiftingHolds pins what comes while the scout of a secondary
// limit that lifted is out: it is held until the scout is answered, and
// then goes a stagger later.
func TestGateLiftingHolds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5000, window: time.Hour, delays: map[string]time.Duration{"repos/o/r/scout": time.Second}}
		c := newHubClient(t, h, 0)
		background := obs.ForBackground(t.Context())
		h.secondary("3")
		start := time.Now()
		if _, err := c.Get(background, "user", Conditional{}, nil); !errors.Is(err, core.ErrRateLimited) {
			t.Fatalf("Get = %v, want the secondary limit", err)
		}
		scout := goAsync(func() error {
			_, err := c.Get(background, "repos/o/r/scout", Conditional{}, nil)
			return err
		})
		lifts := start.Add(3 * time.Second)
		time.Sleep(time.Until(lifts.Add(100 * time.Millisecond)))
		if _, err := c.Get(t.Context(), "repos/o/r", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}
		if want := lifts.Add(minStagger + time.Second + minStagger); !time.Now().Equal(want) {
			t.Errorf("the read was sent at %v, want once the scout was answered, %v", time.Now(), want)
		}
		if err := <-scout; err != nil {
			t.Error(err)
		}
	})
}

// TestGateDeadlineAtItsTurn pins the deadline of a held request at its
// turn: one with time to spare when it is held, that the stagger of those
// let go before it leaves too close to its deadline, fails then, with when
// its turn was as its reset, and isn't sent.
func TestGateDeadlineAtItsTurn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := gateStats(t)
		h := &hub{limit: 50, window: time.Minute}
		c := newHubClient(t, h, 0)
		reset := time.Now().Add(10 * time.Second)
		spendCore(t, c, h, reset)
		release := reset.Add(minGuard)

		background := obs.ForBackground(t.Context())
		dones := make([]<-chan error, 0, 5)
		for i := range 5 {
			dones = append(dones, goAsync(func() error {
				_, err := c.Get(background, "repos/o/r/"+strconv.Itoa(i), Conditional{}, nil)
				return err
			}))
		}
		ctx, cancel := context.WithDeadline(background, release.Add(deadlineMargin+3*minStagger))
		defer cancel()
		_, err := c.Get(ctx, "repos/o/r/late", Conditional{}, nil)
		if got, want := limitedUntil(t, err), release.Add(6*minStagger); !got.Equal(want) {
			t.Errorf("Reset = %v, want its turn, %v", got, want)
		}
		for _, done := range dones {
			if err := <-done; err != nil {
				t.Error(err)
			}
		}
		for _, r := range func() []hubRequest { sent, _ := h.requests(); return sent }() {
			if r.path == "repos/o/r/late" {
				t.Errorf("the late request was sent at %v", r.at)
			}
		}
		if got := stats.Summary().RateLimit.FailedFast["deadline"]; got != 1 {
			t.Errorf("failed for the deadline %d times, want 1", got)
		}
	})
}

// TestGatePrefetchWhileLimited pins reads ahead while a limit is on: one
// fails at once, even when the limit lifts sooner than a read the user
// waits for would wait, and isn't held.
func TestGatePrefetchWhileLimited(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := gateStats(t)
		h := &hub{limit: 5, window: time.Minute}
		c := newHubClient(t, h, 0)
		// A read the user waits for would wait 2s.
		reset := time.Now().Add(time.Second)
		spendCore(t, c, h, reset)

		start := time.Now()
		_, err := c.Get(obs.ForPrefetch(t.Context()), "repos/o/r", Conditional{}, nil)
		if got := limitedUntil(t, err); !got.Equal(reset.Add(minGuard)) {
			t.Errorf("Reset = %v, want the release %v", got, reset.Add(minGuard))
		}
		if d := time.Since(start); d != 0 {
			t.Errorf("failed after %v, want at once", d)
		}
		if got := stats.Summary().RateLimit.FailedFast["prefetch"]; got != 1 {
			t.Errorf("failed fast for the prefetch %d times, want 1", got)
		}
		if sent, _ := h.requests(); len(sent) != 1 {
			t.Errorf("sent %v, want only the first request", sent)
		}
	})
}

// TestGateClose pins that closing the client stops the timers of the
// gate, which would otherwise fire after it.
func TestGateClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5, window: time.Minute}
		c := newHubClient(t, h, 0)
		spendCore(t, c, h, time.Now().Add(10*time.Minute))
		ctx, cancel := context.WithCancel(obs.ForBackground(t.Context()))
		done := goAsync(func() error {
			_, err := c.Get(ctx, "repos/o/r", Conditional{}, nil)
			return err
		})
		c.Close()
		c.budget.mu.Lock()
		stopped := !c.budget.gate.queues[resourceCore].timer.Stop()
		c.budget.mu.Unlock()
		if !stopped {
			t.Error("the timer of the queue still runs after Close")
		}
		cancel()
		<-done
	})
}
