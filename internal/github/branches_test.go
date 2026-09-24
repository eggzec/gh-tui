package github

import (
	"errors"
	"io"
	"net/http"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

var goGH = core.RepoRef{Owner: "cli", Name: "go-gh"}

func TestListBranches(t *testing.T) {
	c := serveIssueFixture(t, "branches_list.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/cli/go-gh/branches", map[string]string{"per_page": "3"})
	})

	got, res, err := c.ListBranches(t.Context(), goGH, "", 3, Conditional{})
	if err != nil {
		t.Fatalf("ListBranches: %v", err)
	}
	want := []core.Branch{
		{Name: "andyfeller/charmbracelet-huh-prompter", SHA: "c84e162986abeec90ac37d5e12ac1ff7b16303a3"},
		{Name: "andyfeller/custom-glamour-style-spike", SHA: "0c30d0f6193cdb5c27a0b5716e7416a8e77c47d0"},
		{Name: "andyfeller/gh_accessible_colors_upstream", SHA: "4fed99b3944feeb04e0b83d69aa556dd092c626b"},
	}
	if !slices.Equal(got.Items, want) || got.Next != "" {
		t.Errorf("page = %+v\nwant %+v and no next", got, want)
	}
	if res.ETag != `W/"e1"` {
		t.Errorf("ETag = %q, want W/\"e1\"", res.ETag)
	}
}

func TestListBranchesPages(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", `<http://`+r.Host+`/repositories/1/branches?per_page=1&page=2>; rel="next"`)
			_, _ = io.WriteString(w, `[{"name":"main","commit":{"sha":"a1"},"protected":true}]`)
		case "2":
			if r.URL.Path != "/repositories/1/branches" {
				t.Errorf("path = %s, want the next link", r.URL.Path)
			}
			_, _ = io.WriteString(w, `[{"name":"dev","commit":{"sha":"b2"},"protected":false}]`)
		}
	}))

	first, _, err := c.ListBranches(t.Context(), goGH, "", 1, Conditional{})
	if err != nil || first.Next == "" || first.Items[0] != (core.Branch{Name: "main", SHA: "a1", Protected: true}) {
		t.Fatalf("first page = %+v, %v", first, err)
	}
	second, _, err := c.ListBranches(t.Context(), goGH, first.Next, 1, Conditional{})
	if err != nil || !second.Last() || second.Items[0].Name != "dev" {
		t.Errorf("second page = %+v, %v; want dev, last", second, err)
	}
}

func TestListBranchesNotModified(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("If-None-Match"); got != `W/"e1"` {
			t.Errorf("If-None-Match = %q, want the cached ETag", got)
		}
		w.WriteHeader(http.StatusNotModified)
	}))

	got, res, err := c.ListBranches(t.Context(), goGH, "", 0, Conditional{ETag: `W/"e1"`})
	if err != nil || !res.NotModified || got.Items != nil {
		t.Errorf("ListBranches = %+v, %+v, %v; want an empty not-modified page", got, res, err)
	}
}

func TestListBranchesNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))

	_, _, err := c.ListBranches(t.Context(), goGH, "", 0, Conditional{})
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("err = %v, want core.ErrNotFound", err)
	}
}
