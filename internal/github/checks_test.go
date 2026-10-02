package github

import (
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestPullChecks(t *testing.T) {
	c, reqs := pullServer(t, "graphql_pull_checks.json")
	got, err := c.PullChecks(t.Context(), bubbletea, 1816)
	if err != nil {
		t.Fatalf("PullChecks: %v", err)
	}
	checkPullQuery(t, reqs(), "query PullChecks(", map[string]any{"owner": "charmbracelet", "name": "bubbletea", "number": float64(1816)})

	if got.SHA != "c11778a9bb071cc48387ff163ac44456ed859042" || got.Head != got.SHA || got.State != core.ChecksSuccess ||
		got.Total != 34 || got.Truncated || len(got.Runs) != 34 || len(got.Statuses) != 0 {
		t.Fatalf("checks = %+v, want the 34 check runs of the head commit", got)
	}
	want := core.Check{
		ID: 106717521578, JobID: 106717521578, RunID: 35719097931, Workflow: "lint",
		Name: "lint / lint (ubuntu-latest)", Status: core.RunCompleted, Conclusion: core.ConclusionSuccess,
		DetailsURL:  "https://github.com/charmbracelet/bubbletea/actions/runs/35719097931/job/106717521578",
		Annotations: 2, StartedAt: pullTime("2026-09-22T11:01:01Z"), CompletedAt: pullTime("2026-09-22T11:01:32Z"),
		Required: true,
	}
	if i := slices.IndexFunc(got.Runs, func(r core.Check) bool { return r.ID == want.ID }); i < 0 || utcCheck(got.Runs[i]) != utcCheck(want) {
		t.Errorf("lint check = %+v\nwant          %+v", got.Runs[max(i, 0)], want)
	}
	codecov := got.Runs[len(got.Runs)-1]
	if codecov.Name != "codecov/project" || codecov.JobID != 0 || codecov.RunID != 0 || codecov.Workflow != "" ||
		codecov.Title != "60.08% (+2.26%) compared to 657aaae" || codecov.Summary == "" {
		t.Errorf("codecov check = %+v, want one outside Actions with its title and summary", codecov)
	}
}

func TestPullChecksStatuses(t *testing.T) {
	c, _ := pullServer(t, "graphql_pull_checks_statuses.json")
	got, err := c.PullChecks(t.Context(), core.RepoRef{Owner: "kubernetes", Name: "kubernetes"}, 142405)
	if err != nil {
		t.Fatalf("PullChecks: %v", err)
	}
	if got.State != core.ChecksFailure || len(got.Runs) != 0 || len(got.Statuses) != 13 {
		t.Fatalf("checks = %+v, want 13 commit statuses", got)
	}
	want := core.StatusContext{
		Context: "pull-kubernetes-cmd", State: "failure",
		// Prow pads the description with em quads.
		Description: "Job failed." + strings.Repeat("\u2001", 20) + " BaseSHA:6384b87ed0bef8bc893d2d4fd7ab93a1ce0fc2e1",
		TargetURL:   "https://prow.k8s.io/view/gs/kubernetes-ci-logs/pr-logs/pull/142405/pull-kubernetes-cmd/2103382557906178048",
		CreatedAt:   pullTime("2026-09-25T07:17:12Z"), Required: true,
	}
	got.Statuses[0].CreatedAt = got.Statuses[0].CreatedAt.UTC()
	want.CreatedAt = want.CreatedAt.UTC()
	if got.Statuses[0] != want {
		t.Errorf("status = %+v\nwant     %+v", got.Statuses[0], want)
	}
}

func TestCommitChecks(t *testing.T) {
	c, reqs := pullServer(t, "graphql_commit_checks.json")
	sha := "ce8b5107c44921f6276821c2f3fca04592a00305"
	got, err := c.CommitChecks(t.Context(), bubbletea, sha)
	if err != nil {
		t.Fatalf("CommitChecks: %v", err)
	}
	checkPullQuery(t, reqs(), "query CommitChecks(", map[string]any{"owner": "charmbracelet", "name": "bubbletea", "sha": sha})
	if got.SHA != sha || got.State != core.ChecksFailure || len(got.Runs) != 34 {
		t.Fatalf("checks = %+v, want the 34 of the commit", got)
	}
	i := slices.IndexFunc(got.Runs, func(r core.Check) bool { return r.ID == 106700606263 })
	if i < 0 {
		t.Fatal("no check of the failed windows build")
	}
	if r := got.Runs[i]; r.Conclusion != core.ConclusionFailure || r.JobID != 106700606263 || r.RunID != 35713862348 || r.Annotations != 1 || r.Required {
		t.Errorf("failed check = %+v", r)
	}
}

func TestChecksMissing(t *testing.T) {
	tests := []struct {
		name string
		body string
		read func(c *Client) error
	}{
		{"pull", `{"data":{"repository":{"pullRequest":null}}}`, func(c *Client) error {
			_, err := c.PullChecks(t.Context(), bubbletea, 1)
			return err
		}},
		{"commit", `{"data":{"repository":{"object":null}}}`, func(c *Client) error {
			_, err := c.CommitChecks(t.Context(), bubbletea, "abc")
			return err
		}},
		{"not found", `{"data":{"repository":null},"errors":[{"type":"NOT_FOUND","message":"Could not resolve"}]}`, func(c *Client) error {
			_, err := c.PullChecks(t.Context(), bubbletea, 1)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			if err := tt.read(c); !errors.Is(err, core.ErrNotFound) {
				t.Errorf("read = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestChecksNone(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"repository":{"object":{"oid":"abc","statusCheckRollup":null}}}}`))
	}))
	got, err := c.CommitChecks(t.Context(), bubbletea, "abc")
	if err != nil || got.SHA != "abc" || got.State != core.ChecksNone || got.Runs != nil {
		t.Errorf("CommitChecks = %+v, %v; want no checks", got, err)
	}
}

func TestListAnnotations(t *testing.T) {
	c := serveActions(t, "check_annotations.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/charmbracelet/bubbletea/check-runs/106696399985/annotations", map[string]string{"per_page": "50"})
	})
	page, res, err := c.ListAnnotations(t.Context(), bubbletea, 106696399985, "", 50, Conditional{})
	if err != nil {
		t.Fatalf("ListAnnotations: %v", err)
	}
	want := core.Annotation{
		Path: "clipboard_backend.go", StartLine: 120, EndLine: 120, StartColumn: 34, EndColumn: 34, Level: core.AnnotationFailure,
		Message: "func commandClipboardBackend.Get is unused (unused)",
	}
	if len(page.Items) != 6 || page.Items[1] != want || page.Items[5].Level != core.AnnotationWarning {
		t.Errorf("annotations = %+v, want 6 with %+v", page.Items, want)
	}
	if _, res, err := c.ListAnnotations(t.Context(), bubbletea, 106696399985, "", 50, Conditional{ETag: res.ETag}); err != nil || !res.NotModified {
		t.Errorf("ListAnnotations with the ETag = %+v, %v; want a 304", res, err)
	}
}

// utcCheck returns r with its times in UTC, to compare with ==.
func utcCheck(r core.Check) core.Check {
	r.StartedAt, r.CompletedAt = r.StartedAt.UTC(), r.CompletedAt.UTC()
	return r
}

// BenchmarkDecodePullChecks decodes the rollup of 34 checks.
func BenchmarkDecodePullChecks(b *testing.B) {
	body := benchFixture(b, "graphql_pull_checks.json")
	for b.Loop() {
		var data struct {
			Repository struct {
				PullRequest struct {
					Commits nodes[struct {
						Commit commitChecks `json:"commit"`
					}] `json:"commits"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		decodeData(b, body, &data)
		data.Repository.PullRequest.Commits.Nodes[0].Commit.core()
	}
}
