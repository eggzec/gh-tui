package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// serverRecords returns the server records in buf.
func serverRecords(t *testing.T, buf fmt.Stringer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, r := range readLog(t, buf.String()) {
		if r["msg"] == "server" {
			out = append(out, r)
		}
	}
	return out
}

// TestServerRecord logs what the first answer says of the server, once.
func TestServerRecord(t *testing.T) {
	tests := []struct {
		name   string
		header map[string]string
		skew   time.Duration
		level  string
		want   map[string]any
		absent []string
	}{
		{
			name:   "github.com",
			header: map[string]string{apiVersionSelectedHeader: apiVersion, "X-RateLimit-Limit": "5000"},
			level:  "INFO",
			want:   map[string]any{"api_version_selected": apiVersion, "rate_limits": true, "skew_ms": 0.0},
			absent: []string{"ghes_version", "api_version_sent"},
		},
		{
			name:   "enterprise without rate limits, clock far off, another API version",
			header: map[string]string{enterpriseHeader: "3.17.4", apiVersionSelectedHeader: "2026-03-10"},
			skew:   -2 * time.Minute,
			level:  "WARN",
			want: map[string]any{
				"ghes_version": "3.17.4", "api_version_selected": "2026-03-10", "api_version_sent": apiVersion,
				"rate_limits": false, "skew_ms": -120000.0,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, _ := captureLog(t, slog.LevelInfo)
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.header {
					w.Header().Set(k, v)
				}
				w.Header().Set(requestIDHeader, "ABCD:1234")
				w.Header().Set("Date", time.Now().Add(tt.skew).UTC().Format(http.TimeFormat))
				_, _ = io.WriteString(w, `{}`)
			}))
			for range 2 {
				_, _ = c.Get(context.Background(), "user", Conditional{}, nil)
			}
			recs := serverRecords(t, buf)
			if len(recs) != 1 {
				t.Fatalf("got %d server records, want 1:\n%s", len(recs), buf)
			}
			r := recs[0]
			if r["level"] != tt.level {
				t.Errorf("level = %v, want %s", r["level"], tt.level)
			}
			for k, want := range tt.want {
				got := r[k]
				// Date has whole seconds, so the skew may be a second off.
				if k == "skew_ms" {
					if d, _ := got.(float64); d-want.(float64) > 1000 || want.(float64)-d > 1000 {
						t.Errorf("skew_ms = %v, want about %v", got, want)
					}
					continue
				}
				if got != want {
					t.Errorf("%s = %v, want %v", k, got, want)
				}
			}
			for _, k := range tt.absent {
				if _, ok := r[k]; ok {
					t.Errorf("record has %s: %v", k, r)
				}
			}
		})
	}
}

// An answer without GitHub's request id, as a proxy's, isn't the server's.
func TestServerRecordNeedsGitHub(t *testing.T) {
	buf, _ := captureLog(t, slog.LevelInfo)
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{}`)
	}))
	_, _ = c.Get(context.Background(), "user", Conditional{}, nil)
	if recs := serverRecords(t, buf); len(recs) != 0 {
		t.Errorf("server records = %v, want none", recs)
	}
}

// The fields an Enterprise Server lacks are logged with its version.
func TestFieldsUnsupportedNamesVersion(t *testing.T) {
	buf, _ := captureLog(t, slog.LevelInfo)
	data, err := os.ReadFile(filepath.Join("testdata", "repos_get_admin.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := &enterpriseServer{t: t, data: data, lacks: map[string]string{"autoMergeAllowed": "Repository"}}
	c := newEnterpriseClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(enterpriseHeader, "3.14.2")
		srv.ServeHTTP(w, r)
	}))
	if _, err := c.GetRepo(t.Context(), core.RepoRef{Owner: "eggzec", Name: "gh-tui"}); err != nil {
		t.Fatal(err)
	}
	for _, r := range readLog(t, buf.String()) {
		if r["msg"] == "graphql fields unsupported" {
			if r["ghes_version"] != "3.14.2" {
				t.Errorf("record = %v, want ghes_version 3.14.2", r)
			}
			return
		}
	}
	t.Errorf("no graphql fields unsupported record:\n%s", buf)
}

// readLog returns the records of a JSON log.
func readLog(t *testing.T, log string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.Lines(log) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q is not JSON: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}
