package github

import (
	"net/http"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
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
// flight, and never goes below zero when more are in flight than is left.
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
