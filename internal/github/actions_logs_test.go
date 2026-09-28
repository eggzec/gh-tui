package github

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// jobLogPath is where the API serves the log of job 9 of bubbletea.
const jobLogPath = "/repos/charmbracelet/bubbletea/actions/jobs/9/logs"

// recordedLog is a job log as GitHub's storage served it.
func recordedLog(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("../core/testdata/actions_log_lint_windows.txt")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// serveJobLog starts a storage that serves the log with storage, and an
// API that redirects the log of job 9 to it, with a signature in the
// query. It returns a client of the API.
func serveJobLog(t *testing.T, storage http.HandlerFunc) *Client {
	t.Helper()
	blob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, h := range []string{"Authorization", "X-GitHub-Api-Version", "Cookie"} {
			if v := r.Header.Get(h); v != "" {
				t.Errorf("storage got %s %q, want none", h, v)
			}
		}
		if r.URL.Query().Get("sig") != "s3cr3t" {
			t.Errorf("storage URL %s lost its signature", r.URL)
		}
		storage(w, r)
	}))
	t.Cleanup(blob.Close)
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != jobLogPath {
			t.Errorf("API request %s, want %s", r.URL.Path, jobLogPath)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("API request without the token")
		}
		http.Redirect(w, r, blob.URL+"/actions-results/job-logs.txt?rsct=text%2Fplain&sig=s3cr3t", http.StatusFound)
	}))
}

func TestJobLog(t *testing.T) {
	buf, _ := captureLog(t, slog.LevelDebug)
	want := recordedLog(t)
	c := serveJobLog(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write(want)
	})

	got, truncated, err := c.JobLog(t.Context(), bubbletea, 9, DefaultLogLimit)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if !bytes.Equal(got, want) || truncated {
		t.Errorf("got %d bytes, truncated %v; want the %d of the log", len(got), truncated, len(want))
	}

	recs := httpRecords(t, buf)
	if len(recs) != 2 {
		t.Fatalf("logged %d requests, want the redirect and the download", len(recs))
	}
	if recs[0]["route"] != "/repos/{owner}/{repo}/actions/jobs/{job_id}/logs" || recs[0]["status"] != float64(http.StatusFound) {
		t.Errorf("API record = %v", recs[0])
	}
	if recs[1]["api"] != apiDownload || recs[1]["route"] != jobLogRoute || recs[1]["repo"] != "charmbracelet/bubbletea" {
		t.Errorf("download record = %v", recs[1])
	}
	if strings.Contains(buf.String(), "s3cr3t") || strings.Contains(buf.String(), "test-token") {
		t.Errorf("the log holds the signature or the token:\n%s", buf)
	}
}

func TestJobLogFromTheAPI(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("2026-09-22T09:49:37Z hello\n"))
	}))
	got, _, err := c.JobLog(t.Context(), bubbletea, 9, DefaultLogLimit)
	if err != nil || string(got) != "2026-09-22T09:49:37Z hello\n" {
		t.Errorf("JobLog = %q, %v; want the body", got, err)
	}
}

// TestJobLogTail reads a log larger than the limit, from a storage that
// serves ranges as Azure's does.
func TestJobLogTail(t *testing.T) {
	log := recordedLog(t)
	var ranges []string
	c := serveJobLog(t, func(w http.ResponseWriter, r *http.Request) {
		ranges = append(ranges, r.Header.Get("Range"))
		http.ServeContent(w, r, "job-logs.txt", time.Time{}, bytes.NewReader(log))
	})

	const limit = 1000
	got, truncated, err := c.JobLog(t.Context(), bubbletea, 9, limit)
	if err != nil {
		t.Fatalf("JobLog: %v", err)
	}
	if !truncated || len(got) > limit || !bytes.HasSuffix(log, got) {
		t.Fatalf("got %d bytes, truncated %v; want at most the last %d", len(got), truncated, limit)
	}
	if !bytes.HasPrefix(got, []byte("2026-09-22T")) {
		t.Errorf("tail starts with %q, want the start of a line", got[:min(len(got), 30)])
	}
	if want := "bytes=" + strconv.Itoa(len(log)-limit) + "-"; len(ranges) != 2 || ranges[0] != "" || ranges[1] != want {
		t.Errorf("ranges = %q, want the whole log and then %q", ranges, want)
	}
}

func TestJobLogFails(t *testing.T) {
	tests := []struct {
		name    string
		storage http.HandlerFunc
		want    error
	}{
		{
			// What the storage answers while the job runs.
			"in progress",
			func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><Error><Code>BlobNotFound</Code></Error>`))
			},
			core.ErrLogPending,
		},
		{
			"too large without ranges",
			func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write(bytes.Repeat([]byte("x\n"), 1000))
			},
			core.ErrTooLarge,
		},
		{
			"too large of unknown size",
			func(w http.ResponseWriter, _ *http.Request) {
				for range 10 {
					_, _ = w.Write(bytes.Repeat([]byte("x\n"), 100))
					w.(http.Flusher).Flush()
				}
			},
			core.ErrTooLarge,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := serveJobLog(t, tt.storage)
			_, _, err := c.JobLog(t.Context(), bubbletea, 9, 1000)
			if !errors.Is(err, tt.want) {
				t.Errorf("JobLog = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestJobLogRefused(t *testing.T) {
	tests := []struct {
		name   string
		answer http.HandlerFunc
		want   error
	}{
		{
			"expired",
			func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"message":"Server Error","status":"410"}`, http.StatusGone)
			},
			core.ErrLogExpired,
		},
		{
			"not found",
			func(w http.ResponseWriter, _ *http.Request) {
				http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
			},
			core.ErrNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, tt.answer)
			if _, _, err := c.JobLog(t.Context(), bubbletea, 9, DefaultLogLimit); !errors.Is(err, tt.want) {
				t.Errorf("JobLog = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestJobLogRedirectElsewhere(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "ftp://storage.example.com/log?sig=s3cr3t")
		w.WriteHeader(http.StatusFound)
	}))
	_, _, err := c.JobLog(t.Context(), bubbletea, 9, DefaultLogLimit)
	if err == nil || strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("JobLog = %v, want a refusal that doesn't show the URL", err)
	}
}

func TestJobLogStorageUnreachable(t *testing.T) {
	blob := httptest.NewServer(http.NotFoundHandler())
	blobURL := blob.URL
	blob.Close()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, blobURL+"/log?sig=s3cr3t", http.StatusFound)
	}))
	_, _, err := c.JobLog(t.Context(), bubbletea, 9, DefaultLogLimit)
	if err == nil || strings.Contains(err.Error(), "s3cr3t") {
		t.Errorf("JobLog = %v, want an error that doesn't show the signature", err)
	}
	if !Unreachable(t.Context(), err) {
		t.Errorf("Unreachable(%v) = false, want the outage reported", err)
	}
}

// TestJobLogFrom reads the log of a job in progress from an offset, from a
// storage that serves ranges as Azure's does.
func TestJobLogFrom(t *testing.T) {
	log := recordedLog(t)
	half := int64(bytes.IndexByte(log[len(log)/2:], '\n') + len(log)/2 + 1)
	tests := []struct {
		name   string
		blob   []byte
		offset int64
		limit  int64
		want   LogPart
		ranges []string
	}{
		{"from the start", log, 0, DefaultLogLimit, LogPart{Text: log, Size: int64(len(log))}, []string{""}},
		{"what was added", log, half, DefaultLogLimit, LogPart{Text: log[half:], Start: half, Size: int64(len(log))}, []string{"bytes=" + strconv.FormatInt(half, 10) + "-"}},
		{"nothing added", log, int64(len(log)), DefaultLogLimit, LogPart{Start: int64(len(log)), Size: int64(len(log))}, []string{"bytes=" + strconv.Itoa(len(log)) + "-"}},
		{
			// The log started over, so it is read again from its start.
			"shorter", log[:half], int64(len(log)), DefaultLogLimit,
			LogPart{Text: log[:half], Size: half},
			[]string{"bytes=" + strconv.Itoa(len(log)) + "-", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ranges []string
			c := serveJobLog(t, func(w http.ResponseWriter, r *http.Request) {
				ranges = append(ranges, r.Header.Get("Range"))
				http.ServeContent(w, r, "job-logs.txt", time.Time{}, bytes.NewReader(tt.blob))
			})
			got, err := c.JobLogFrom(t.Context(), bubbletea, 9, tt.offset, tt.limit)
			if err != nil {
				t.Fatalf("JobLogFrom: %v", err)
			}
			if !bytes.Equal(got.Text, tt.want.Text) || got.Start != tt.want.Start || got.Size != tt.want.Size {
				t.Errorf("got %d bytes from %d of %d, want %d from %d of %d",
					len(got.Text), got.Start, got.Size, len(tt.want.Text), tt.want.Start, tt.want.Size)
			}
			if !slices.Equal(ranges, tt.ranges) {
				t.Errorf("ranges = %q, want %q", ranges, tt.ranges)
			}
		})
	}
}

// TestJobLogFromTail reads the end of a log to which more than the limit
// was added since the offset.
func TestJobLogFromTail(t *testing.T) {
	log := recordedLog(t)
	c := serveJobLog(t, func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "job-logs.txt", time.Time{}, bytes.NewReader(log))
	})
	const limit = 1000
	got, err := c.JobLogFrom(t.Context(), bubbletea, 9, 10, limit)
	if err != nil {
		t.Fatalf("JobLogFrom: %v", err)
	}
	if got.Start <= int64(len(log))-limit || got.Size != int64(len(log)) || !bytes.Equal(got.Text, log[got.Start:]) {
		t.Errorf("got %d bytes from %d of %d, want the last lines within %d", len(got.Text), got.Start, got.Size, limit)
	}
}

// TestJobLogFromWithoutRanges reads a log from a storage that ignores the
// range, and sends the whole log instead.
func TestJobLogFromWithoutRanges(t *testing.T) {
	c := serveJobLog(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("a\nb\n"))
	})
	got, err := c.JobLogFrom(t.Context(), bubbletea, 9, 2, DefaultLogLimit)
	if err != nil || string(got.Text) != "a\nb\n" || got.Start != 0 || got.Size != 4 {
		t.Errorf("JobLogFrom = %+v, %v; want the whole log from 0", got, err)
	}
}

func TestJobLogFromPending(t *testing.T) {
	for _, offset := range []int64{0, 100} {
		c := serveJobLog(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		})
		if _, err := c.JobLogFrom(t.Context(), bubbletea, 9, offset, DefaultLogLimit); !errors.Is(err, core.ErrLogPending) {
			t.Errorf("JobLogFrom from %d = %v, want ErrLogPending", offset, err)
		}
	}
}

// serveRunningLog starts a storage that serves the log with storage, and
// an API that redirects to it and counts the redirects.
func serveRunningLog(t *testing.T, storage http.HandlerFunc) (*Client, *atomic.Int32) {
	t.Helper()
	blob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if v := r.Header.Get("Authorization"); v != "" {
			t.Errorf("storage got Authorization %q, want none", v)
		}
		storage(w, r)
	}))
	t.Cleanup(blob.Close)
	var asked atomic.Int32
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		http.Redirect(w, r, blob.URL+"/job-logs.txt?sig="+strconv.Itoa(int(asked.Load())), http.StatusFound)
	}))
	return c, &asked
}

// The signed URL serves the reads of a minute, and is asked for again once
// it expired or the storage refused it.
func TestJobLogFromKeepsTheURL(t *testing.T) {
	log := []byte("a\nb\n")
	refuse := ""
	c, asked := serveRunningLog(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("sig") == refuse {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		http.ServeContent(w, r, "job-logs.txt", time.Time{}, bytes.NewReader(log))
	})
	now := time.Now()
	c.budget.now = func() time.Time { return now }
	read := func(offset int64) {
		t.Helper()
		if _, err := c.JobLogFrom(t.Context(), bubbletea, 9, offset, DefaultLogLimit); err != nil {
			t.Fatalf("JobLogFrom from %d: %v", offset, err)
		}
	}
	for _, offset := range []int64{0, 2, 4} {
		read(offset)
	}
	if n := asked.Load(); n != 1 {
		t.Errorf("asked the API %d times in three reads, want once", n)
	}
	now = now.Add(logURLTTL)
	read(4)
	if n := asked.Load(); n != 2 {
		t.Errorf("asked the API %d times, want again once the URL expired", n)
	}
	refuse = "2"
	read(4)
	if n := asked.Load(); n != 3 {
		t.Errorf("asked the API %d times, want again once the storage refused the URL", n)
	}
}

func TestJobLogFromFallbacks(t *testing.T) {
	log := []byte("2026-09-22T09:49:37Z one\n2026-09-22T09:49:38Z two\n")
	tests := []struct {
		name    string
		storage http.HandlerFunc
		limit   int64
		want    LogPart
	}{
		{
			// Without its size, a refused range may be a log that started
			// over, so the log is read again.
			"416 of unknown size",
			func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Range") != "" {
					w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
					return
				}
				_, _ = w.Write(log)
			},
			DefaultLogLimit,
			LogPart{Text: log, Size: int64(len(log))},
		},
		{
			"206 of unknown size, too large",
			func(w http.ResponseWriter, r *http.Request) {
				if rng := r.Header.Get("Range"); rng == "bytes=1-" {
					w.Header().Set("Content-Range", "bytes 1-49/*")
					w.WriteHeader(http.StatusPartialContent)
					_, _ = w.Write(log[1:])
					return
				}
				http.ServeContent(w, r, "job-logs.txt", time.Time{}, bytes.NewReader(log))
			},
			30,
			LogPart{Text: log[25:], Start: 25, Size: int64(len(log))},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, _ := serveRunningLog(t, tt.storage)
			got, err := c.JobLogFrom(t.Context(), bubbletea, 9, 1, tt.limit)
			if err != nil {
				t.Fatalf("JobLogFrom: %v", err)
			}
			if !bytes.Equal(got.Text, tt.want.Text) || got.Start != tt.want.Start || got.Size != tt.want.Size {
				t.Errorf("got %q from %d of %d, want %q from %d of %d", got.Text, got.Start, got.Size, tt.want.Text, tt.want.Start, tt.want.Size)
			}
		})
	}
}
