package watch

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
)

// logBuffer is a buffer that the pollers may log into while the test reads
// it.
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

func TestPollLog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var buf logBuffer
		prev := slog.Default()
		slog.SetDefault(obs.NewLogger(&buf, slog.LevelDebug, "s_test"))
		t.Cleanup(func() { slog.SetDefault(prev) })

		var (
			mu     sync.Mutex
			traces []string
		)
		results := []error{nil, errPoll, nil}
		n := 0
		e := New(WithInterval(10*time.Second), WithMaxBackoff(time.Minute))
		e.Subscribe("pulls:octo/a", func(ctx context.Context) (Result, error) {
			id, _ := obs.TraceID(ctx)
			mu.Lock()
			defer mu.Unlock()
			traces = append(traces, id)
			err := results[min(n, len(results)-1)]
			n++
			return Result{Changed: n == 3}, err
		})
		run(t, e)
		synctest.Sleep(45 * time.Second)

		recs := buf.records(t)
		if len(recs) != 3 {
			t.Fatalf("got %d records, want 3 polls", len(recs))
		}
		want := []struct {
			level   string
			changed bool
			next    float64
		}{{"DEBUG", false, 10}, {"WARN", false, 20}, {"INFO", true, 10}}
		for i, w := range want {
			r := recs[i]
			if r["msg"] != "sync poll" || r["level"] != w.level || r["changed"] != w.changed || r["next_in_s"] != w.next || r["key"] != "pulls:octo/a" {
				t.Errorf("poll %d = %v, want %s changed=%v next %vs", i, r, w.level, w.changed, w.next)
			}
			if r["trace_id"] != traces[i] || traces[i] == "" {
				t.Errorf("poll %d logged in trace %v, polled in %q", i, r["trace_id"], traces[i])
			}
		}
		if recs[1]["err"] != errPoll.Error() || recs[1]["failures"] != 1.0 {
			t.Errorf("failed poll = %v", recs[1])
		}
		if traces[0] == traces[1] {
			t.Error("two polls share a trace")
		}
	})
}
