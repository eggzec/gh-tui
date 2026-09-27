package github

import (
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// answerQuota answers with the quota of resource, from GitHub.
func answerQuota(resource string, limit, remaining int, reset time.Time) answer {
	h := quotaHeader(resource, limit, remaining, reset)
	h["X-GitHub-Request-Id"] = "ABCD:1234"
	return answer{header: h}
}

// coreQuota returns the core quota of s.
func coreQuota(t *testing.T, s core.RateStatus) core.Quota {
	t.Helper()
	i := slices.IndexFunc(s.Quotas, func(q core.Quota) bool { return q.Resource == resourceCore })
	if i < 0 {
		t.Fatalf("no core quota in %+v", s.Quotas)
	}
	return s.Quotas[i]
}

// TestRateStatusOrder checks that the resources gh-tui uses most come
// first, and the others after them by name.
func TestRateStatusOrder(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		if got := c.RateStatus().Quotas; len(got) != 0 {
			t.Fatalf("Quotas before any answer = %+v, want none", got)
		}
		reset := time.Now().Add(time.Hour)
		for _, r := range []string{"source_import", resourceCodeSearch, resourceSearch, "audit_log", resourceCore, resourceGraphQL} {
			a["x"] <- answerQuota(r, 100, 50, reset)
			if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
				t.Fatalf("Get: %v", err)
			}
		}
		quotas := c.RateStatus().Quotas
		got := make([]string, 0, len(quotas))
		for _, q := range quotas {
			got = append(got, q.Resource)
		}
		want := []string{resourceCore, resourceGraphQL, resourceSearch, resourceCodeSearch, "audit_log", "source_import"}
		if !slices.Equal(got, want) {
			t.Errorf("resources = %v, want %v", got, want)
		}
	})
}

// TestRateStatusRemaining checks that Remaining counts the requests in
// flight, never goes below zero when more are in flight than is left, and
// is the full limit once the reset has passed.
func TestRateStatusRemaining(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 3)}
		c := newAnswered(t, a)
		reset := time.Now().Add(time.Hour)
		a["x"] <- answerQuota(resourceCore, 5000, 2, reset)
		if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
			t.Fatalf("Get: %v", err)
		}

		first := getAsync(c, "x")
		if got := coreQuota(t, c.RateStatus()); got.Remaining != 1 || got.Limit != 5000 {
			t.Errorf("with one in flight: Remaining, Limit = %d, %d; want 1, 5000", got.Remaining, got.Limit)
		}
		second := getAsync(c, "x")
		third := getAsync(c, "x")
		if got := coreQuota(t, c.RateStatus()); got.Remaining != 0 {
			t.Errorf("with three in flight of two left: Remaining = %d, want 0", got.Remaining)
		}
		for _, done := range []<-chan error{first, second, third} {
			a["x"] <- answerQuota(resourceCore, 5000, 2, reset)
			<-done
		}
		if got := coreQuota(t, c.RateStatus()); got.Remaining != 2 {
			t.Errorf("once answered: Remaining = %d, want 2", got.Remaining)
		}

		// Past the reset the window has refilled, before GitHub says so.
		time.Sleep(time.Until(reset))
		next := getAsync(c, "x")
		if got := coreQuota(t, c.RateStatus()); got.Remaining != 4999 || !got.Reset.Equal(reset) {
			t.Errorf("past the reset, one in flight: Remaining, Reset = %d, %v; want 4999, %v", got.Remaining, got.Reset, reset)
		}
		a["x"] <- answerQuota(resourceCore, 5000, 4990, reset.Add(time.Hour))
		<-next
		if got := coreQuota(t, c.RateStatus()); got.Remaining != 4990 || !got.Reset.Equal(reset.Add(time.Hour)) {
			t.Errorf("once GitHub reports the new window: Remaining, Reset = %d, %v; want 4990, the new reset", got.Remaining, got.Reset)
		}
	})
}

// TestRateStatusLocalTimes answers from a GitHub whose clock is 5s ahead,
// with core spent, and checks that the reset and the release are in local
// time, and that the resource shows as limited only until its release.
func TestRateStatusLocalTimes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const skew = 5 * time.Second
		reset := time.Now().Add(skew + 10*time.Minute) // in GitHub's clock
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		a["x"] <- spent(resourceCore, reset, skew)
		_, _ = c.Get(t.Context(), "x", Conditional{}, nil)

		s := c.RateStatus()
		q := coreQuota(t, s)
		local := reset.Add(-skew)
		if !q.Reset.Equal(local) {
			t.Errorf("Reset = %v, want %v, in local time", q.Reset, local)
		}
		if want := local.Add(minGuard); !q.LimitedUntil.Equal(want) {
			t.Errorf("LimitedUntil = %v, want %v, the release", q.LimitedUntil, want)
		}
		if q.Remaining != 0 || q.Held != 0 || !q.SeenAt.Equal(time.Now()) || !s.At.Equal(time.Now()) {
			t.Errorf("quota = %+v at %v, want none left or held, seen now", q, s.At)
		}
		if !s.SecondaryUntil.IsZero() {
			t.Errorf("SecondaryUntil = %v, want zero", s.SecondaryUntil)
		}

		time.Sleep(time.Until(q.LimitedUntil))
		if q := coreQuota(t, c.RateStatus()); !q.LimitedUntil.IsZero() {
			t.Errorf("LimitedUntil after the release = %v, want zero", q.LimitedUntil)
		}
	})
}

// TestRateStatusContact checks that the status says when GitHub last
// answered and when a request last failed.
func TestRateStatusContact(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newAnswered(t, a)
		if s := c.RateStatus(); !s.Answered.IsZero() || !s.Failed.IsZero() {
			t.Errorf("before any request: Answered, Failed = %v, %v; want zero", s.Answered, s.Failed)
		}
		a["x"] <- answer{status: http.StatusNotFound, header: map[string]string{"X-GitHub-Request-Id": "ABCD:1234"}}
		_, _ = c.Get(t.Context(), "x", Conditional{}, nil)
		answered := time.Now()

		time.Sleep(time.Second)
		_, _ = c.Get(t.Context(), "unanswered", Conditional{}, nil)
		s := c.RateStatus()
		if !s.Answered.Equal(answered) || !s.Failed.Equal(time.Now()) {
			t.Errorf("Answered, Failed = %v, %v; want %v, %v", s.Answered, s.Failed, answered, time.Now())
		}
	})
}

// notified is a client that keeps the status it reads each time it is
// told the rate limits changed, as the app does.
type notified struct {
	*Client
	mu   sync.Mutex
	told []core.RateStatus
}

func newNotified(t *testing.T, base http.RoundTripper) *notified {
	t.Helper()
	n := &notified{}
	c, err := New(WithBaseURL("https://api.github.com/"), WithToken("t"), WithHTTPClient(&http.Client{Transport: base}),
		WithRateNotify(func() {
			// Read under no lock of the client's, or this would deadlock.
			s := n.RateStatus()
			n.mu.Lock()
			n.told = append(n.told, s)
			n.mu.Unlock()
		}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	n.Client = retryAtOnce(c)
	return n
}

// last returns how many times n was told, and the status it read last.
func (n *notified) last() (int, core.RateStatus) {
	synctest.Wait()
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.told) == 0 {
		return 0, core.RateStatus{}
	}
	return len(n.told), n.told[len(n.told)-1]
}

// TestRateNotifyThrottle answers many requests within a second, and
// checks that they are told of once at once and once a second later,
// with the last state.
func TestRateNotifyThrottle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newNotified(t, a)
		reset := time.Now().Add(time.Hour)
		for i := range 50 {
			a["x"] <- answerQuota(resourceCore, 100, 99-i, reset)
			if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
				t.Fatalf("Get: %v", err)
			}
		}
		if n, s := c.last(); n != 1 || coreQuota(t, s).Remaining != 99 {
			t.Fatalf("told %d times, last of %+v; want once, of the first answer", n, s.Quotas)
		}
		time.Sleep(notifyEvery)
		if n, s := c.last(); n != 2 || coreQuota(t, s).Remaining != 50 {
			t.Fatalf("a second later: told %d times, last of %+v; want twice, of the last answer", n, s.Quotas)
		}
		time.Sleep(time.Minute)
		if n, _ := c.last(); n != 2 {
			t.Errorf("with no change since: told %d times, want 2", n)
		}
	})
}

// TestRateNotifyChanges goes through the changes that are told of, and
// some that aren't, a while apart so that none waits for the throttle.
func TestRateNotifyChanges(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newNotified(t, a)
		reset := time.Now().Add(30 * time.Minute)
		told := 0
		step := func(name string, change bool, ans *answer, path string) {
			t.Helper()
			time.Sleep(time.Minute)
			if ans != nil {
				a["x"] <- *ans
			}
			if path != "" {
				_, _ = c.Get(t.Context(), path, Conditional{}, nil)
			}
			if change {
				told++
			}
			if n, _ := c.last(); n != told {
				t.Errorf("%s: told %d times, want %d", name, n, told)
				told = n
			}
		}
		ans := func(a answer) *answer { return &a }

		step("first quota", true, ans(answerQuota(resourceCore, 5000, 4990, reset)), "x")
		step("less left, same percent", false, ans(answerQuota(resourceCore, 5000, 4960, reset)), "x")
		step("less left, a percent less", true, ans(answerQuota(resourceCore, 5000, 4949, reset)), "x")
		step("another resource", true, ans(answerQuota(resourceSearch, 30, 30, reset)), "x")
		// Near enough to be real.
		reset = reset.Add(30 * time.Minute)
		step("a new window", true, ans(answerQuota(resourceCore, 5000, 4949, reset)), "x")
		step("spent", true, ans(spent(resourceCore, reset, 0)), "x")
		// The window refills at its reset, and the resource is released
		// a guard later, each with no answer.
		spentQuota := coreQuota(t, c.RateStatus())
		time.Sleep(time.Until(spentQuota.Reset))
		told++
		if n, s := c.last(); n != told || coreQuota(t, s).Remaining != 5000 || coreQuota(t, s).LimitedUntil.IsZero() {
			t.Errorf("at the reset: told %d times, last of %+v; want %d, the full limit, still limited", n, s.Quotas, told)
			told = n
		}
		time.Sleep(time.Until(spentQuota.LimitedUntil))
		told++
		if n, s := c.last(); n != told || !coreQuota(t, s).LimitedUntil.IsZero() {
			t.Errorf("at the release: told %d times, last of %+v; want %d, not limited", n, s.Quotas, told)
			told = n
		}
		// Until an answer shows the new window, the gate holds each
		// request for a probe of the limits, which is told of too.
		reset = reset.Add(time.Hour)
		step("held for a probe", true, ans(answerQuota(resourceCore, 5000, 5000, reset)), "x")
		time.Sleep(notifyEvery)
		told++
		if n, s := c.last(); n != told || coreQuota(t, s).Held != 0 || coreQuota(t, s).Remaining != 5000 {
			t.Errorf("let go in the new window: told %d times, last of %+v; want %d, none held", n, s.Quotas, told)
			told = n
		}
		step("offline", true, nil, "unanswered")
		step("still offline", false, nil, "unanswered")
		step("online", true, ans(answerQuota(resourceCore, 5000, 4999, reset)), "x")
		step("still online", false, ans(answerQuota(resourceCore, 5000, 4998, reset)), "x")
	})
}

// TestRateNotifyClose checks that a change waiting for the throttle isn't
// told of once the client is closed, nor a release.
func TestRateNotifyClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := answers{"x": make(chan answer, 1)}
		c := newNotified(t, a)
		reset := time.Now().Add(time.Hour)
		a["x"] <- answerQuota(resourceCore, 100, 50, reset)
		_, _ = c.Get(t.Context(), "x", Conditional{}, nil)
		a["x"] <- spent(resourceCore, reset, 0)
		_, _ = c.Get(t.Context(), "x", Conditional{}, nil)
		c.Close()
		time.Sleep(2 * time.Hour)
		if n, _ := c.last(); n != 1 {
			t.Errorf("told %d times, want only before Close", n)
		}
		a["x"] <- answerQuota(resourceCore, 100, 100, reset.Add(time.Hour))
		if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
			t.Errorf("Get after Close: %v", err)
		}
		if n, _ := c.last(); n != 1 {
			t.Errorf("after a request: told %d times, want 1", n)
		}
	})
}

// TestRateStatusConcurrent sends many requests at once while reading the
// status, for the race detector, and checks what is told once they end.
func TestRateStatusConcurrent(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const workers, each = 64, 20
		reset := time.Now().Add(time.Hour)
		var sent atomic.Int64
		c := newNotified(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			n := sent.Add(1)
			resource := resourceCore
			if strings.HasPrefix(req.URL.Path, "/search/") {
				resource = resourceSearch
			}
			h := make(http.Header)
			for k, v := range quotaHeader(resource, 5000, 5000-int(n), reset) {
				h.Set(k, v)
			}
			h.Set("X-GitHub-Request-Id", strconv.FormatInt(n, 10))
			return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
		}))
		var wg sync.WaitGroup
		for i := range workers {
			wg.Go(func() {
				path := "x"
				if i%2 == 0 {
					path = "search/issues"
				}
				for range each {
					_, _ = c.Get(t.Context(), path, Conditional{}, nil)
					s := c.RateStatus()
					if !slices.IsSortedFunc(s.Quotas, func(x, y core.Quota) int {
						return resourceRank(x.Resource) - resourceRank(y.Resource)
					}) {
						t.Errorf("Quotas out of order: %+v", s.Quotas)
					}
				}
			})
		}
		wg.Wait()
		if n, _ := c.last(); n > 2 {
			t.Errorf("told %d times at one instant, want at most twice", n)
		}
		time.Sleep(notifyEvery)
		_, s := c.last()
		if len(s.Quotas) != 2 {
			t.Fatalf("Quotas = %+v, want core and search", s.Quotas)
		}
		// Each resource's lowest answer wins, whatever the order they came
		// in; what is left is below the last sent's by up to its count.
		for _, q := range s.Quotas {
			if q.Held != 0 || q.Remaining > 5000-workers*each/2 || q.Remaining < 5000-workers*each {
				t.Errorf("%s: Remaining %d, Held %d; want at most %d, none held", q.Resource, q.Remaining, q.Held, 5000-workers*each/2)
			}
		}
		if want := c.RateStatus(); !slices.Equal(want.Quotas, s.Quotas) {
			t.Errorf("last told %+v, want the final %+v", s.Quotas, want.Quotas)
		}
	})
}

// TestRateStatusHeld checks that the requests held for a limit show in
// the status, and that holding one and letting it go are told of.
func TestRateStatusHeld(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		h := &hub{limit: 5, window: time.Minute}
		c := newNotified(t, h)
		c.budget.gate.jitter = func() float64 { return 0 }
		reset := time.Now().Add(10 * time.Second)
		spendCore(t, c.Client, h, reset)
		time.Sleep(2 * time.Second)
		told, _ := c.last()

		done := goAsync(func() error {
			_, err := c.Get(obs.ForBackground(t.Context()), "repos/o/r", Conditional{}, nil)
			return err
		})
		n, s := c.last()
		if n != told+1 || coreQuota(t, s).Held != 1 || coreQuota(t, c.RateStatus()).Held != 1 {
			t.Errorf("once held: told %d times, last of %+v; want %d, 1 held", n, s.Quotas, told+1)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		time.Sleep(notifyEvery)
		if n, s := c.last(); n <= told+1 || coreQuota(t, s).Held != 0 {
			t.Errorf("once let go: told %d times, last of %+v; want more than %d, none held", n, s.Quotas, told+1)
		}
	})
}
