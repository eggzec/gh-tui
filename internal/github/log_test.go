package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
)

// captureLog makes the default logger write JSON lines at level into the
// returned buffer, and counts into fresh stats, for the rest of the test.
func captureLog(t *testing.T, level slog.Level) (*bytes.Buffer, *obs.Stats) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(obs.NewLogger(&buf, level, "s_test"))
	stats := obs.NewStats()
	prevStats := obs.SetDefault(stats)
	t.Cleanup(func() {
		slog.SetDefault(prev)
		obs.SetDefault(prevStats)
	})
	return &buf, stats
}

// httpRecords returns the records of HTTP attempts in buf.
func httpRecords(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for line := range strings.Lines(buf.String()) {
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("line %q is not JSON: %v", line, err)
		}
		if m["msg"] == "http" {
			out = append(out, m)
		}
	}
	return out
}

func TestRestRoute(t *testing.T) {
	tests := []struct {
		path, route, repo string
	}{
		{"repos/cli/cli/issues", "/repos/{owner}/{repo}/issues", "cli/cli"},
		{"repos/cli/cli/issues/42", "/repos/{owner}/{repo}/issues/{number}", "cli/cli"},
		{"repos/cli/cli/issues/42/comments", "/repos/{owner}/{repo}/issues/{number}/comments", "cli/cli"},
		{"repos/cli/cli/issues/comments/99", "/repos/{owner}/{repo}/issues/comments/{id}", "cli/cli"},
		{"repos/cli/cli/issues/42/labels/bug%20fix", "/repos/{owner}/{repo}/issues/{number}/labels/{name}", "cli/cli"},
		{"repos/cli/cli/issues/42/labels/issues", "/repos/{owner}/{repo}/issues/{number}/labels/{name}", "cli/cli"},
		{"repos/cli/cli/pulls", "/repos/{owner}/{repo}/pulls", "cli/cli"},
		{"repos/cli/cli/git/trees/main", "/repos/{owner}/{repo}/git/trees/{sha}", "cli/cli"},
		{"repos/cli/cli/git/trees/feat%2Fx", "/repos/{owner}/{repo}/git/trees/{sha}", "cli/cli"},
		{"repos/cli/cli/git/blobs/abc123", "/repos/{owner}/{repo}/git/blobs/{sha}", "cli/cli"},
		{"repos/cli/cli/contents/docs/issues/a.md", "/repos/{owner}/{repo}/contents/{path}", "cli/cli"},
		{"repos/o/issues/issues", "/repos/{owner}/{repo}/issues", "o/issues"},
		{"notifications", "/notifications", ""},
		{"notifications/threads/123", "/notifications/threads/{id}", ""},
		{"user/starred/cli/cli", "/user/starred/{owner}/{repo}", "cli/cli"},
		{"search/issues", "/search/issues", ""},
		{"/repos/cli/cli/", "/repos/{owner}/{repo}", "cli/cli"},
		{"repos/cli/cli/actions/runs", "/repos/{owner}/{repo}/actions/runs", "cli/cli"},
		{"repos/cli/cli/actions/runs/42/attempts/2/jobs", "/repos/{owner}/{repo}/actions/runs/{run_id}/attempts/{attempt}/jobs", "cli/cli"},
		{"repos/cli/cli/actions/runs/42/rerun-failed-jobs", "/repos/{owner}/{repo}/actions/runs/{run_id}/rerun-failed-jobs", "cli/cli"},
		{"repos/cli/cli/actions/workflows/7/runs", "/repos/{owner}/{repo}/actions/workflows/{workflow_id}/runs", "cli/cli"},
		{"repos/cli/cli/actions/jobs/9/logs", "/repos/{owner}/{repo}/actions/jobs/{job_id}/logs", "cli/cli"},
		{"repos/cli/cli/check-runs/9/annotations", "/repos/{owner}/{repo}/check-runs/{check_run_id}/annotations", "cli/cli"},
		{"somewhere/else", "/{}/{}", ""},
	}
	for _, tt := range tests {
		route, repo := restRoute(tt.path)
		if route != tt.route || repo != tt.repo {
			t.Errorf("restRoute(%q) = %q, %q; want %q, %q", tt.path, route, repo, tt.route, tt.repo)
		}
	}
}

func TestOperation(t *testing.T) {
	tests := []struct{ query, want string }{
		{listPullsQuery, "ListPulls"},
		{getPullQuery, "GetPull"},
		{pullCommentsQuery, "PullComments"},
		{pullIDQuery, "PullID"},
		{listReposQuery, "ListRepos"},
		{pullChecksQuery, "PullChecks"},
		{commitChecksQuery, "CommitChecks"},
		{closePullMutation, "ClosePullRequest"},
		{mergePullMutation, "MergePullRequest"},
		{"query($a: Int) { x }", "query"},
		{"mutation { x }", "mutation"},
		{"  query\n  Named { x }", "Named"},
		{"{ viewer { login } }", "query"},
	}
	for _, tt := range tests {
		if got := operation(tt.query); got != tt.want {
			t.Errorf("operation(%.30q) = %q, want %q", tt.query, got, tt.want)
		}
	}
}

// TestQueriesAskRateLimit checks that every query asks what it cost, so that
// the log has it. Mutations can't.
func TestQueriesAskRateLimit(t *testing.T) {
	for _, q := range []string{listPullsQuery, getPullQuery, pullCommentsQuery, pullReviewsQuery, pullIDQuery, listReposQuery, getRepoQuery} {
		if !strings.Contains(q, rateLimitField) {
			t.Errorf("query %s doesn't select %s", operation(q), rateLimitField)
		}
	}
}

func TestHeaderRate(t *testing.T) {
	h := http.Header{}
	h.Set("X-RateLimit-Limit", "5000")
	h.Set("X-RateLimit-Remaining", "4990")
	h.Set("X-RateLimit-Used", "10")
	h.Set("X-RateLimit-Reset", "1900000000")
	h.Set("X-RateLimit-Resource", "core")
	want := obs.Rate{Resource: "core", Limit: 5000, Remaining: 4990, Used: 10, Reset: time.Unix(1900000000, 0)}
	if got := headerRate(h); got != want {
		t.Errorf("headerRate = %+v, want %+v", got, want)
	}
	h.Del("X-RateLimit-Used")
	if got := headerRate(h); got.Used != 10 {
		t.Errorf("used without its header = %d, want limit minus remaining", got.Used)
	}
	if got := headerRate(http.Header{}); got != (obs.Rate{}) {
		t.Errorf("headerRate without headers = %+v, want zero", got)
	}
}

func TestQueryRate(t *testing.T) {
	data := json.RawMessage(`{"rateLimit":{"cost":2,"limit":5000,"remaining":4800,"used":200,"resetAt":"2030-01-01T00:00:00Z"},"repository":{}}`)
	got := queryRate(data)
	if got == nil || got.Cost != 2 || got.Remaining != 4800 || !got.ResetAt.Equal(time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("queryRate = %+v", got)
	}
	if got := queryRate(json.RawMessage(`{"result":{}}`)); got != nil {
		t.Errorf("queryRate without rateLimit = %+v, want nil", got)
	}
}

func rateHeaders(w http.ResponseWriter, resource string, remaining int) {
	w.Header().Set("X-RateLimit-Limit", "5000")
	w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
	w.Header().Set("X-RateLimit-Used", strconv.Itoa(5000-remaining))
	w.Header().Set("X-RateLimit-Reset", "1900000000")
	w.Header().Set("X-RateLimit-Resource", resource)
	w.Header().Set("X-GitHub-Request-Id", "ABCD:1234")
}

func TestLogREST(t *testing.T) {
	buf, stats := captureLog(t, slog.LevelInfo)
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rateHeaders(w, "core", 4990)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = io.WriteString(w, `{"number":42,"title":"t","user":{"login":"a"}}`)
	}))
	ctx := obs.WithTrace(context.Background(), "open.issue")
	repo := core.RepoRef{Owner: "cli", Name: "cli"}
	if _, _, err := c.GetIssue(ctx, repo, 42, Conditional{}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.GetIssue(ctx, repo, 42, Conditional{ETag: `"v1"`}); err != nil {
		t.Fatal(err)
	}

	recs := httpRecords(t, buf)
	if len(recs) != 2 {
		t.Fatalf("got %d http records, want 2:\n%s", len(recs), buf)
	}
	traceID, _ := obs.TraceID(ctx)
	r := recs[0]
	for k, want := range map[string]any{
		"level": "INFO", "span": "http", "api": "rest", "method": "GET",
		"route": "/repos/{owner}/{repo}/issues/{number}", "repo": "cli/cli",
		"status": 200.0, "not_modified": false, "gh_request_id": "ABCD:1234",
		"trace_id": traceID, "trace": "open.issue", "session_id": "s_test",
	} {
		if r[k] != want {
			t.Errorf("%s = %v, want %v", k, r[k], want)
		}
	}
	if id, _ := r["request_id"].(string); !strings.HasPrefix(id, obs.RequestPrefix) || id == recs[1]["request_id"] {
		t.Errorf("request ids = %v and %v, want one of each attempt", r["request_id"], recs[1]["request_id"])
	}
	if b, _ := r["bytes"].(float64); b == 0 {
		t.Errorf("bytes = %v, want the body's size", r["bytes"])
	}
	rate, _ := r["rate"].(map[string]any)
	if rate["resource"] != "core" || rate["remaining"] != 4990.0 || rate["used"] != 10.0 || rate["limit"] != 5000.0 {
		t.Errorf("rate = %v", rate)
	}
	if reset, _ := time.Parse(time.RFC3339, rate["reset"].(string)); !reset.Equal(time.Unix(1900000000, 0)) {
		t.Errorf("rate.reset = %v, want an RFC 3339 time", rate["reset"])
	}
	if _, ok := r["path"]; ok {
		t.Errorf("info record has the debug fields: %v", r)
	}
	if recs[1]["not_modified"] != true || recs[1]["status"] != 304.0 {
		t.Errorf("second record = %v, want a 304", recs[1])
	}

	q := stats.Summary().Quotas
	if len(q) != 1 || q[0].Requests != 2 || q[0].NotModified != 1 || q[0].Remaining != 4990 {
		t.Errorf("quotas = %+v", q)
	}
}

func TestLogGraphQL(t *testing.T) {
	buf, stats := captureLog(t, slog.LevelDebug)
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		rateHeaders(w, "graphql", 4000)
		_, _ = io.WriteString(w, `{"data":{"rateLimit":{"cost":1,"limit":5000,"remaining":4000,"used":1000,"resetAt":"2030-01-01T00:00:00Z"},"repository":{"pullRequest":{"id":"PR_1"}}}}`)
	}))
	if _, err := c.PullRequestID(context.Background(), core.RepoRef{Owner: "cli", Name: "cli"}, 1); err != nil {
		t.Fatal(err)
	}
	recs := httpRecords(t, buf)
	if len(recs) != 1 {
		t.Fatalf("got %d http records, want 1", len(recs))
	}
	r := recs[0]
	if r["api"] != "graphql" || r["route"] != "PullID" || r["repo"] != "cli/cli" || r["method"] != "POST" {
		t.Errorf("record = %v", r)
	}
	rate, _ := r["rate"].(map[string]any)
	if rate["cost"] != 1.0 || rate["resource"] != "graphql" || rate["remaining"] != 4000.0 {
		t.Errorf("rate = %v", rate)
	}
	if r["path"] != "/graphql" || r["conditional"] != false {
		t.Errorf("debug fields = %v", r)
	}
	if _, ok := r["ttfb_ms"]; !ok {
		t.Errorf("debug record has no ttfb_ms: %v", r)
	}
	if q := stats.Summary().Quotas; len(q) != 1 || q[0].Cost != 1 {
		t.Errorf("quotas = %+v, want the cost counted", q)
	}
}

func TestLogGraphQLRateWithoutHeaders(t *testing.T) {
	buf, _ := captureLog(t, slog.LevelInfo)
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"rateLimit":{"cost":3,"limit":5000,"remaining":10,"used":4990,"resetAt":"2030-01-01T00:00:00Z"},"repository":{"pullRequest":{"id":"PR_1"}}}}`)
	}))
	if _, err := c.PullRequestID(context.Background(), core.RepoRef{Owner: "cli", Name: "cli"}, 1); err != nil {
		t.Fatal(err)
	}
	rate, _ := httpRecords(t, buf)[0]["rate"].(map[string]any)
	if rate["cost"] != 3.0 || rate["remaining"] != 10.0 || rate["resource"] != "graphql" {
		t.Errorf("rate = %v, want the query's rateLimit", rate)
	}
}

func TestLogLevels(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		header    map[string]string
		wantLevel string
	}{
		{name: "not found", status: http.StatusNotFound, wantLevel: "WARN"},
		{name: "secondary rate limit", status: http.StatusForbidden, header: map[string]string{"Retry-After": "60"}, wantLevel: "WARN"},
		{name: "server error", status: http.StatusBadGateway, wantLevel: "ERROR"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf, _ := captureLog(t, slog.LevelInfo)
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"message":"no"}`)
			}))
			_, _ = c.Get(context.Background(), "notifications", Conditional{}, nil)
			r := httpRecords(t, buf)[0]
			if r["level"] != tt.wantLevel || r["status"] != float64(tt.status) {
				t.Errorf("record = %v, want %s", r, tt.wantLevel)
			}
			if v := tt.header["Retry-After"]; v != "" && r["retry_after"] != v {
				t.Errorf("retry_after = %v, want %s", r["retry_after"], v)
			}
		})
	}
}

// failingTransport fails every request.
type failingTransport struct{ err error }

func (f failingTransport) RoundTrip(*http.Request) (*http.Response, error) { return nil, f.err }

func TestLogTransportError(t *testing.T) {
	buf, stats := captureLog(t, slog.LevelDebug)
	c, err := New(WithBaseURL("http://gh.invalid/"), WithToken("t"),
		WithHTTPClient(&http.Client{Transport: failingTransport{errors.New("connection refused")}}))
	if err != nil {
		t.Fatal(err)
	}
	_, _ = c.Get(context.Background(), "repos/cli/cli/issues", Conditional{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c2, _ := New(WithBaseURL("http://gh.invalid/"), WithToken("t"),
		WithHTTPClient(&http.Client{Transport: failingTransport{context.Canceled}}))
	_, _ = c2.Get(ctx, "repos/cli/cli/issues", Conditional{}, nil)

	recs := httpRecords(t, buf)
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2:\n%s", len(recs), buf)
	}
	if r := recs[0]; r["level"] != "ERROR" || !strings.Contains(r["err"].(string), "connection refused") || r["canceled"] != false {
		t.Errorf("failed record = %v", r)
	}
	if r := recs[1]; r["level"] != "INFO" || r["canceled"] != true || r["status"] != 0.0 || r["duration_ms"] == nil {
		t.Errorf("canceled record = %v", r)
	}
	if q := stats.Summary().Quotas; len(q) != 1 || q[0].Failed != 2 || q[0].Resource != "none" {
		t.Errorf("quotas = %+v", q)
	}
}

// A request canceled while its body is read, as when the user navigates
// away from a slow search, is logged with the error.
func TestLogTransportCanceledBody(t *testing.T) {
	buf, _ := captureLog(t, slog.LevelInfo)
	release, flushed := make(chan struct{}), make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data": {`))
		w.(http.Flusher).Flush()
		close(flushed)
		<-release
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	c, err := New(WithBaseURL(srv.URL+"/"), WithToken("t"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		<-flushed
		cancel()
	}()
	if err := c.Query(ctx, "query Slow { viewer { login } }", nil, nil); err == nil {
		t.Fatal("Query succeeded, want it canceled")
	}
	recs := httpRecords(t, buf)
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1:\n%s", len(recs), buf)
	}
	if r := recs[0]; r["level"] != "INFO" || r["canceled"] != true || r["err"] == nil || r["duration_ms"] == nil {
		t.Errorf("record = %v, want the canceled read", r)
	}
}

// TestLogNeverHasSecrets sends requests with a known token and a body that
// stands for private content, at the most verbose level, and checks that
// neither reaches the log.
func TestLogNeverHasSecrets(t *testing.T) {
	const (
		token  = "ghp_s3cr3tT0k3nV4lu3"
		secret = "PRIVATE-CONTENT-7f3a"
	)
	buf, _ := captureLog(t, slog.LevelDebug)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("request without the token")
		}
		rateHeaders(w, "core", 4000)
		w.Header().Set("Set-Cookie", "session="+secret)
		w.Header().Set("ETag", `"etag"`)
		if r.URL.Path == "/graphql" {
			_, _ = io.WriteString(w, `{"data":{"repository":{"pullRequest":{"id":"`+secret+`"}}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"number":1,"title":"`+secret+`","body":"`+secret+`","user":{"login":"a"}}`)
	}))
	t.Cleanup(srv.Close)
	c, err := New(WithBaseURL(srv.URL), WithToken(token), WithHTTPClient(srv.Client()))
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	repo := core.RepoRef{Owner: "cli", Name: "cli"}
	if _, _, err := c.GetIssue(ctx, repo, 1, Conditional{ETag: `"old"`}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateIssueComment(ctx, repo, 1, secret); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PullRequestID(ctx, repo, 1); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if n := len(httpRecords(t, buf)); n != 3 {
		t.Fatalf("got %d http records, want 3:\n%s", n, out)
	}
	for _, s := range []string{token, "Bearer", "Authorization", "authorization", secret, "Set-Cookie", "session="} {
		if strings.Contains(out, s) {
			t.Errorf("log contains %q:\n%s", s, out)
		}
	}
}

// nopTransport answers every request at once with an empty body.
type nopTransport struct{}

func (nopTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	h := http.Header{}
	h.Set("X-RateLimit-Limit", "5000")
	h.Set("X-RateLimit-Remaining", "4000")
	h.Set("X-RateLimit-Reset", "1900000000")
	h.Set("X-RateLimit-Resource", "core")
	return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(strings.NewReader("{}")), Request: req}, nil
}

// BenchmarkTransport measures what logging adds to a request: at info,
// the record and the counting, and below the level, the counting only.
func BenchmarkTransport(b *testing.B) {
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://api.github.com/repos/cli/cli/issues/42", http.NoBody)
	send := func(b *testing.B, rt http.RoundTripper) {
		b.Helper()
		b.ReportAllocs()
		for b.Loop() {
			resp, err := rt.RoundTrip(req)
			if err != nil {
				b.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	}
	logged := &logTransport{base: nopTransport{}, restRoot: "/", graphqlPath: "/graphql"}
	prev := slog.Default()
	b.Cleanup(func() { slog.SetDefault(prev) })

	b.Run("bare", func(b *testing.B) { send(b, nopTransport{}) })
	b.Run("info", func(b *testing.B) {
		slog.SetDefault(obs.NewLogger(io.Discard, slog.LevelInfo, "s_bench"))
		send(b, logged)
	})
	b.Run("disabled", func(b *testing.B) {
		slog.SetDefault(obs.NewLogger(io.Discard, slog.LevelError, "s_bench"))
		send(b, logged)
	})
}
