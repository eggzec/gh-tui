package github

import (
	"bytes"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

var bubbletea = core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}

// serveActions answers requests with the named fixture, an ETag and a
// Link to the next page, after check looked at them. A request with the
// ETag gets a 304.
func serveActions(t *testing.T, name string, check func(r *http.Request)) *Client {
	t.Helper()
	body := fixture(t, name)
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		check(r)
		w.Header().Set("ETag", `W/"a1"`)
		if r.Header.Get("If-None-Match") == `W/"a1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("Link", `<http://`+r.Host+r.URL.Path+`?page=2>; rel="next"`)
		_, _ = w.Write(body)
	}))
}

func TestListRuns(t *testing.T) {
	tests := []struct {
		name    string
		filter  core.RunFilter
		perPage int
		path    string
		query   map[string]string
	}{
		{"all", core.RunFilter{}, 0, "/repos/charmbracelet/bubbletea/actions/runs", nil},
		{
			"filtered", core.RunFilter{Branch: "main", Event: "push", Status: "failure", Actor: "meowgorithm", HeadSHA: "abc"}, 50,
			"/repos/charmbracelet/bubbletea/actions/runs",
			map[string]string{"branch": "main", "event": "push", "status": "failure", "actor": "meowgorithm", "head_sha": "abc", "per_page": "50"},
		},
		{
			"workflow", core.RunFilter{WorkflowID: 2304143, Status: "in_progress"}, 500,
			"/repos/charmbracelet/bubbletea/actions/workflows/2304143/runs",
			map[string]string{"status": "in_progress", "per_page": "100"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := serveActions(t, "actions_runs.json", func(r *http.Request) {
				checkIssueRequest(t, r, http.MethodGet, tt.path, tt.query)
			})
			page, res, err := c.ListRuns(t.Context(), bubbletea, tt.filter, "", tt.perPage, Conditional{})
			if err != nil {
				t.Fatalf("ListRuns: %v", err)
			}
			if res.ETag != `W/"a1"` || !strings.HasSuffix(page.Next, tt.path+"?page=2") {
				t.Errorf("ETag %q, Next %q; want the response's", res.ETag, page.Next)
			}
			if len(page.Items) != 5 {
				t.Fatalf("got %d runs, want 5", len(page.Items))
			}
			got := page.Items[0]
			want := core.Run{
				ID: 36010288597, Attempt: 1, Name: "build",
				DisplayTitle: "fix: prevent signal goroutine deadlock on shutdown",
				Number:       5633, Event: "pull_request", Branch: "fix/signal-send-deadlock",
				HeadSHA: "546cecadf4d8cb71dd6d42be3a7fc4ee840d35a7",
				Status:  core.RunCompleted, Conclusion: core.ConclusionActionRequired,
				Actor: "simpleqt", WorkflowID: 2304143,
				CreatedAt: issueTime("2026-09-24T14:05:44Z"), UpdatedAt: issueTime("2026-09-24T14:05:44Z"),
				RunStartedAt: issueTime("2026-09-24T14:05:44Z"),
				URL:          "https://github.com/charmbracelet/bubbletea/actions/runs/36010288597",
			}
			if !equalRun(got, want) {
				t.Errorf("run = %+v\nwant  %+v", got, want)
			}
		})
	}
}

func TestListRunsCursorAndNotModified(t *testing.T) {
	c := serveActions(t, "actions_runs.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/charmbracelet/bubbletea/actions/runs", map[string]string{"page": "2"})
	})
	next := c.restURL.String() + "repos/charmbracelet/bubbletea/actions/runs?page=2"
	page, res, err := c.ListRuns(t.Context(), bubbletea, core.RunFilter{Branch: "ignored"}, next, 0, Conditional{ETag: `W/"a1"`})
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if !res.NotModified || len(page.Items) != 0 {
		t.Errorf("NotModified %v with %d runs, want a 304 and no runs", res.NotModified, len(page.Items))
	}
}

func TestGetRun(t *testing.T) {
	c := serveActions(t, "actions_run.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/charmbracelet/bubbletea/actions/runs/35713862348", nil)
	})
	run, res, err := c.GetRun(t.Context(), bubbletea, 35713862348, Conditional{})
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if res.ETag == "" || run.ID != 35713862348 || run.Conclusion != core.ConclusionFailure || run.Actor != "andrinoff" ||
		!slices.Equal(run.PullRequests, []int{1816}) || run.DisplayTitle != "feat: clipboard without OSC52" {
		t.Errorf("run = %+v", run)
	}
	if _, res, err := c.GetRun(t.Context(), bubbletea, 35713862348, Conditional{ETag: res.ETag}); err != nil || !res.NotModified {
		t.Errorf("GetRun with the ETag = %+v, %v; want a 304", res, err)
	}
}

func TestListWorkflows(t *testing.T) {
	c := serveActions(t, "actions_workflows.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/charmbracelet/bubbletea/actions/workflows", map[string]string{"per_page": "100"})
	})
	page, _, err := c.ListWorkflows(t.Context(), bubbletea, "", 100, Conditional{})
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	want := core.Workflow{
		ID: 2304143, Name: "build", Path: ".github/workflows/build.yml", State: "active",
		URL: "https://github.com/charmbracelet/bubbletea/blob/main/.github/workflows/build.yml",
	}
	if len(page.Items) != 11 || page.Items[0] != want {
		t.Errorf("got %d workflows, the first %+v; want 11, the first %+v", len(page.Items), page.Items[0], want)
	}
}

func TestListJobs(t *testing.T) {
	tests := []struct {
		name    string
		attempt int
		path    string
		query   map[string]string
	}{
		{"latest", 0, "/repos/charmbracelet/bubbletea/actions/runs/35713862348/jobs", map[string]string{"filter": "latest", "per_page": "100"}},
		{"attempt", 2, "/repos/charmbracelet/bubbletea/actions/runs/35713862348/attempts/2/jobs", map[string]string{"per_page": "100"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := serveActions(t, "actions_jobs.json", func(r *http.Request) {
				checkIssueRequest(t, r, http.MethodGet, tt.path, tt.query)
			})
			page, res, err := c.ListJobs(t.Context(), bubbletea, 35713862348, tt.attempt, "", 100, Conditional{})
			if err != nil {
				t.Fatalf("ListJobs: %v", err)
			}
			if res.ETag == "" || len(page.Items) != 12 {
				t.Fatalf("got %d jobs, ETag %q; want 12 and the ETag", len(page.Items), res.ETag)
			}
			j := page.Items[0]
			if j.ID != 106700605701 || j.RunID != 35713862348 || j.Attempt != 1 || j.Name != "build-examples / govulncheck" ||
				j.WorkflowName != "build" || j.Status != core.RunCompleted || j.Conclusion != core.ConclusionSuccess ||
				!j.StartedAt.Equal(issueTime("2026-09-22T10:03:30Z")) || !j.CompletedAt.Equal(issueTime("2026-09-22T10:03:58Z")) ||
				j.RunnerName != "GitHub Actions 1000328379" || !slices.Equal(j.Labels, []string{"ubuntu-latest"}) ||
				j.URL != "https://github.com/charmbracelet/bubbletea/actions/runs/35713862348/job/106700605701" {
				t.Errorf("job = %+v", j)
			}
			wantStep := core.Step{
				Number: 1, Name: "Set up job", Status: core.RunCompleted, Conclusion: core.ConclusionSuccess,
				StartedAt: issueTime("2026-09-22T10:03:32Z"), CompletedAt: issueTime("2026-09-22T10:03:33Z"),
			}
			if len(j.Steps) != 10 || j.Steps[0] != wantStep {
				t.Errorf("got %d steps, the first %+v; want 10, the first %+v", len(j.Steps), j.Steps[0], wantStep)
			}
		})
	}
}

func TestGetJob(t *testing.T) {
	c := serveActions(t, "actions_job.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/charmbracelet/bubbletea/actions/jobs/106700606263", nil)
	})
	j, res, err := c.GetJob(t.Context(), bubbletea, 106700606263, Conditional{})
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if !j.Done() || j.Conclusion != core.ConclusionFailure || j.Name != "build / build (windows-latest)" || len(j.Steps) != 11 {
		t.Errorf("job = %+v, want the failed windows build with 11 steps", j)
	}
	if i := slices.IndexFunc(j.Steps, func(s core.Step) bool { return s.Conclusion == core.ConclusionFailure }); i < 0 || j.Steps[i].Name != "Test" {
		t.Errorf("steps = %+v, want the Test step failed", j.Steps)
	}
	if _, res, err := c.GetJob(t.Context(), bubbletea, 106700606263, Conditional{ETag: res.ETag}); err != nil || !res.NotModified {
		t.Errorf("GetJob with the ETag = %+v, %v; want a 304", res, err)
	}
}

func TestGetJobInProgress(t *testing.T) {
	c := serveActions(t, "actions_job_in_progress.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/rust-lang/rust/actions/jobs/107985969634", nil)
	})
	j, _, err := c.GetJob(t.Context(), core.RepoRef{Owner: "rust-lang", Name: "rust"}, 107985969634, Conditional{})
	if err != nil {
		t.Fatalf("GetJob: %v", err)
	}
	if j.Done() || j.Status != core.RunInProgress || j.Conclusion != core.ConclusionNone || !j.CompletedAt.IsZero() || j.StartedAt.IsZero() {
		t.Errorf("job = %+v, want one in progress", j)
	}
	i := slices.IndexFunc(j.Steps, func(s core.Step) bool { return s.Status == core.RunInProgress })
	if i < 0 || j.Steps[i].Name != "run the build" || !j.Steps[i].CompletedAt.IsZero() {
		t.Fatalf("steps = %+v, want one in progress", j.Steps)
	}
	if next := j.Steps[i+1]; next.Status != core.RunPending || !next.StartedAt.IsZero() {
		t.Errorf("step after = %+v, want it pending", next)
	}
}

func TestListJobsFails(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	if _, _, err := c.ListJobs(t.Context(), bubbletea, 1, 0, "", 0, Conditional{}); !Refused(err) {
		t.Errorf("ListJobs = %v, want a refusal", err)
	}
}

func equalRun(a, b core.Run) bool {
	return a.ID == b.ID && a.Attempt == b.Attempt && a.Name == b.Name && a.DisplayTitle == b.DisplayTitle &&
		a.Number == b.Number && a.Event == b.Event && a.Branch == b.Branch && a.HeadSHA == b.HeadSHA &&
		a.Status == b.Status && a.Conclusion == b.Conclusion && a.Actor == b.Actor && a.WorkflowID == b.WorkflowID &&
		a.CreatedAt.Equal(b.CreatedAt) && a.UpdatedAt.Equal(b.UpdatedAt) && a.RunStartedAt.Equal(b.RunStartedAt) &&
		a.URL == b.URL && slices.Equal(a.PullRequests, b.PullRequests)
}

// BenchmarkDecodeRuns decodes a page of 100 runs, made of the recorded
// ones, as ListRuns does.
func BenchmarkDecodeRuns(b *testing.B) {
	var page struct {
		TotalCount   int               `json:"total_count"`
		WorkflowRuns []json.RawMessage `json:"workflow_runs"`
	}
	if err := json.Unmarshal(benchFixture(b, "actions_runs.json"), &page); err != nil {
		b.Fatal(err)
	}
	runs := make([]json.RawMessage, 100)
	for i := range runs {
		runs[i] = page.WorkflowRuns[i%len(page.WorkflowRuns)]
	}
	page.WorkflowRuns = runs
	body, err := json.Marshal(page)
	if err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		var v struct {
			WorkflowRuns []restRun `json:"workflow_runs"`
		}
		if err := decode(b.Context(), bytes.NewReader(body), &v); err != nil {
			b.Fatal(err)
		}
		convert(v.WorkflowRuns, restRun.core)
	}
}
