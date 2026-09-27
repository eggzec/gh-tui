package github

import (
	"errors"
	"maps"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestRateLimitParsed(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "4999")
		w.Header().Set("X-RateLimit-Reset", "1790000000")
		w.Header().Set("X-RateLimit-Resource", "core")
	}))
	if got := c.RateLimit(resourceCore); got != (RateLimit{}) {
		t.Errorf("RateLimit before any request = %+v, want zero", got)
	}

	res, err := c.Get(t.Context(), "x", Conditional{}, nil)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	want := RateLimit{Limit: 5000, Remaining: 4999, Reset: time.Unix(1790000000, 0), Resource: "core"}
	if res.RateLimit != want {
		t.Errorf("Response.RateLimit = %+v, want %+v", res.RateLimit, want)
	}
	if got := c.RateLimit(resourceCore); got != want {
		t.Errorf("Client.RateLimit = %+v, want %+v", got, want)
	}
}

func TestRateLimitKeptWithoutHeaders(t *testing.T) {
	var calls atomic.Int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("X-RateLimit-Limit", "60")
			w.Header().Set("X-RateLimit-Remaining", "59")
			w.Header().Set("X-RateLimit-Reset", "1790000000")
		}
	}))
	for range 2 {
		if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
			t.Fatalf("Get: %v", err)
		}
	}
	if got := c.RateLimit(resourceCore).Remaining; got != 59 {
		t.Errorf("Remaining = %d, want 59 from the first response", got)
	}
}

func TestRateLimitConcurrent(t *testing.T) {
	const n = 50
	var remaining atomic.Int32
	remaining.Store(n)
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(n))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(int(remaining.Add(-1))))
		w.Header().Set("X-RateLimit-Reset", "1790000000")
	}))

	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			if _, err := c.Get(t.Context(), "x", Conditional{}, nil); err != nil {
				t.Errorf("Get: %v", err)
			}
		})
		wg.Go(func() { _ = c.RateLimit(resourceCore) })
	}
	wg.Wait()

	if got := c.RateLimit(resourceCore); got.Limit != n || got.Remaining < 0 || got.Remaining >= n {
		t.Errorf("RateLimit = %+v, want limit %d and remaining in [0, %d)", got, n, n)
	}
}

func TestRateLimitErrors(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	reset := now.Add(10 * time.Minute)
	tests := []struct {
		name      string
		status    int
		header    http.Header
		wantReset time.Time
	}{
		{
			name:   "primary limit exhausted",
			status: http.StatusForbidden,
			header: http.Header{
				"X-Ratelimit-Limit":     {"5000"},
				"X-Ratelimit-Remaining": {"0"},
				"X-Ratelimit-Reset":     {strconv.FormatInt(reset.Unix(), 10)},
			},
			wantReset: reset.Add(minGuard),
		},
		{
			name:      "secondary limit with retry after",
			status:    http.StatusForbidden,
			header:    http.Header{"Retry-After": {"30"}},
			wantReset: now.Add(30 * time.Second),
		},
		{
			name:   "too many requests with retry after",
			status: http.StatusTooManyRequests,
			header: http.Header{
				"Retry-After":           {"5"},
				"X-Ratelimit-Limit":     {"5000"},
				"X-Ratelimit-Remaining": {"10"},
				"X-Ratelimit-Reset":     {"1790000000"},
			},
			wantReset: now.Add(5 * time.Second),
		},
		{
			name:      "too many requests without headers",
			status:    http.StatusTooManyRequests,
			wantReset: now.Add(secondaryBackoff),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClientAt(t, now, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				maps.Copy(w.Header(), tt.header)
				w.WriteHeader(tt.status)
			}))

			_, err := c.Get(t.Context(), "x", Conditional{}, nil)

			if !errors.Is(err, core.ErrRateLimited) {
				t.Fatalf("error %v is not ErrRateLimited", err)
			}
			var rl *core.RateLimitError
			if !errors.As(err, &rl) {
				t.Fatalf("error %v is not a *core.RateLimitError", err)
			}
			if !rl.Reset.Equal(tt.wantReset) {
				t.Errorf("Reset = %v, want %v", rl.Reset, tt.wantReset)
			}
			var apiErr *Error
			if !errors.As(err, &apiErr) || apiErr.StatusCode != tt.status {
				t.Errorf("error %v is not an *Error with status %d", err, tt.status)
			}
		})
	}
}

func TestForbiddenWithQuotaLeftIsNotRateLimited(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Remaining", "10")
		w.Header().Set("X-RateLimit-Reset", "1790000000")
		w.WriteHeader(http.StatusForbidden)
	}))
	_, err := c.Get(t.Context(), "x", Conditional{}, nil)
	if err == nil || errors.Is(err, core.ErrRateLimited) {
		t.Errorf("error = %v, want a non-rate-limit error", err)
	}
}
