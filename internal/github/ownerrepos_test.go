package github

import (
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestViewerOwnRepos(t *testing.T) {
	c, reqs := serveFixture(t, "viewer_own_repos.json")

	got, err := c.ViewerOwnRepos(t.Context(), 100, "")
	if err != nil {
		t.Fatalf("ViewerOwnRepos: %v", err)
	}
	req := <-reqs
	if op := operation(req.Query); op != "ViewerOwnRepos" {
		t.Errorf("operation = %q, want ViewerOwnRepos", op)
	}
	if req.Query != viewerOwnReposQuery {
		t.Errorf("query = %q, want viewerOwnReposQuery", req.Query)
	}
	if v := req.Variables; len(v) != 2 || v["first"] != 100.0 || v["after"] != nil {
		t.Errorf("variables = %v, want first 100 and a null after", v)
	}
	if !got.Last() || len(got.Items) != 2 || got.Items[1].Ref != (core.RepoRef{Owner: "octocat", Name: "Spoon-Knife"}) {
		t.Errorf("page = %+v, want the last page of two, ending with octocat/Spoon-Knife", got)
	}
}

func TestOrgRepos(t *testing.T) {
	c, reqs := serveFixture(t, "org_repos.json")

	got, err := c.OrgRepos(t.Context(), "charmbracelet", 2, "Y3Vyc29y")
	if err != nil {
		t.Fatalf("OrgRepos: %v", err)
	}
	req := <-reqs
	if op := operation(req.Query); op != "OrgRepos" {
		t.Errorf("operation = %q, want OrgRepos", op)
	}
	if req.Query != orgReposQuery {
		t.Errorf("query = %q, want orgReposQuery", req.Query)
	}
	if v := req.Variables; len(v) != 3 || v["login"] != "charmbracelet" || v["first"] != 2.0 || v["after"] != "Y3Vyc29y" {
		t.Errorf("variables = %v, want login charmbracelet, first 2 after Y3Vyc29y", v)
	}
	if got.Next != "Y3Vyc29yOnYyOpK0MjAyNi0wOS0yNFQxMToyNToyMVrODeVIaQ==" || len(got.Items) != 2 {
		t.Fatalf("page = %+v, want two repos and a next cursor", got)
	}
	vhs := got.Items[0]
	if vhs.Ref != (core.RepoRef{Owner: "charmbracelet", Name: "vhs"}) || vhs.Language != "Go" || vhs.DefaultBranch != "main" || vhs.UpdatedAt.Before(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("first repo = %+v, want charmbracelet/vhs in Go", vhs)
	}
}

func TestOrgReposNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"organization":null},"errors":[{"type":"NOT_FOUND","path":["organization"],"message":"Could not resolve to an Organization with the login of 'nope'."}]}`))
	}))

	_, err := c.OrgRepos(t.Context(), "nope", 100, "")
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func BenchmarkDecodeOrgRepos(b *testing.B) {
	body := benchFixture(b, "org_repos_100.json")
	for b.Loop() {
		var data struct {
			Organization struct {
				Repositories ownerRepos `json:"repositories"`
			} `json:"organization"`
		}
		decodeData(b, body, &data)
		if p := data.Organization.Repositories.core(); len(p.Items) != 100 {
			b.Fatalf("decoded %d repos, want 100", len(p.Items))
		}
	}
}
