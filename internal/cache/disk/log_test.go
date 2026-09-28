package disk

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/obs"
)

// capture makes the default logger write JSON lines at info level into
// the returned buffer, and counts into fresh stats, for the rest of the
// test.
func capture(t *testing.T) (*bytes.Buffer, *obs.Stats) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(obs.NewLogger(&buf, slog.LevelInfo, "s_test"))
	stats := obs.NewStats()
	prevStats := obs.SetDefault(stats)
	t.Cleanup(func() {
		slog.SetDefault(prev)
		obs.SetDefault(prevStats)
	})
	return &buf, stats
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

// diskSummary returns what stats count of kind on disk.
func diskSummary(stats *obs.Stats, kind string) obs.DiskSummary {
	for _, d := range stats.Summary().Disk {
		if d.Kind == kind {
			return d
		}
	}
	return obs.DiskSummary{}
}

// A write that fails, as every one does on a full disk, is counted each
// time and logged at warn level once per kind and operation, and again
// once logEvery has passed, with how many it held back.
func TestWriteFailureLogged(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		buf, stats := capture(t)
		s := open(t)
		// A file where the directory of the kind goes fails every write.
		if err := os.WriteFile(filepath.Join(s.Dir(), "blob"), nil, filePerm); err != nil {
			t.Fatal(err)
		}
		for range 20 {
			if err := s.Put("blob", key, text); err == nil {
				t.Fatal("Put succeeded")
			}
		}
		if err := s.Replace("blob", key, text); err == nil {
			t.Fatal("Replace succeeded")
		}
		recs := logged(t, buf, "disk cache write failed")
		if len(recs) != 2 {
			t.Fatalf("%d records for 21 failed writes, want 1 for put and 1 for replace:\n%s", len(recs), buf)
		}
		r := recs[0]
		if r["level"] != "WARN" || r["span"] != "cache.disk" || r["kind"] != "blob" || r["op"] != "put" ||
			!strings.Contains(r["err"].(string), "put blob "+key) || r["suppressed"] != nil {
			t.Errorf("record = %v", r)
		}
		if recs[1]["op"] != "replace" {
			t.Errorf("second record = %v, want the replace", recs[1])
		}
		if got := diskSummary(stats, "blob").WriteFailed; got != 21 {
			t.Errorf("write_failed = %d, want 21", got)
		}

		time.Sleep(logEvery)
		buf.Reset()
		_ = s.Put("blob", key, text)
		if recs := logged(t, buf, "disk cache write failed"); len(recs) != 1 || recs[0]["suppressed"] != float64(19) {
			t.Errorf("records after %v = %v, want 1 that held back 19", logEvery, recs)
		}
	})
}

// A failed write's record names the paths of the store below the home
// directory as ~, so that it doesn't name the user.
func TestWriteFailureShortensHome(t *testing.T) {
	buf, _ := capture(t)
	s := open(t)
	home := filepath.Dir(s.Dir())
	t.Setenv("HOME", home)
	if err := os.WriteFile(filepath.Join(s.Dir(), "blob"), nil, filePerm); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("blob", key, text); err == nil || !strings.Contains(err.Error(), home) {
		t.Fatalf("Put = %v, want an error that names the store", err)
	}
	recs := logged(t, buf, "disk cache write failed")
	if len(recs) != 1 {
		t.Fatalf("%d records, want 1:\n%s", len(recs), buf)
	}
	msg := recs[0]["err"].(string)
	if strings.Contains(msg, home) || !strings.Contains(msg, "~"+string(filepath.Separator)) {
		t.Errorf("err = %q, want the store under ~ rather than %s", msg, home)
	}
}

// A write that succeeds logs nothing and counts nothing.
func TestWriteLogsNothing(t *testing.T) {
	buf, stats := capture(t)
	s := open(t)
	if err := s.Put("blob", key, text); err != nil {
		t.Fatal(err)
	}
	s.Delete("blob", key)
	s.Delete("blob", key)
	if buf.Len() != 0 {
		t.Errorf("log = %s, want nothing", buf)
	}
	if got := diskSummary(stats, "blob"); got.WriteFailed != 0 || got.Dropped != 0 {
		t.Errorf("summary = %+v, want no failures", got)
	}
}

// An object that can't be read is dropped, counted and logged.
func TestUnreadableLogged(t *testing.T) {
	buf, stats := capture(t)
	s := open(t)
	if err := s.Put("blob", key, text); err != nil {
		t.Fatal(err)
	}
	p := objectPath(s, "blob", key) + gzExt
	for range 3 {
		if err := os.WriteFile(p, []byte("not gzip at all"), filePerm); err != nil {
			t.Fatal(err)
		}
		if _, ok := s.Get("blob", key); ok {
			t.Fatal("Get of a corrupt object hit")
		}
	}
	recs := logged(t, buf, "kept dropped")
	if len(recs) != 1 || recs[0]["level"] != "WARN" || recs[0]["kind"] != "blob" || recs[0]["reason"] != "unreadable" || recs[0]["err"] == nil {
		t.Errorf("records = %v, want 1 that says the object was unreadable", recs)
	}
	if got := diskSummary(stats, "blob").Dropped; got != 3 {
		t.Errorf("dropped = %d, want 3", got)
	}
}
