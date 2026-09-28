package github

import (
	"context"
	"errors"
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

// arrivals is a base transport that keeps the requests that reached base,
// and when.
type arrivals struct {
	base http.RoundTripper
	mu   sync.Mutex
	log  []hubRequest
}

func (a *arrivals) RoundTrip(req *http.Request) (*http.Response, error) {
	a.mu.Lock()
	a.log = append(a.log, hubRequest{strings.TrimPrefix(req.URL.Path, "/"), time.Now()})
	a.mu.Unlock()
	return a.base.RoundTrip(req)
}

// requests returns the requests to paths that start with prefix.
func (a *arrivals) requests(prefix string) []hubRequest {
	a.mu.Lock()
	defer a.mu.Unlock()
	var out []hubRequest
	for _, r := range a.log {
		if strings.HasPrefix(r.path, prefix) {
			out = append(out, r)
		}
	}
	return out
}

// unrecalled is a base transport that fails t if an attempt that reached
// it was recalled, as it came or while it was on its way: once an attempt
// is sent, nothing may recall it.
type unrecalled struct {
	base http.RoundTripper
	t    *testing.T
}

func (u unrecalled) RoundTrip(req *http.Request) (*http.Response, error) {
	check := func() {
		if errors.Is(context.Cause(req.Context()), errRecalled) {
			u.t.Errorf("%s was recalled after it was sent", req.URL.Path)
		}
	}
	check()
	resp, err := u.base.RoundTrip(req)
	check()
	return resp, err
}

// TestGateRecall pins the recall of what was let through but not sent
// (design 5, test 12): with the 8 slots taken and 3 requests of the
// background waiting for one, an answer that says the quota is spent
// sends the 3 back to the gate, which holds them, and the base never sees
// them before the limit lifts; the 8 in flight complete.
func TestGateRecall(t *testing.T) {
	watchdog(t)
	synctest.Test(t, func(t *testing.T) {
		stats := gateStats(t)
		reset := time.Now().Add(10 * time.Minute)
		a := answers{"x": make(chan answer, 1)}
		for i := range maxInFlight {
			a["f"+strconv.Itoa(i)] = make(chan answer, 1)
		}
		for i := range 3 {
			a["b"+strconv.Itoa(i)] = make(chan answer, 1)
		}
		base := &arrivals{base: a}
		c, err := New(WithBaseURL("https://api.github.com/"), WithToken("t"), WithHTTPClient(&http.Client{Transport: base}))
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		a["x"] <- answer{header: quotaHeader(resourceCore, 5000, 5000, reset)}
		if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}

		inFlight := make([]<-chan error, 0, maxInFlight)
		for i := range maxInFlight {
			inFlight = append(inFlight, getAsync(c, "f"+strconv.Itoa(i)))
		}
		background := obs.ForBackground(t.Context())
		waiting := make([]<-chan error, 0, 3)
		for i := range 3 {
			waiting = append(waiting, goAsync(func() error {
				_, err := c.Get(background, "b"+strconv.Itoa(i), Conditional{}, nil)
				return err
			}))
		}
		if n := coreQuota(t, c.RateStatus()).Held; n != 0 {
			t.Fatalf("%d held before the limit, want none", n)
		}

		a["f0"] <- answer{header: quotaHeader(resourceCore, 5000, 0, reset)}
		synctest.Wait()
		if n := coreQuota(t, c.RateStatus()).Held; n != 3 {
			t.Errorf("%d held once the quota is spent, want the 3 recalled", n)
		}
		for i := range maxInFlight {
			if i > 0 {
				a["f"+strconv.Itoa(i)] <- answer{header: quotaHeader(resourceCore, 5000, 0, reset)}
			}
			if err := <-inFlight[i]; err != nil {
				t.Errorf("request %d in flight = %v", i, err)
			}
		}
		synctest.Wait()
		if sent := base.requests("b"); len(sent) != 0 {
			t.Errorf("sent %v, want the recalled ones held", sent)
		}
		var recalled int64
		for _, r := range stats.Summary().RateLimit.Resources {
			recalled += r.Recalled
		}
		if recalled != 3 {
			t.Errorf("%d recalled, want 3", recalled)
		}
		// A recall is no failure to reach GitHub, which would show the
		// connection as lost.
		if _, failed := c.budget.contact(); !failed.IsZero() {
			t.Errorf("a request failed at %v, want the recalls not counted as failures", failed)
		}

		// No probe answers, so the first held finds out whether the limit
		// lifted, and the others follow.
		for i := range 3 {
			a["b"+strconv.Itoa(i)] <- answer{header: quotaHeader(resourceCore, 5000, 4999, reset.Add(time.Hour))}
		}
		for _, done := range waiting {
			if err := <-done; err != nil {
				t.Errorf("recalled request = %v", err)
			}
		}
		release := reset.Add(minGuard)
		for _, r := range base.requests("b") {
			if r.at.Before(release) {
				t.Errorf("%s sent at %v, before the limit lifted at %v", r.path, r.at, release)
			}
		}
	})
}

// TestGateRecallBeforeItsTurn pins a request recalled while it waits for
// its turn after a hold, before it has an attempt to end: it goes back to
// the gate as it would from the wait for a slot.
func TestGateRecallBeforeItsTurn(t *testing.T) {
	b := newBudget("api.github.com", "/", "/graphql")
	now := time.Now()
	b.mu.Lock()
	b.quotas[resourceCore] = &quota{limit: 5000, remaining: 5000, reset: now.Add(time.Hour), guard: minGuard}
	r := b.reserve(resourceCore, "", 1, now)
	b.quotas[resourceCore].release = now.Add(time.Minute)
	b.recall(now)
	b.mu.Unlock()
	if b.track(r, func(error) {}) {
		t.Error("track of a recalled request = true, want it sent back to the gate")
	}
	if err := b.send(r); !errors.Is(err, errRecalled) {
		t.Errorf("send of a recalled request = %v, want errRecalled", err)
	}
}

// TestGateSentNotRecalled pins that recall passes over a request once it
// is sent, and that one not sent yet is recalled, and its attempt ended
// with errRecalled as the cause.
func TestGateSentNotRecalled(t *testing.T) {
	b := newBudget("api.github.com", "/", "/graphql")
	now := time.Now()
	b.mu.Lock()
	b.quotas[resourceCore] = &quota{limit: 5000, remaining: 5000, reset: now.Add(time.Hour), guard: minGuard}
	sent, unsent := b.reserve(resourceCore, "", 1, now), b.reserve(resourceCore, "", 1, now)
	b.mu.Unlock()
	sentCtx, cancelSent := context.WithCancelCause(t.Context())
	unsentCtx, cancelUnsent := context.WithCancelCause(t.Context())
	if !b.track(sent, cancelSent) || !b.track(unsent, cancelUnsent) {
		t.Fatal("track = false before any limit")
	}
	if err := b.send(sent); err != nil {
		t.Fatalf("send = %v", err)
	}
	b.mu.Lock()
	b.quotas[resourceCore].release = now.Add(time.Minute)
	b.recall(now)
	b.mu.Unlock()
	if err := context.Cause(sentCtx); err != nil {
		t.Errorf("the attempt sent ended with %v, want it left alone", err)
	}
	if err := context.Cause(unsentCtx); !errors.Is(err, errRecalled) {
		t.Errorf("the attempt not sent ended with %v, want errRecalled", err)
	}
}

// TestGateRecallKeepsTheWait pins that a read the user waits for waits no
// longer in all for being recalled: held 1.5s, let go, and recalled before
// its turn by a limit that lifts 3.5s after it came, it fails at once,
// within foregroundWait of when it came, rather than wait anew.
func TestGateRecallKeepsTheWait(t *testing.T) {
	watchdog(t)
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		a := answers{"x": make(chan answer, 1), "y": make(chan answer, 1), "search/issues": make(chan answer, 1)}
		c := newAnswered(t, a)
		defer c.Close()
		c.budget.gate.jitter = func() float64 { return 0.999 }
		// Core is spent until 2s from now, its reset and a guard.
		a["x"] <- answer{header: quotaHeader(resourceCore, 5000, 0, start.Add(time.Second))}
		if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
			t.Fatal(err)
		}
		inFlight := getAsync(c, "search/issues")
		time.Sleep(500 * time.Millisecond)
		came := time.Now()
		// Answered only if it is sent, which it mustn't be.
		a["y"] <- answer{header: quotaHeader(resourceCore, 5000, 4999, start.Add(time.Hour))}
		read := goAsync(func() error {
			_, err := c.Get(t.Context(), "y", Conditional{}, nil)
			return err
		})
		// No probe answers, so the read is let go at the release to find
		// out, and waits for its turn a stagger later, when an answer
		// says that core is spent again.
		time.Sleep(time.Until(start.Add(2*time.Second + maxStagger/2)))
		a["search/issues"] <- answer{header: quotaHeader(resourceCore, 5000, 0, start.Add(3*time.Second))}
		if err := <-inFlight; err != nil {
			t.Fatal(err)
		}
		err := <-read
		if !errors.Is(err, core.ErrRateLimited) {
			t.Errorf("recalled read = %v, want a rate limit", err)
		}
		if d := time.Since(came); d > foregroundWait {
			t.Errorf("recalled read came back %v after it came, want within %v", d, foregroundWait)
		}
	})
}

// TestGateScoutNotRecalled pins that a request let go after the release of
// its quota passed, to find out whether the limit lifted, isn't recalled
// for that limit.
func TestGateScoutNotRecalled(t *testing.T) {
	b := newBudget("api.github.com", "/", "/graphql")
	now := time.Now()
	b.mu.Lock()
	b.quotas[resourceCore] = &quota{limit: 5000, reset: now.Add(-time.Second), release: now, guard: minGuard}
	r := b.reserve(resourceCore, "", 1, now)
	b.recall(now)
	b.mu.Unlock()
	if !b.track(r, func(error) {}) {
		t.Error("the request let go after the release was recalled")
	}
}

// TestGateAttemptEndsOnClose pins that the context of an attempt, which
// recall would end, ends once its body is closed, rather than stay with
// its parent's for as long as that lives.
func TestGateAttemptEndsOnClose(t *testing.T) {
	var ctx context.Context
	base := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		ctx = req.Context()
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
	})
	c, err := New(WithBaseURL("https://api.github.com/"), WithToken("t"), WithHTTPClient(&http.Client{Transport: base}))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
		t.Fatal(err)
	}
	if ctx == nil || ctx.Err() == nil {
		t.Error("the attempt's context lives on after its body was closed")
	}
}
