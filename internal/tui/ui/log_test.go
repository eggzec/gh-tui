package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// logBuffer is a buffer that commands may log into while the test reads it.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) records(t *testing.T) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for line := range strings.Lines(b.buf.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q is not JSON: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

// captureLog logs at debug level into the returned buffer, and counts into
// the returned stats, for the rest of the test.
func captureLog(t *testing.T) (*logBuffer, *obs.Stats) {
	t.Helper()
	buf := new(logBuffer)
	prev := slog.Default()
	slog.SetDefault(obs.NewLogger(buf, slog.LevelDebug, "s_test"))
	stats := obs.NewStats()
	prevStats := obs.SetDefault(stats)
	t.Cleanup(func() {
		slog.SetDefault(prev)
		obs.SetDefault(prevStats)
	})
	return buf, stats
}

func TestDoTracesOp(t *testing.T) {
	buf, _ := captureLog(t)
	var traces []string
	op := func(err error) Op {
		return opFunc(func(ctx context.Context) error {
			id, _ := obs.TraceID(ctx)
			traces = append(traces, id)
			return err
		})
	}
	Do(t.Context(), PullsTitle, op(nil), "merge #1")()
	Do(t.Context(), PullsTitle, op(errors.New("conflict")), "merge #2")()

	recs := buf.records(t)
	want := []struct{ level, msg, what string }{
		{"INFO", "op sent", "merge #1"}, {"INFO", "op confirmed", "merge #1"},
		{"INFO", "op sent", "merge #2"}, {"WARN", "op rolled back", "merge #2"},
	}
	if len(recs) != len(want) {
		t.Fatalf("got %d records, want %d", len(recs), len(want))
	}
	for i, w := range want {
		r := recs[i]
		if r["level"] != w.level || r["msg"] != w.msg || r["what"] != w.what || r["trace"] != "op" || r["trace_id"] != traces[i/2] {
			t.Errorf("record %d = %v, want %s %s of %s in trace %s", i, r, w.level, w.msg, w.what, traces[i/2])
		}
	}
	if traces[0] == "" || traces[0] == traces[1] {
		t.Errorf("traces = %v, want one per op", traces)
	}
	if recs[3]["err"] != "conflict" {
		t.Errorf("rollback has err %v, want conflict", recs[3]["err"])
	}
}

func TestFeedPagesTracesReads(t *testing.T) {
	buf, _ := captureLog(t)
	var got string
	fetch := FeedPages("list.pulls", func(cursor string) string { return cursor }, func(ctx context.Context, _ string, _ bool) (core.Page[int], error) {
		got, _ = obs.TraceID(ctx)
		return core.Page[int]{Items: []int{1, 2}}, nil
	})
	if _, _, err := fetch(t.Context(), ""); err != nil {
		t.Fatal(err)
	}
	recs := buf.records(t)
	if got == "" || len(recs) != 1 || recs[0]["trace_id"] != got || recs[0]["trace"] != "list.pulls" || recs[0]["items"] != 2.0 {
		t.Errorf("records = %v, want the end of trace %s", recs, got)
	}
}

func TestAheadCountsUse(t *testing.T) {
	buf, stats := captureLog(t)
	r := newReader()
	r.cached[2] = true
	a := NewAhead("pull", r.readRow, r.current, 3, time.Millisecond)
	a.Reset(t.Context())
	run(a.First(rowsOf(30)))
	a.Opened(1)
	a.Opened(2) // cached before, not read ahead
	a.Opened(1) // counts once

	got := stats.Summary().Prefetch
	want := obs.PrefetchStats{Kind: "pull", Sent: 2, Cached: 1, Read: 2, Opened: 1, Useful: 0.5}
	if len(got) != 1 || got[0] != want {
		t.Errorf("prefetch = %+v, want %+v", got, want)
	}
	var decision map[string]any
	for _, rec := range buf.records(t) {
		if rec["msg"] == "prefetch" {
			decision = rec
		}
	}
	if decision["trigger"] != "rows" || decision["sent"] != 2.0 || decision["skipped_cached"] != 1.0 || decision["kind"] != "pull" {
		t.Errorf("decision = %v", decision)
	}
}

func TestAheadCountsRateLimit(t *testing.T) {
	_, stats := captureLog(t)
	r := newReader()
	r.err = &core.RateLimitError{Reset: time.Now().Add(time.Hour)}
	a := NewAhead("issue", r.readRow, r.current, 5, time.Millisecond)
	a.Reset(t.Context())
	run(a.First(rowsOf(30)))
	// Hovering while limited reads nothing.
	a.Moved(9, true)

	p := stats.Summary().Prefetch[0]
	if p.RateLimited == 0 || p.Limited == 0 || p.Read != 0 || p.Sent != p.RateLimited {
		t.Errorf("prefetch = %+v, want rate-limited reads, then skips", p)
	}
}
