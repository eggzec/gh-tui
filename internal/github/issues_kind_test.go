package github

import (
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestGetIssueKind(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		number  int
		want    core.NumberKind
		// issue is the issue returned with the kind, if any.
		issue core.Issue
	}{
		{"issue", "issues_get.json", 42, core.KindIssue, func() core.Issue {
			it := wantIssue42
			it.Assignees = nil
			return it
		}()},
		// The issue-shaped body of a pull request isn't returned.
		{"pull request", "issues_get_pull.json", 43, core.KindPull, core.Issue{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := "/repos/octo-org/hello/issues/" + strconv.Itoa(tt.number)
			c := serveIssueFixture(t, tt.fixture, func(r *http.Request) {
				checkIssueRequest(t, r, http.MethodGet, path, nil)
			})
			kind, it, res, err := c.GetIssueKind(t.Context(), issueRepo, tt.number, Conditional{})
			if err != nil {
				t.Fatalf("GetIssueKind: %v", err)
			}
			if kind != tt.want {
				t.Errorf("kind = %q, want %q", kind, tt.want)
			}
			if !equalIssue(it, tt.issue) {
				t.Errorf("issue = %+v, want %+v", it, tt.issue)
			}
			if res.ETag != `W/"e1"` || res.URL == "" {
				t.Errorf("response = %+v, want the ETag and URL of the response", res)
			}
		})
	}
}

func TestGetIssueKindNotModified(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `W/"e1"` {
			t.Errorf("If-None-Match = %q, want the stored ETag", r.Header.Get("If-None-Match"))
		}
		w.Header().Set("ETag", `W/"e1"`)
		w.WriteHeader(http.StatusNotModified)
	}))
	kind, it, res, err := c.GetIssueKind(t.Context(), issueRepo, 42, Conditional{ETag: `W/"e1"`})
	if err != nil {
		t.Fatalf("error = %v, want none on a 304", err)
	}
	if !res.NotModified || kind.Known() || it.Number != 0 {
		t.Errorf("GetIssueKind = %q, %+v, NotModified %v; want a 304 with no kind or issue", kind, it, res.NotModified)
	}
}

func TestGetIssueKindErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		headers map[string]string
		want    error
		refused bool
	}{
		{"not found", http.StatusNotFound, nil, core.ErrNotFound, true},
		{"forbidden", http.StatusForbidden, nil, nil, true},
		// A deleted issue, or any number of a repository with issues
		// turned off.
		{"gone", http.StatusGone, nil, nil, true},
		{"rate limited", http.StatusForbidden, map[string]string{
			"X-RateLimit-Limit": "5000", "X-RateLimit-Remaining": "0", "X-RateLimit-Reset": "1790000000",
		}, core.ErrRateLimited, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"message":"` + http.StatusText(tt.status) + `"}`))
			}))
			kind, it, _, err := c.GetIssueKind(t.Context(), issueRepo, 42, Conditional{})
			if e, ok := errors.AsType[*Error](err); !ok || e.StatusCode != tt.status {
				t.Fatalf("error = %v, want a %d", err, tt.status)
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
			if Refused(err) != tt.refused {
				t.Errorf("Refused = %v, want %v", !tt.refused, tt.refused)
			}
			if kind != "" || it.Number != 0 {
				t.Errorf("GetIssueKind = %q, %+v; want nothing along with the error", kind, it)
			}
		})
	}
}
