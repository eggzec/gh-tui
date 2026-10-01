package revalidate

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

// logBuffer is a buffer that the revalidator's goroutines may log into
// while the test reads it.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) records(t *testing.T, msg string) []map[string]any {
	t.Helper()
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []map[string]any
	for line := range strings.Lines(b.buf.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q is not JSON: %v", line, err)
		}
		if m["msg"] == msg {
			out = append(out, m)
		}
	}
	return out
}

func TestPassLog(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var buf logBuffer
		prev := slog.Default()
		slog.SetDefault(obs.NewLogger(&buf, slog.LevelDebug, "s_test"))
		t.Cleanup(func() { slog.SetDefault(prev) })
		stats := obs.NewStats()
		prevStats := obs.SetDefault(stats)
		t.Cleanup(func() { obs.SetDefault(prevStats) })

		var (
			mu     sync.Mutex
			traces []string
		)
		check := func(res Result) func(ctx context.Context) Result {
			return func(ctx context.Context) Result {
				id, _ := obs.TraceID(ctx)
				mu.Lock()
				traces = append(traces, id)
				mu.Unlock()
				return res
			}
		}
		entries := []Entry{
			{ID: "issues:octo/a", Repo: repoA, UsedAt: time.Now(), Check: check(Result{Status: NotModified})},
			{ID: "notifications", UsedAt: time.Now(), Check: check(Result{Status: Changed, Sync: "notifications"})},
		}
		start(t, entries, new(recorder), Settings{Interval: time.Minute, PerMinute: 30, Recent: settings.Recent})
		synctest.Sleep(time.Second)

		passes := buf.records(t, "revalidate pass")
		if len(passes) != 1 {
			t.Fatalf("got %d pass records, want 1", len(passes))
		}
		p := passes[0]
		for k, want := range map[string]any{
			"level": "INFO", "trace": "revalidate.pass", "listed": 2.0, "due": 2.0, "budget": 30.0,
			"sent": 2.0, "not_modified": 1.0, "changed": 1.0, "published": 1.0, "next_in_s": 60.0,
		} {
			if p[k] != want {
				t.Errorf("pass %s = %v, want %v", k, p[k], want)
			}
		}
		checks := buf.records(t, "revalidate check")
		if len(checks) != 2 {
			t.Fatalf("got %d check records, want 2", len(checks))
		}
		for _, c := range checks {
			if c["trace_id"] != p["trace_id"] {
				t.Errorf("check %v is outside the pass's trace %v", c, p["trace_id"])
			}
		}
		for _, id := range traces {
			if id != p["trace_id"] {
				t.Errorf("a check ran in trace %q, want the pass's %v", id, p["trace_id"])
			}
		}
		if r := stats.Summary().Revalidate; r.Passes != 1 || r.Sent != 2 || r.Budget != 30 {
			t.Errorf("summary = %+v", r)
		}
	})
}

func TestPassLogWarnsWhenOffline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var buf logBuffer
		prev := slog.Default()
		slog.SetDefault(obs.NewLogger(&buf, slog.LevelInfo, "s_test"))
		t.Cleanup(func() { slog.SetDefault(prev) })

		srv := newServer(func(string, int) Result { return Result{Status: Offline} })
		start(t, []Entry{srv.entry("a", repoA, 0)}, new(recorder), everyMinute)
		synctest.Sleep(time.Second)

		passes := buf.records(t, "revalidate pass")
		if len(passes) != 1 || passes[0]["level"] != "WARN" || passes[0]["offline"] != true || passes[0]["next_in_s"] != 120.0 {
			t.Errorf("passes = %v, want one offline at warn, backing off to 2m", passes)
		}
		if n := len(buf.records(t, "revalidate check")); n != 0 {
			t.Errorf("got %d check records at info, want none", n)
		}
	})
}
