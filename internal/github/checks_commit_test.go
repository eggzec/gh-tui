package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

const commitSHA = "ce8b5107c44921f6276821c2f3fca04592a00305"

func TestCommitChecksAll(t *testing.T) {
	c, reqs := pullServer(t, "graphql_commit_contexts.json")
	got, err := c.CommitChecksAll(t.Context(), bubbletea, commitSHA, true)
	if err != nil {
		t.Fatalf("CommitChecksAll: %v", err)
	}
	checkPullQuery(t, reqs(), "query CommitContextsNotes(", map[string]any{"owner": "charmbracelet", "name": "bubbletea", "sha": commitSHA})
	if got.SHA != commitSHA || got.State != core.ChecksPending || got.Total != 4 || got.Truncated || !got.Notes ||
		len(got.Runs) != 3 || len(got.Statuses) != 1 || !got.Pending() {
		t.Fatalf("checks = %+v, want 3 runs and a status, pending", got)
	}

	job := got.Runs[0]
	if job.ID != 106700606263 || job.JobID != 106700606263 || job.RunID != 35713862348 || job.RunNumber != 87 ||
		job.Event != "pull_request" || job.Workflow != "build" || !job.InActions() || job.SuiteID != 4242 ||
		job.Conclusion != core.ConclusionFailure || job.Status != core.RunCompleted || job.Annotations != 2 {
		t.Errorf("job = %+v, want a failed job of run 87 of the workflow build", job)
	}
	if job.App == nil || *job.App != (core.CheckApp{Name: "GitHub Actions", Slug: "github-actions", LogoURL: "https://avatars.githubusercontent.com/in/15368?v=4"}) {
		t.Errorf("job app = %+v, want GitHub Actions", job.App)
	}
	wantNotes := []core.Annotation{
		{Path: "tea.go", StartLine: 3, EndLine: 3, StartColumn: 5, EndColumn: 9, Level: core.AnnotationFailure, Title: "build", Message: "undefined: x", RawDetails: "details"},
		{Path: "tea_test.go", StartLine: 10, EndLine: 12, Level: core.AnnotationWarning, Message: "slow"},
	}
	if len(job.Notes) != 2 || job.Notes[0] != wantNotes[0] || job.Notes[1] != wantNotes[1] {
		t.Errorf("notes = %+v\nwant    %+v", job.Notes, wantNotes)
	}

	cov := got.Runs[1]
	if cov.App == nil || cov.App.Slug != "codecov" || cov.InActions() || cov.JobID != 0 || cov.Status != core.RunInProgress ||
		cov.Conclusion != "" || cov.Title != "60.08% (+2.26%) compared to 657aaae" || cov.Summary != "coverage summary" ||
		cov.Text != "coverage details" || len(cov.Notes) != 0 {
		t.Errorf("codecov = %+v, want an App's run outside Actions, in progress", cov)
	}

	if orphan := got.Runs[2]; orphan.App != nil || orphan.SuiteID != 4244 || orphan.InActions() || orphan.Name != "orphan" {
		t.Errorf("orphan = %+v, want a run of a suite without an App", orphan)
	}

	want := core.StatusContext{
		Context: "ci/jenkins", State: "pending", Description: "Build queued",
		TargetURL: "https://jenkins.example.com/job/1", CreatedAt: pullTime("2026-09-22T11:03:00Z"),
	}
	if s := got.Statuses[0]; !s.CreatedAt.Equal(want.CreatedAt) || s.Context != want.Context || s.State != want.State ||
		s.Description != want.Description || s.TargetURL != want.TargetURL {
		t.Errorf("status = %+v, want %+v", s, want)
	}
}

// Without notes the read asks for the count of annotations alone.
func TestCommitChecksAllWithoutNotes(t *testing.T) {
	c, reqs := pullServer(t, "graphql_commit_contexts.json")
	got, err := c.CommitChecksAll(t.Context(), bubbletea, commitSHA, false)
	if err != nil {
		t.Fatalf("CommitChecksAll: %v", err)
	}
	r := reqs()
	checkPullQuery(t, r, "query CommitContexts(", map[string]any{"owner": "charmbracelet", "name": "bubbletea", "sha": commitSHA})
	if strings.Contains(r[0].Query, "rawDetails") || strings.Contains(r[0].Query, "annotationLevel") {
		t.Errorf("query without notes selects annotations:\n%s", r[0].Query)
	}
	if got.Notes {
		t.Error("checks read without notes say they have them")
	}
}

// contextsServer lists total contexts, 100 to a page: check runs, then from
// statuses on, commit statuses. With forever set it ignores the cursor it is
// sent: every answer is the first page, with a next page after the same cursor.
func contextsServer(t *testing.T, total, statuses int, forever bool) (client *Client, requests func() []pullQuery) {
	t.Helper()
	var (
		mu   sync.Mutex
		reqs []pullQuery
	)
	node := func(i int) string {
		if i >= total-statuses {
			return fmt.Sprintf(`{"__typename":"StatusContext","context":"status %d","state":"SUCCESS","description":"","targetUrl":"","createdAt":"2026-09-22T11:03:00Z"}`, i)
		}
		return fmt.Sprintf(`{"__typename":"CheckRun","databaseId":%d,"name":"job %d","status":"COMPLETED","conclusion":"SUCCESS",`+
			`"checkSuite":{"databaseId":1,"app":{"name":"GitHub Actions","slug":"github-actions","logoUrl":""},`+
			`"workflowRun":{"databaseId":7,"runNumber":3,"event":"push","url":"","workflow":{"name":"ci"}}}}`, i, i)
	}
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req pullQuery
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		mu.Lock()
		reqs = append(reqs, req)
		mu.Unlock()
		from := 0
		if after, ok := req.Variables["after"].(string); ok && !forever {
			if _, err := fmt.Sscanf(after, "after-%d", &from); err != nil {
				t.Errorf("cursor %q: %v", after, err)
			}
		}
		to := min(from+100, total)
		var nodes []string
		for i := from; i < to; i++ {
			nodes = append(nodes, node(i))
		}
		_, _ = fmt.Fprintf(w, `{"data":{"repository":{"object":{"oid":"abc","statusCheckRollup":{"state":"SUCCESS","contexts":{"totalCount":%d,`+
			`"pageInfo":{"hasNextPage":%t,"endCursor":"after-%d"},"nodes":[%s]}}}}}}`,
			total, to < total || forever, to, strings.Join(nodes, ","))
	}))
	return c, func() []pullQuery {
		mu.Lock()
		defer mu.Unlock()
		return reqs
	}
}

// A commit of more than 100 contexts is read to its end, with the cursor of
// each page asked for the next.
func TestCommitChecksAllPages(t *testing.T) {
	c, reqs := contextsServer(t, 230, 15, false)
	got, err := c.CommitChecksAll(t.Context(), bubbletea, "abc", false)
	if err != nil {
		t.Fatalf("CommitChecksAll: %v", err)
	}
	if len(got.Runs) != 215 || len(got.Statuses) != 15 || got.Total != 230 || got.Truncated {
		t.Fatalf("checks = %d runs, %d statuses of %d, truncated %v; want 215, 15 of 230, whole", len(got.Runs), len(got.Statuses), got.Total, got.Truncated)
	}
	if got.Runs[150].Name != "job 150" || got.Statuses[14].Context != "status 229" {
		t.Errorf("run 150 = %q, last status = %q; want them in order", got.Runs[150].Name, got.Statuses[14].Context)
	}
	r := reqs()
	after := make([]any, 0, len(r))
	for _, q := range r {
		after = append(after, q.Variables["after"])
	}
	if len(r) != 3 || fmt.Sprint(after) != "[<nil> after-100 after-200]" {
		t.Errorf("%d requests with cursors %v, want 3 with none, after-100 and after-200", len(r), after)
	}
}

// A server that never ends its pages is read to a bound, and the read says
// it is not whole.
func TestCommitChecksAllStopsAtMaxPages(t *testing.T) {
	c, reqs := contextsServer(t, 100*commitContextPages+50, 0, false)
	got, err := c.CommitChecksAll(t.Context(), bubbletea, "abc", false)
	if err != nil {
		t.Fatalf("CommitChecksAll: %v", err)
	}
	if n := len(reqs()); n != commitContextPages {
		t.Errorf("%d requests, want %d", n, commitContextPages)
	}
	if len(got.Runs) != 100*commitContextPages || !got.Truncated {
		t.Errorf("checks = %d runs, truncated %v; want %d, truncated", len(got.Runs), got.Truncated, 100*commitContextPages)
	}
}

// A server that answers every page with the same cursor, whatever it is sent,
// is read twice at most: the repeat is dropped, not added again, and the read
// says it is not whole.
func TestCommitChecksAllStuckCursor(t *testing.T) {
	c, reqs := contextsServer(t, 100, 0, true)
	got, err := c.CommitChecksAll(t.Context(), bubbletea, "abc", false)
	if err != nil {
		t.Fatalf("CommitChecksAll: %v", err)
	}
	if n := len(reqs()); n > 2 {
		t.Errorf("%d requests, want 2 at most", n)
	}
	if len(got.Runs) != 100 || !got.Truncated {
		t.Errorf("checks = %d runs, truncated %v; want 100 without repeats, truncated", len(got.Runs), got.Truncated)
	}
}

// A next page without a cursor to ask for it is a read that is not whole.
func TestCommitChecksAllNoCursor(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"repository":{"object":{"oid":"abc","statusCheckRollup":{"state":"SUCCESS","contexts":{"totalCount":0,` +
			`"pageInfo":{"hasNextPage":true,"endCursor":""},"nodes":[]}}}}}}`))
	}))
	got, err := c.CommitChecksAll(t.Context(), bubbletea, "abc", false)
	if err != nil || !got.Truncated {
		t.Errorf("CommitChecksAll = %+v, %v; want a truncated read", got, err)
	}
}

// A server whose schema lacks the App of a suite and the annotations of a
// run answers the read without them, and the rest is read.
func TestCommitChecksAllWithoutAppOrAnnotations(t *testing.T) {
	data := `{"data":{"repository":{"object":{"oid":"abc","statusCheckRollup":{"state":"SUCCESS","contexts":{"totalCount":1,` +
		`"pageInfo":{"hasNextPage":false,"endCursor":"x"},"nodes":[{"__typename":"CheckRun","databaseId":5,"name":"lint",` +
		`"status":"COMPLETED","conclusion":"SUCCESS","checkSuite":{"databaseId":9,` +
		`"workflowRun":{"databaseId":7,"runNumber":3,"event":"push","url":"","workflow":{"name":"ci"}}}}]}}}}}}`
	srv := &enterpriseServer{t: t, data: []byte(data), lacks: map[string]string{"app": "CheckSuite", "annotations": "CheckRun"}}
	c := newEnterpriseClient(t, srv)
	got, err := c.CommitChecksAll(t.Context(), bubbletea, "abc", true)
	if err != nil {
		t.Fatalf("CommitChecksAll: %v", err)
	}
	if len(got.Runs) != 1 {
		t.Fatalf("checks = %+v, want the one run", got)
	}
	if r := got.Runs[0]; r.App != nil || r.Notes != nil || r.Annotations != 0 || r.RunID != 7 || r.RunNumber != 3 || r.JobID != 5 || r.Workflow != "ci" {
		t.Errorf("run = %+v, want one of its workflow run, without App and annotations", r)
	}
	sent := srv.sent()
	if len(sent) != 2 || !strings.Contains(sent[0], "annotations(first:") || !strings.Contains(sent[0], "app {") {
		t.Fatalf("sent %d queries, want the full one and then another:\n%q", len(sent), sent)
	}
	if strings.Contains(sent[1], "annotations") || strings.Contains(sent[1], "logoUrl") {
		t.Errorf("second query still selects the missing fields:\n%s", sent[1])
	}
}

func TestCommitChecksAllMissing(t *testing.T) {
	for name, body := range map[string]string{
		"no object":    `{"data":{"repository":{"object":null}}}`,
		"not a commit": `{"data":{"repository":{"object":{}}}}`,
		"no repo":      `{"data":{"repository":null},"errors":[{"type":"NOT_FOUND","message":"Could not resolve"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			if _, err := c.CommitChecksAll(t.Context(), bubbletea, "abc", false); !errors.Is(err, core.ErrNotFound) {
				t.Errorf("CommitChecksAll = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestCommitChecksAllNone(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"repository":{"object":{"oid":"abc","statusCheckRollup":null}}}}`))
	}))
	got, err := c.CommitChecksAll(t.Context(), bubbletea, "abc", false)
	if err != nil || got.SHA != "abc" || len(got.Runs) != 0 || got.Pending() {
		t.Errorf("CommitChecksAll = %+v, %v; want no checks", got, err)
	}
}
