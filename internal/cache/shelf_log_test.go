package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
)

// captureLog makes the default logger write JSON lines at info level
// into the returned buffer, and counts into fresh stats, for the rest of
// the test.
func captureLog(t *testing.T) (*bytes.Buffer, *obs.Stats) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(obs.NewLogger(&buf, slog.LevelInfo, "s_test"))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf, freshStats(t)
}

// logged returns the records in buf with the message msg.
func logged(t *testing.T, buf *bytes.Buffer, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.Lines(buf.String()) {
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

func dropped(s *obs.Stats, kind string) int64 {
	for _, d := range s.Summary().Disk {
		if d.Kind == kind {
			return d.Dropped
		}
	}
	return 0
}

// An entry dropped since it no longer matches its shelf is counted, and
// logged with why.
func TestShelfLogsMismatch(t *testing.T) {
	tests := []struct {
		name   string
		damage func([]byte) []byte
		schema int
		reason string
		level  string
	}{
		{name: "corrupt", damage: func(d []byte) []byte { return d[:len(d)/2] }, schema: 1, reason: "decode", level: "WARN"},
		{name: "other format", damage: func(d []byte) []byte {
			return bytes.Replace(d, []byte(`"format":1`), []byte(`"format":0`), 1)
		}, schema: 1, reason: "format", level: "INFO"},
		{name: "other key", damage: func(d []byte) []byte {
			return bytes.Replace(d, []byte(`"key":"k"`), []byte(`"key":"j"`), 1)
		}, schema: 1, reason: "key", level: "WARN"},
		{name: "other schema", schema: 2, reason: "schema", level: "INFO"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, stats := captureLog(t)
			store := newMemStore()
			_ = NewShelf[page](store, "page", 1).Save("k", Entry[page]{Value: page{Items: []string{"a"}}})
			if tt.damage != nil {
				_, data := store.only(t)
				_ = store.Put("page", objectName("k"), tt.damage(data))
			}
			if _, ok := NewShelf[page](store, "page", tt.schema).Load("k"); ok {
				t.Fatal("Load hit")
			}
			recs := logged(t, buf, "kept dropped")
			if len(recs) != 1 {
				t.Fatalf("%d records, want 1:\n%s", len(recs), buf)
			}
			r := recs[0]
			if r["level"] != tt.level || r["span"] != "cache.disk" || r["kind"] != "page" || r["reason"] != tt.reason || r["entry"] != nil {
				t.Errorf("record = %v", r)
			}
			if got := dropped(stats, "page"); got != 1 {
				t.Errorf("dropped = %d, want 1", got)
			}
		})
	}
}

// A schema bump drops every entry of a shelf, and logs once for them all,
// and then once every shelfLogEvery, with how many it held back.
func TestShelfLogsMismatchThrottled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf, stats := captureLog(t)
		store := newMemStore()
		old := NewShelf[page](store, "page", 1)
		for i := range 51 {
			_ = old.Save(strconv.Itoa(i), Entry[page]{})
		}
		s := NewShelf[page](store, "page", 2)
		for i := range 50 {
			s.Load(strconv.Itoa(i))
		}
		if recs := logged(t, buf, "kept dropped"); len(recs) != 1 {
			t.Errorf("%d records for 50 entries dropped, want 1", len(recs))
		}
		if got := dropped(stats, "page"); got != 50 {
			t.Errorf("dropped = %d, want 50", got)
		}
		time.Sleep(shelfLogEvery)
		buf.Reset()
		s.Load("50")
		if recs := logged(t, buf, "kept dropped"); len(recs) != 1 || recs[0]["suppressed"] != float64(49) {
			t.Errorf("records after %v = %v, want 1 that held back 49", shelfLogEvery, recs)
		}
	})
}

// Drop logs an entry it drops, by its key without the query, which may
// hold what the user typed, and logs nothing when nothing is kept.
func TestShelfDropLogs(t *testing.T) {
	buf, stats := captureLog(t)
	store := newMemStore()
	s := NewShelf[page](store, "pulls", 1)
	const key = "pulls:o/r?filter=author%3Asecret"
	_ = s.Save(key, Entry[page]{})
	ctx := obs.WithTrace(t.Context(), "list.pulls")
	s.Drop(ctx, key, "not found")
	s.Drop(ctx, key, "not found")
	if _, ok := s.Load(key); ok {
		t.Error("Load after Drop hit")
	}
	recs := logged(t, buf, "kept dropped")
	if len(recs) != 1 {
		t.Fatalf("%d records, want 1 for the entry kept:\n%s", len(recs), buf)
	}
	id, _ := obs.TraceID(ctx)
	r := recs[0]
	if r["level"] != "INFO" || r["kind"] != "pulls" || r["reason"] != "not found" || r["entry"] != "pulls:o/r" || r["trace_id"] != id {
		t.Errorf("record = %v", r)
	}
	if strings.Contains(buf.String(), "secret") {
		t.Errorf("the log holds the query:\n%s", buf)
	}
	if got := dropped(stats, "pulls"); got != 1 {
		t.Errorf("dropped = %d, want 1", got)
	}
	var nilShelf *Shelf[page]
	nilShelf.Drop(ctx, key, "not found")
}

// A value that can't be encoded fails its write, which is counted and
// logged, since callers go on without it.
func TestShelfLogsEncodeFailure(t *testing.T) {
	buf, stats := captureLog(t)
	s := NewShelf[float64](newMemStore(), "num", 1)
	if err := s.Save("k", Entry[float64]{Value: math.NaN()}); err == nil {
		t.Fatal("Save of NaN succeeded")
	}
	recs := logged(t, buf, "disk cache write failed")
	if len(recs) != 1 || recs[0]["level"] != "WARN" || recs[0]["kind"] != "num" || recs[0]["op"] != "encode" {
		t.Errorf("records = %v, want 1 for the encoding", recs)
	}
	for _, d := range stats.Summary().Disk {
		if d.Kind == "num" && d.WriteFailed != 1 {
			t.Errorf("write_failed = %d, want 1", d.WriteFailed)
		}
	}
}

// The debug records of reads name a key without its query, which may
// hold what the user typed, such as a search.
func TestLogsKeyWithoutQuery(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(obs.NewLogger(&buf, slog.LevelDebug, "s_test"))
	t.Cleanup(func() { slog.SetDefault(prev) })
	freshStats(t)
	const key = "search?kind=issues&q=secret+plans"
	c := New[page]()
	fetch := func(context.Context, Entry[page], bool) (Entry[page], error) { return Entry[page]{}, nil }
	for range 2 {
		if _, err := c.Fetch(t.Context(), key, fetch); err != nil {
			t.Fatal(err)
		}
	}
	store := newMemStore()
	_ = NewShelf[page](store, "search", 1).Save(key, Entry[page]{})
	NewShelf[page](store, "search", 1).Warm(New[page](WithTTL(time.Nanosecond)), key, false)
	recs := logged(t, &buf, "cache")
	if len(recs) == 0 {
		t.Fatalf("no cache records:\n%s", &buf)
	}
	for _, r := range recs {
		if r["key"] != "search" {
			t.Errorf("record = %v, want the key search", r)
		}
	}
	if strings.Contains(buf.String(), "secret") {
		t.Errorf("the log holds the query:\n%s", &buf)
	}
}
