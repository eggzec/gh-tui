package github

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"
)

// hang is a transport that answers once the request is done, with its
// error, or at once with a stalling body if body is set.
type hang struct {
	body bool
	// ctx is the context of the last request.
	ctx *context.Context
}

func (h hang) RoundTrip(req *http.Request) (*http.Response, error) {
	*h.ctx = req.Context()
	if h.body {
		return stall{}.RoundTrip(req)
	}
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestTimeoutPerAttempt(t *testing.T) {
	tests := []struct {
		name string
		body bool
		// cancelAfter cancels the caller's context then, if set.
		cancelAfter time.Duration
		wantTimeout bool
	}{
		{name: "headers", wantTimeout: true},
		{name: "body", body: true, wantTimeout: true},
		{name: "caller canceled", cancelAfter: time.Second},
		{name: "caller canceled while body is read", body: true, cancelAfter: time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				var sent context.Context
				tr := &timeoutTransport{base: hang{body: tt.body, ctx: &sent}, timeout: 5 * time.Second}
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				if tt.cancelAfter > 0 {
					time.AfterFunc(tt.cancelAfter, cancel)
				}
				start := time.Now()
				req := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/x", http.NoBody)
				resp, err := tr.RoundTrip(req)
				if err == nil {
					_, err = io.ReadAll(resp.Body)
					_ = resp.Body.Close()
				}
				want := 5 * time.Second
				if tt.cancelAfter > 0 {
					want = tt.cancelAfter
				}
				if d := time.Since(start); d != want {
					t.Errorf("attempt ended after %v, want %v", d, want)
				}
				_, timeout := errors.AsType[*timeoutError](err)
				if timeout != tt.wantTimeout || !errors.Is(err, context.DeadlineExceeded) && tt.wantTimeout {
					t.Errorf("err = %v, want a timeout %v", err, tt.wantTimeout)
				}
				if sent.Err() == nil {
					t.Error("the attempt's context is live after its end")
				}
			})
		})
	}
}

// Each attempt gets the whole timeout, however long those before it took.
func TestTimeoutEachAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var sent context.Context
		tr := &timeoutTransport{base: hang{ctx: &sent}, timeout: 5 * time.Second}
		for range 3 {
			start := time.Now()
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://api.github.com/x", http.NoBody)
			resp, err := tr.RoundTrip(req)
			if err == nil {
				_ = resp.Body.Close()
				t.Fatal("RoundTrip succeeded, want a timeout")
			}
			if d := time.Since(start); d != 5*time.Second {
				t.Errorf("attempt ended after %v, want 5s", d)
			}
		}
	})
}

// Closing a body ends its attempt, so its deadline's timer goes at once.
func TestTimeoutEndsWithBody(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var sent context.Context
		tr := &timeoutTransport{base: bodiesWithContext{&sent}, timeout: time.Hour}
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "https://api.github.com/x", http.NoBody)
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatal(err)
		}
		if sent.Err() != nil {
			t.Fatal("the attempt ended before its body was closed")
		}
		_ = resp.Body.Close()
		if sent.Err() == nil {
			t.Error("the attempt's context is live after its body was closed")
		}
	})
}

// bodiesWithContext answers with an empty body, and keeps the context of
// the request.
type bodiesWithContext struct{ ctx *context.Context }

func (b bodiesWithContext) RoundTrip(req *http.Request) (*http.Response, error) {
	*b.ctx = req.Context()
	return bodies{}.RoundTrip(req)
}
