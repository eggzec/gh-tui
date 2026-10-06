package ui

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// pages is a fake read of first pages, by filter, that records the pages
// read and can hold them until released or fail them.
type pages struct {
	mu     sync.Mutex
	cached map[string]bool
	read   []string
	// hold, if set, holds each read until it is closed or the read is
	// cancelled.
	hold chan struct{}
	errs map[string]error
	// kept are the pages served kept, in place of a read GitHub rate
	// limited, as the services do: with no error, telling the watch of
	// the read.
	kept map[string]bool
}

func newPages() *pages {
	return &pages{cached: map[string]bool{}, errs: map[string]error{}, kept: map[string]bool{}}
}

func (p *pages) readPage(ctx context.Context, q string) error {
	p.mu.Lock()
	p.read = append(p.read, q)
	hold, err, kept := p.hold, p.errs[q], p.kept[q]
	p.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err != nil {
		return err
	}
	if kept {
		core.ServedLimited(ctx)
		return nil
	}
	p.mu.Lock()
	p.cached[q] = true
	p.mu.Unlock()
	return nil
}

func (p *pages) fresh(q string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cached[q]
}

func (p *pages) reads() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.read)
}

func newFilters(p *pages) *Filters[string] {
	return NewFilters("list_filter", p.readPage, p.fresh, func(q string) string { return q })
}

// decisions returns the decision, and the outcome of those sent, of each
// prefetch filter record in buf, in order.
func decisions(t *testing.T, buf *logBuffer) []string {
	t.Helper()
	var out []string
	for _, r := range buf.records(t) {
		if r["msg"] != "prefetch filter" {
			continue
		}
		if r["trace"] != "prefetch.filters" {
			t.Errorf("record %v is not in the prefetch.filters trace", r)
		}
		d := r["filter"].(string) + ":" + r["decision"].(string)
		if o, ok := r["outcome"]; ok {
			d += ":" + o.(string)
		}
		out = append(out, d)
	}
	return out
}

func TestFiltersReadsEachOnce(t *testing.T) {
	buf, stats := captureLog(t)
	p := newPages()
	p.cached["merged"] = true
	f := newFilters(p)
	f.Reset(t.Context(), "r")
	f.Arm()

	run(f.Read(func() []string { return []string{"closed", "merged", "all"} }))
	if got, want := p.reads(), []string{"closed", "all"}; !slices.Equal(got, want) {
		t.Errorf("read %v, want %v: the cached one skipped", got, want)
	}
	if cmd := f.Read(func() []string { return []string{"closed", "merged", "all"} }); cmd != nil {
		t.Error("Read read ahead again before a Reset")
	}
	want := []string{"closed:sent:read", "merged:skipped_cached", "all:sent:read"}
	if got := decisions(t, buf); !slices.Equal(got, want) {
		t.Errorf("logged %v, want %v", got, want)
	}

	f.Opened("closed")
	f.Opened("merged")
	f.Opened("closed")
	got := stats.Summary().Prefetch
	if len(got) != 1 {
		t.Fatalf("summary has %d prefetch kinds, want 1", len(got))
	}
	if s := got[0]; s.Kind != "list_filter" || s.Sent != 2 || s.Cached != 1 || s.Read != 2 || s.Opened != 1 {
		t.Errorf("summary = %+v, want 2 sent, 1 cached, 2 read, 1 opened", s)
	}

	// Another repository reads ahead again once armed.
	f.Reset(t.Context(), "other")
	f.Arm()
	run(f.Read(func() []string { return []string{"open"} }))
	if got, want := p.reads(), []string{"closed", "all", "open"}; !slices.Equal(got, want) {
		t.Errorf("read %v after Reset, want %v", got, want)
	}
}

func TestFiltersWaitForArm(t *testing.T) {
	others := func() []string { return []string{"closed"} }
	tests := []struct {
		name string
		// steps runs on a Filters reset for repository a, and returns
		// what the last Read read.
		steps func(ctx context.Context, f *Filters[string]) tea.Cmd
		want  bool
	}{{
		name:  "not armed",
		steps: func(_ context.Context, f *Filters[string]) tea.Cmd { return f.Read(others) },
	}, {
		name: "armed",
		steps: func(_ context.Context, f *Filters[string]) tea.Cmd {
			f.Arm()
			return f.Read(others)
		},
		want: true,
	}, {
		name: "armed in another repository",
		steps: func(ctx context.Context, f *Filters[string]) tea.Cmd {
			f.Reset(ctx, "b")
			f.Arm()
			f.Reset(ctx, "a")
			return f.Read(others)
		},
	}, {
		name: "armed before, back again",
		steps: func(ctx context.Context, f *Filters[string]) tea.Cmd {
			f.Arm()
			f.Reset(ctx, "b")
			f.Reset(ctx, "a")
			return f.Read(others)
		},
		want: true,
	}, {
		name: "read before, back again",
		steps: func(ctx context.Context, f *Filters[string]) tea.Cmd {
			f.Arm()
			run(f.Read(others))
			f.Reset(ctx, "b")
			f.Reset(ctx, "a")
			f.Arm()
			return f.Read(others)
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFilters(newPages())
			f.Reset(t.Context(), "a")
			if got := tt.steps(t.Context(), f) != nil; got != tt.want {
				t.Errorf("read ahead = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFiltersRetryAfterCancel(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPages()
		p.hold = make(chan struct{})
		f := newFilters(p)
		f.Reset(t.Context(), "a")
		f.Arm()
		others := func() []string { return []string{"closed", "merged"} }
		done := make(chan struct{})
		go func() {
			run(f.Read(others))
			close(done)
		}()
		synctest.Wait()
		if cmd := f.Read(others); cmd != nil {
			t.Error("Read read ahead again while the pages were being read")
		}
		// Another repository cancels the reads, and coming back reads them
		// again.
		f.Reset(t.Context(), "b")
		<-done
		close(p.hold)
		f.Reset(t.Context(), "a")
		cmd := f.Read(others)
		if cmd == nil {
			t.Fatal("Read didn't read again the pages a Reset cancelled")
		}
		run(cmd)
		if cmd := f.Read(others); cmd != nil {
			t.Error("Read read ahead again once the pages were read")
		}
	})
}

func TestFiltersReadsOneAtATime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := newPages()
		p.hold = make(chan struct{})
		f := newFilters(p)
		f.Reset(t.Context(), "r")
		f.Arm()
		done := make(chan struct{})
		go func() {
			run(f.Read(func() []string { return []string{"closed", "merged"} }))
			close(done)
		}()
		synctest.Wait()
		if got := p.reads(); !slices.Equal(got, []string{"closed"}) {
			t.Errorf("reading %v at once, want only the first", got)
		}
		close(p.hold)
		<-done
		if got := p.reads(); !slices.Equal(got, []string{"closed", "merged"}) {
			t.Errorf("read %v, want both in order", got)
		}
	})
}

func TestFiltersResetCancels(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf, stats := captureLog(t)
		p := newPages()
		p.hold = make(chan struct{})
		f := newFilters(p)
		f.Reset(t.Context(), "r")
		f.Arm()
		done := make(chan struct{})
		go func() {
			run(f.Read(func() []string { return []string{"closed", "merged"} }))
			close(done)
		}()
		synctest.Wait()
		f.Reset(t.Context(), "other")
		<-done
		if got := p.reads(); !slices.Equal(got, []string{"closed"}) {
			t.Errorf("read %v, want only the one before the reset", got)
		}
		want := []string{"closed:sent:canceled", "merged:canceled"}
		if got := decisions(t, buf); !slices.Equal(got, want) {
			t.Errorf("logged %v, want %v", got, want)
		}
		if s := stats.Summary().Prefetch[0]; s.Sent != 1 || s.Canceled != 2 || s.Read != 0 {
			t.Errorf("summary = %+v, want 1 sent, 2 canceled", s)
		}
	})
}

func TestFiltersStopAtRateLimit(t *testing.T) {
	buf, stats := captureLog(t)
	p := newPages()
	p.errs["closed"] = &core.RateLimitError{Reset: time.Now()}
	f := newFilters(p)
	f.Reset(t.Context(), "r")
	f.Arm()
	run(f.Read(func() []string { return []string{"closed", "merged", "all"} }))
	if got := p.reads(); !slices.Equal(got, []string{"closed"}) {
		t.Errorf("read %v, want nothing after the rate limit", got)
	}
	want := []string{"closed:sent:rate_limited", "merged:skipped_rate_limited", "all:skipped_rate_limited"}
	if got := decisions(t, buf); !slices.Equal(got, want) {
		t.Errorf("logged %v, want %v", got, want)
	}
	if s := stats.Summary().Prefetch[0]; s.RateLimited != 1 || s.Limited != 2 {
		t.Errorf("summary = %+v, want 1 rate limited, 2 skipped for the limit", s)
	}
	if cmd := f.Read(func() []string { return []string{"merged"} }); cmd != nil {
		t.Error("Read read ahead again before a Reset")
	}
}

// TestFiltersStopAtKeptForLimit checks that a page served kept, in place
// of a read GitHub rate limited, stops the rest as the rate limit's error
// does, and doesn't count as read.
func TestFiltersStopAtKeptForLimit(t *testing.T) {
	buf, stats := captureLog(t)
	p := newPages()
	p.kept["closed"] = true
	f := newFilters(p)
	f.Reset(t.Context(), "r")
	f.Arm()
	run(f.Read(func() []string { return []string{"closed", "merged", "all"} }))
	if got := p.reads(); !slices.Equal(got, []string{"closed"}) {
		t.Errorf("read %v, want nothing after the rate limit", got)
	}
	want := []string{"closed:sent:rate_limited", "merged:skipped_rate_limited", "all:skipped_rate_limited"}
	if got := decisions(t, buf); !slices.Equal(got, want) {
		t.Errorf("logged %v, want %v", got, want)
	}
	if s := stats.Summary().Prefetch[0]; s.RateLimited != 1 || s.Limited != 2 || s.Read != 0 {
		t.Errorf("summary = %+v, want 1 rate limited, 2 skipped for the limit, none read", s)
	}
}

func TestFiltersStopOnBudget(t *testing.T) {
	buf, stats := captureLog(t)
	obs.SetPrefetchBudget(10)
	t.Cleanup(func() { obs.SetPrefetchBudget(0) })
	p := newPages()
	read := func(ctx context.Context, q string) error {
		obs.ChargeGraphQL(ctx, 500)
		return p.readPage(ctx, q)
	}
	f := NewFilters("list_filter", read, p.fresh, func(q string) string { return q })
	f.Reset(t.Context(), "r")
	f.Arm()
	run(f.Read(func() []string { return []string{"closed", "merged"} }))
	if got := p.reads(); !slices.Equal(got, []string{"closed"}) {
		t.Errorf("read %v, want nothing past the budget", got)
	}
	want := []string{"closed:sent:read", "merged:skipped_budget"}
	if got := decisions(t, buf); !slices.Equal(got, want) {
		t.Errorf("logged %v, want %v", got, want)
	}
	if s := stats.Summary().Prefetch[0]; s.OverBudget != 1 {
		t.Errorf("summary = %+v, want 1 skipped for the budget", s)
	}
}

func TestFiltersNil(t *testing.T) {
	var f *Filters[string]
	f.Reset(t.Context(), "r")
	f.Arm()
	f.Opened("closed")
	if cmd := f.Read(func() []string { return []string{"closed"} }); cmd != nil {
		t.Error("a nil Filters read ahead")
	}
}
