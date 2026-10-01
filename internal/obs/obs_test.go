package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"
)

// capture makes the default logger write JSON lines at level into the
// returned buffer for the rest of the test.
func capture(t *testing.T, level slog.Level) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(NewLogger(&buf, level, "s_test"))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

// records decodes the JSON lines in buf.
func records(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.Lines(buf.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q is not JSON: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestNewID(t *testing.T) {
	re := regexp.MustCompile(`^t_[0-9a-f]{10}$`)
	seen := map[string]bool{}
	for range 1000 {
		id := NewID(TracePrefix)
		if !re.MatchString(id) {
			t.Fatalf("NewID = %q, want t_ and 10 hex digits", id)
		}
		if seen[id] {
			t.Fatalf("NewID repeated %q", id)
		}
		seen[id] = true
	}
}

func TestHandlerAddsIDs(t *testing.T) {
	buf := capture(t, slog.LevelInfo)
	ctx := WithTrace(context.Background(), "open.pull")
	id, name := TraceID(ctx)
	slog.InfoContext(ctx, "hello", "n", 1)
	slog.Info("untraced")
	From(ctx).Info("via from")

	recs := records(t, buf)
	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3:\n%s", len(recs), buf)
	}
	for _, r := range recs {
		if r["session_id"] != "s_test" {
			t.Errorf("record %v has session_id %v, want s_test", r["msg"], r["session_id"])
		}
	}
	for _, r := range []map[string]any{recs[0], recs[2]} {
		if r["trace_id"] != id || r["trace"] != name {
			t.Errorf("record %v has trace %v %v, want %s %s", r["msg"], r["trace_id"], r["trace"], id, name)
		}
	}
	if recs[0]["n"] != 1.0 {
		t.Errorf("record lost its attribute: %v", recs[0])
	}
	if _, ok := recs[1]["trace_id"]; ok {
		t.Errorf("untraced record has a trace_id: %v", recs[1])
	}
	// The ids come before the record's own attributes, so they line up.
	if line, _, _ := strings.Cut(buf.String(), "\n"); strings.Index(line, `"trace_id"`) > strings.Index(line, `"n"`) {
		t.Errorf("trace_id comes after the attributes: %s", line)
	}
}

func TestWithTraceNests(t *testing.T) {
	outer := WithTrace(context.Background(), "list.pulls")
	inner := WithTrace(outer, "prefetch.pull")
	a, _ := TraceID(outer)
	b, name := TraceID(inner)
	if a == b || name != "prefetch.pull" {
		t.Errorf("inner trace = %s %s, want a new id named prefetch.pull", b, name)
	}
	if id, name := TraceID(context.Background()); id != "" || name != "" {
		t.Errorf("TraceID outside a trace = %q %q, want empty", id, name)
	}
}

func TestEnd(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		cancel    bool
		wantLevel string
		wantMsg   string
	}{
		{name: "ok", wantLevel: "DEBUG", wantMsg: "done"},
		{name: "failed", err: errors.New("boom"), wantLevel: "ERROR", wantMsg: "failed"},
		{name: "canceled", err: context.Canceled, wantLevel: "DEBUG", wantMsg: "canceled"},
		{name: "context done", err: errors.New("read: closed"), cancel: true, wantLevel: "DEBUG", wantMsg: "canceled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := capture(t, slog.LevelDebug)
			parent, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx, end := Begin(parent, "job")
			if tt.cancel {
				cancel()
			}
			end(tt.err, "what", "x")
			recs := records(t, buf)
			if len(recs) != 1 {
				t.Fatalf("got %d records, want 1", len(recs))
			}
			r := recs[0]
			id, _ := TraceID(ctx)
			if r["level"] != tt.wantLevel || r["msg"] != tt.wantMsg || r["trace_id"] != id || r["what"] != "x" {
				t.Errorf("record = %v, want %s %s in trace %s", r, tt.wantLevel, tt.wantMsg, id)
			}
			if _, ok := r["duration_ms"].(float64); !ok {
				t.Errorf("record has no duration_ms: %v", r)
			}
			if tt.err != nil && r["err"] != tt.err.Error() {
				t.Errorf("err = %v, want %v", r["err"], tt.err)
			}
		})
	}
}

func TestEndBelowLevel(t *testing.T) {
	buf := capture(t, slog.LevelInfo)
	_, end := Begin(context.Background(), "job")
	end(nil)
	if buf.Len() != 0 {
		t.Errorf("a trace that ended well logged at info: %s", buf)
	}
}

func BenchmarkDisabledLog(b *testing.B) {
	prev := slog.Default()
	slog.SetDefault(NewLogger(io.Discard, slog.LevelInfo, "s_bench"))
	b.Cleanup(func() { slog.SetDefault(prev) })
	ctx := WithTrace(context.Background(), "bench")
	b.ReportAllocs()
	for b.Loop() {
		if Enabled(ctx, slog.LevelDebug) {
			slog.DebugContext(ctx, "cache", "kind", "pulls", "layer", "memory")
		}
	}
}

func BenchmarkLog(b *testing.B) {
	prev := slog.Default()
	slog.SetDefault(NewLogger(io.Discard, slog.LevelInfo, "s_bench"))
	b.Cleanup(func() { slog.SetDefault(prev) })
	ctx := WithTrace(context.Background(), "bench")
	b.ReportAllocs()
	for b.Loop() {
		slog.InfoContext(ctx, "http", "method", "GET", "status", 200, "duration_ms", 12.5)
	}
}

// EndWith logs through the logger it is given, not the default one.
func TestEndWith(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	var def, own bytes.Buffer
	slog.SetDefault(NewLogger(&def, slog.LevelDebug, "s_default"))
	l := NewLogger(&own, slog.LevelDebug, "s_own")
	ctx := WithTrace(context.Background(), "test.end")
	EndWith(ctx, l, time.Now(), nil, "span", "test")
	if !strings.Contains(own.String(), `"msg":"done"`) || !strings.Contains(own.String(), `"trace":"test.end"`) {
		t.Errorf("own logger got %q, want the end record in its trace", own.String())
	}
	if def.Len() != 0 {
		t.Errorf("default logger got %q, want nothing", def.String())
	}
}
