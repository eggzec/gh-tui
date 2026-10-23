package github

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

var previewRepo = core.RepoRef{Owner: "o", Name: "r"}

func TestRepoReadme(t *testing.T) {
	c, _ := serveReadmes(t, map[string]string{"/repos/o/r/readme": "# R"}, nil)

	got, err := c.RepoReadme(t.Context(), previewRepo)
	if err != nil || got != "# R" {
		t.Errorf("RepoReadme = %q, %v, want # R", got, err)
	}
	got, err = c.RepoReadme(t.Context(), core.RepoRef{Owner: "o", Name: "none"})
	if err != nil || got != "" {
		t.Errorf("RepoReadme of a repository without one = %q, %v, want empty and no error", got, err)
	}
}

func TestRepoReadmeForbidden(t *testing.T) {
	c, _ := serveReadmes(t, nil, map[string]int{"/repos/o/r/readme": http.StatusForbidden})
	if _, err := c.RepoReadme(t.Context(), previewRepo); err == nil {
		t.Error("RepoReadme error = nil, want the refusal")
	}
}

func TestLatestRelease(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			_, _ = io.WriteString(w, `{"id":7,"tag_name":"v1.2.0","published_at":"2026-03-04T05:06:07Z"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
		}
	}))

	got, ok, err := c.LatestRelease(t.Context(), previewRepo)
	if err != nil || !ok || got.Tag != "v1.2.0" || !got.PublishedAt.Equal(time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)) {
		t.Errorf("LatestRelease = %+v, %v, %v, want v1.2.0 published 2026-03-04", got, ok, err)
	}
	_, ok, err = c.LatestRelease(t.Context(), core.RepoRef{Owner: "o", Name: "none"})
	if err != nil || ok {
		t.Errorf("LatestRelease without a release = %v, %v, want none and no error", ok, err)
	}
}

func TestOpenCounts(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			t.Errorf("request = %s %s, want POST /graphql", r.Method, r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":{"repository":{"issues":{"totalCount":12},"pullRequests":{"totalCount":3}}}}`)
	}))

	issues, pulls, err := c.OpenCounts(t.Context(), previewRepo)
	if err != nil || issues != 12 || pulls != 3 {
		t.Errorf("OpenCounts = %d, %d, %v, want 12, 3", issues, pulls, err)
	}
}
