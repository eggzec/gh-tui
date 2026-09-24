package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/obs"
	pullsvc "github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func restoreLogger(t *testing.T) {
	t.Helper()
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
}

func TestOpenLog(t *testing.T) {
	restoreLogger(t)
	path := filepath.Join(t.TempDir(), "state", "gh-tui.log")
	cfg := config.Default().Log
	cfg.File, cfg.Level = path, config.LevelWarn
	closeLog, warning := openLog(cfg)
	if warning != "" {
		t.Fatalf("warning = %q", warning)
	}
	slog.Info("hidden")
	slog.Warn("shown")
	closeLog()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	// The start record is at info, below the level.
	if len(lines) != 1 {
		t.Fatalf("log = %q, want only the warning", data)
	}
	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatal(err)
	}
	if id, _ := rec["session_id"].(string); rec["msg"] != "shown" || !strings.HasPrefix(id, obs.SessionPrefix) {
		t.Errorf("record = %v, want the warning with a session id", rec)
	}
}

func TestOpenLogFails(t *testing.T) {
	restoreLogger(t)
	dir := t.TempDir()
	// A file where the directory should be.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default().Log
	cfg.File = filepath.Join(blocker, "gh-tui.log")
	closeLog, warning := openLog(cfg)
	defer closeLog()
	if !strings.HasPrefix(warning, "Logging is off") {
		t.Errorf("warning = %q, want logging off", warning)
	}
	slog.Error("dropped") // must not panic or write anywhere
}

// graphqlTransport answers every GraphQL query with an empty page of pull
// requests, and records the trace of each request.
type graphqlTransport struct{ traces []string }

func (g *graphqlTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	id, _ := obs.TraceID(req.Context())
	g.traces = append(g.traces, id)
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("X-RateLimit-Limit", "5000")
	h.Set("X-RateLimit-Remaining", "4999")
	h.Set("X-RateLimit-Reset", "1900000000")
	h.Set("X-RateLimit-Resource", "graphql")
	body := `{"data":{"rateLimit":{"cost":1,"limit":5000,"remaining":4999,"used":1,"resetAt":"2030-01-01T00:00:00Z"},` +
		`"repository":{"pullRequests":{"nodes":[],"pageInfo":{"hasNextPage":false}}}}}`
	return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

// TestTracePropagates reads a list the way the pulls section does, and
// checks that the trace it starts reaches the HTTP request and its record.
func TestTracePropagates(t *testing.T) {
	restoreLogger(t)
	var buf bytes.Buffer
	slog.SetDefault(obs.NewLogger(&buf, slog.LevelDebug, "s_test"))

	rt := new(graphqlTransport)
	client, err := github.New(github.WithBaseURL("https://gh.test/"), github.WithToken("t"), github.WithHTTPClient(&http.Client{Transport: rt}))
	if err != nil {
		t.Fatal(err)
	}
	svc := pullsvc.New(client)
	q := pullsvc.ListQuery{Repo: core.RepoRef{Owner: "cli", Name: "cli"}}
	fetch := ui.FeedPages("list.pulls", new(ui.Offline), func(ctx context.Context, cursor string) (core.Page[core.PullRequest], error) {
		q.Cursor = cursor
		return svc.List(ctx, q)
	})
	if _, _, err := fetch(context.Background(), ""); err != nil {
		t.Fatal(err)
	}

	if len(rt.traces) != 1 || rt.traces[0] == "" {
		t.Fatalf("request traces = %v, want one", rt.traces)
	}
	var req, end map[string]any
	for line := range strings.Lines(buf.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatal(err)
		}
		switch m["msg"] {
		case "http":
			req = m
		case "done":
			end = m
		}
	}
	if req["trace_id"] != rt.traces[0] || req["trace"] != "list.pulls" || req["route"] != "ListPulls" {
		t.Errorf("http record = %v, want it in trace %s", req, rt.traces[0])
	}
	if end["trace_id"] != rt.traces[0] {
		t.Errorf("end record = %v, want it in trace %s", end, rt.traces[0])
	}
}
