package github

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestListIssueDependencies(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkIssueRequest(t, r, "GET", "/repos/octo-org/hello/issues/7/dependencies/blocked_by", map[string]string{"per_page": "100"})
		_, _ = w.Write([]byte(`[
			{"number":3,"title":"Same repo","state":"open","html_url":"https://github.com/octo-org/hello/issues/3","repository_url":"https://api.github.com/repos/octo-org/hello"},
			{"number":9,"title":"Other repo","state":"closed","state_reason":"completed","html_url":"https://github.com/acme/lib/pull/9","repository_url":"https://api.github.com/repos/acme/lib","pull_request":{}}
		]`))
	}))
	got, err := c.ListIssueDependencies(context.Background(), issueRepo, 7, "blocked_by")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d dependencies, want 2", len(got))
	}
	if got[0].Repo != issueRepo || got[0].Number != 3 || got[0].IsPullRequest {
		t.Errorf("first = %+v", got[0])
	}
	if want := (core.RepoRef{Owner: "acme", Name: "lib"}); got[1].Repo != want || !got[1].IsPullRequest || got[1].State != core.StateClosed {
		t.Errorf("second = %+v", got[1])
	}
}

func TestListIssueDependenciesNotFound(t *testing.T) {
	t.Parallel()
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	_, err := c.ListIssueDependencies(context.Background(), issueRepo, 7, "blocking")
	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("err = %v, want core.ErrNotFound", err)
	}
}
