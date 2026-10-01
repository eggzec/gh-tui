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
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

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

// fakeToken looks like a token, so a test can tell that none is logged.
const fakeToken = "ghp_16C7e42F292c6912E7710c838347Ae178B4a"

// readRecords reads the records of the log at path.
func readRecords(t *testing.T, data []byte) []map[string]any {
	t.Helper()
	var recs []map[string]any
	for line := range strings.Lines(string(data)) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q: %v", line, err)
		}
		recs = append(recs, m)
	}
	return recs
}

func TestLogStart(t *testing.T) {
	restoreLogger(t)
	dir := t.TempDir()
	// The paths below the home directory are logged with it as ~.
	t.Setenv("HOME", dir)
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "LC_") {
			t.Setenv(name, "")
		}
	}
	for k, v := range map[string]string{
		"GH_TOKEN": fakeToken, "GH_ENTERPRISE_TOKEN": fakeToken, "GH_TUI_LOG": "", "GH_DEBUG": "",
		"TERM": "xterm-256color", "COLORTERM": "truecolor", "TERM_PROGRAM": "tmux", "TERM_PROGRAM_VERSION": "3.5",
		"TMUX": "/tmp/tmux-1000/default,1,0", "LANG": "en_US.UTF-8", "LC_ALL": "C.UTF-8", "NO_COLOR": "",
		"SSH_TTY": "", "SSH_CONNECTION": "",
	} {
		t.Setenv(k, v)
	}
	cfg := config.Default()
	cfg.Log.File = filepath.Join(dir, "gh-tui.log")
	cfg.Editor = "vim -c " + fakeToken
	cfg.Sync.Poll.Lists = 30 * time.Second
	closeLog, warning := openLog(cfg.Log)
	if warning != "" {
		t.Fatalf("warning = %q", warning)
	}
	logStart(cfg, filepath.Join(dir, "config.yaml"), levelFrom(cfg.Log, true))
	closeLog()

	data, err := os.ReadFile(cfg.Log.File)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), fakeToken) {
		t.Fatalf("the start record gives the token away: %s", data)
	}
	recs := readRecords(t, data)
	if len(recs) != 1 || recs[0]["msg"] != "start" {
		t.Fatalf("records = %v, want the start record", recs)
	}
	rec := recs[0]
	for key, want := range map[string]any{
		"goos": runtime.GOOS, "goarch": runtime.GOARCH, "go": runtime.Version(),
		"span": "app", "log_level": "INFO", "level_from": "--debug", "file": filepath.Join("~", "gh-tui.log"),
		"config_path": filepath.Join("~", "config.yaml"), "config_exists": false,
		"term": "xterm-256color", "colorterm": "truecolor", "term_program": "tmux", "term_program_version": "3.5",
		"multiplexer": "tmux", "ssh": false, "no_color": false,
	} {
		if rec[key] != want {
			t.Errorf("start %s = %v, want %v", key, rec[key], want)
		}
	}
	for _, key := range []string{"pid", "version", "cgo", "session_id"} {
		if _, ok := rec[key]; !ok {
			t.Errorf("start has no %s: %v", key, rec)
		}
	}
	if got, want := rec["locale"], map[string]any{"lang": "en_US.UTF-8", "lc_all": "C.UTF-8"}; !reflect.DeepEqual(got, want) {
		t.Errorf("start locale = %v, want %v", got, want)
	}
	want := map[string]any{"editor": config.Redacted, "log.file": config.Redacted, "sync.poll.lists": "30s"}
	if got := rec["non_default"]; !reflect.DeepEqual(got, want) {
		t.Errorf("start non_default = %v, want %v", got, want)
	}
}

func TestLevelFrom(t *testing.T) {
	debug := config.Log{Level: config.LevelDebug}
	for _, tt := range []struct {
		name            string
		cfg             config.Log
		flag            bool
		ghDebug, envLog string
		want            string
	}{
		{name: "default", cfg: config.Default().Log, want: "default"},
		{name: "config", cfg: debug, want: "config"},
		{name: "GH_TUI_LOG", cfg: debug, envLog: "debug", want: "GH_TUI_LOG"},
		{name: "GH_DEBUG", cfg: debug, envLog: "warn", ghDebug: "1", want: "GH_DEBUG"},
		{name: "--debug", cfg: debug, ghDebug: "1", flag: true, want: "--debug"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("GH_DEBUG", tt.ghDebug)
			t.Setenv(config.EnvLog, tt.envLog)
			if got := levelFrom(tt.cfg, tt.flag); got != tt.want {
				t.Errorf("levelFrom = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOpenLogFails(t *testing.T) {
	restoreLogger(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A file where the directory should be.
	blocker := filepath.Join(home, "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default().Log
	cfg.File = filepath.Join(blocker, "gh-tui.log")
	closeLog, warning := openLog(cfg)
	defer closeLog()
	want := "Logging is off: couldn't open " + filepath.Join("~", "blocker", "gh-tui.log") + " ("
	if !strings.HasPrefix(warning, want) || strings.Contains(warning, home) {
		t.Errorf("warning = %q, want it to start %q and not name the home directory", warning, want)
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
	query := func(cursor string) pullsvc.ListQuery {
		q := q
		q.Cursor = cursor
		return q
	}
	fetch := ui.FeedPages("list.pulls", query, func(ctx context.Context, q pullsvc.ListQuery, again bool) (core.Page[core.PullRequest], error) {
		q.Again = again
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

// GH_DEBUG turns on debug logging as gh reads it: any value but "", 0,
// false and no.
func TestGHDebug(t *testing.T) {
	for value, want := range map[string]bool{
		"": false, "0": false, "false": false, "no": false,
		"1": true, "true": true, "api": true, "yes": true,
	} {
		if got := ghDebug(value); got != want {
			t.Errorf("ghDebug(%q) = %v, want %v", value, got, want)
		}
	}
}

// The set command changes the level while the app runs, and the change is
// logged either way.
func TestSetLogLevel(t *testing.T) {
	restoreLogger(t)
	cfg := config.Default().Log
	cfg.File = filepath.Join(t.TempDir(), "gh-tui.log")
	closeLog, warning := openLog(cfg)
	if warning != "" {
		t.Fatalf("warning = %q", warning)
	}
	slog.Debug("hidden")
	setLogLevel(config.LevelDebug)
	slog.Debug("shown")
	setLogLevel(config.LevelWarn)
	slog.Info("hidden too")
	closeLog()

	data, err := os.ReadFile(cfg.File)
	if err != nil {
		t.Fatal(err)
	}
	recs := readRecords(t, data)
	got := make([]string, 0, len(recs))
	for _, r := range recs {
		msg, _ := r["msg"].(string)
		if msg == "log level set" {
			msg += " " + r["from"].(string) + ">" + r["to"].(string)
		}
		got = append(got, msg)
	}
	want := []string{"log level set INFO>DEBUG", "shown", "log level set DEBUG>WARN"}
	if !slices.Equal(got, want) {
		t.Errorf("records = %q, want %q", got, want)
	}
}

// The app is handed the config resolved for its host and account, with
// the level the log was opened at, not the top level of the file.
func TestSessionConfig(t *testing.T) {
	t.Setenv(config.EnvLog, "")
	path := filepath.Join(t.TempDir(), "config.yaml")
	const file = "sync:\n  poll: {lists: 1m}\nhosts:\n  ghe.corp.com:\n    sync: {poll: {lists: 2m}}\n" +
		"profiles:\n  work:\n    accounts: [ali@ghe.corp.com]\n    repos: [platform/api]\n"
	if err := os.WriteFile(path, []byte(file), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, src, err := sessionConfig(f, "ghe.corp.com", "Ali", config.LevelDebug)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Sync.Poll.Lists != 2*time.Minute || !slices.Equal(cfg.Repos, []string{"platform/api"}) || cfg.Log.Level != config.LevelDebug {
		t.Errorf("interval %v, repos %v, log level %q; want the host's 2m, the profile's repos and debug", cfg.Sync.Poll.Lists, cfg.Repos, cfg.Log.Level)
	}
	if src.Profile != "work" || !src.HostLayer {
		t.Errorf("source = %+v, want the host and profile work", src)
	}
}

// The config record says which layers applied and what the session's
// config changes, which the start record tells only of the top level.
func TestLogConfig(t *testing.T) {
	restoreLogger(t)
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	cfg := config.Default()
	cfg.Sync.Poll.Lists = 2 * time.Minute
	logConfig(cfg, config.Source{Host: "ghe.corp.com", HostLayer: true, Profile: "work"})
	recs := readRecords(t, buf.Bytes())
	if len(recs) != 1 {
		t.Fatalf("records = %v, want one", recs)
	}
	rec := recs[0]
	if rec["msg"] != "config" || rec["span"] != "config" || rec["host_layer"] != true || rec["profile"] != "work" {
		t.Errorf("record = %v, want the config record with the host layer and profile work", rec)
	}
	if got, want := rec["non_default"], map[string]any{"sync.poll.lists": "2m0s"}; !reflect.DeepEqual(got, want) {
		t.Errorf("non_default = %v, want %v", got, want)
	}
}
