package github

import (
	"errors"
	"net/http"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestListReleasesPages(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/releases" {
			t.Errorf("path = %s, want /repos/o/r/releases", r.URL.Path)
		}
		w.Header().Set("ETag", `W/"l"`)
		switch r.URL.Query().Get("page") {
		case "":
			if r.URL.Query().Get("per_page") != "2" {
				t.Errorf("per_page = %q, want 2", r.URL.Query().Get("per_page"))
			}
			w.Header().Set("Link", `<http://`+r.Host+r.URL.Path+`?per_page=2&page=2>; rel="next"`)
			_, _ = w.Write([]byte(`[{"id":2,"tag_name":"v2","target_commitish":"main","assets":[{"id":21,"name":"a.zip","size":3,"download_count":4}]},{"id":1,"tag_name":"v1"}]`))
		case "2":
			_, _ = w.Write([]byte(`[{"id":0,"tag_name":"v0","draft":true}]`))
		}
	}))
	repo := core.RepoRef{Owner: "o", Name: "r"}

	first, res, err := c.ListReleases(t.Context(), repo, "", 2, Conditional{})
	if err != nil {
		t.Fatalf("ListReleases: %v", err)
	}
	if len(first.Items) != 2 || first.Items[0].Tag != "v2" || first.Items[0].Target != "main" || first.Next == "" || res.ETag != `W/"l"` {
		t.Fatalf("first page = %+v, %+v", first, res)
	}
	if a := first.Items[0].Assets; len(a) != 1 || a[0].ID != 21 || a[0].Downloads != 4 {
		t.Errorf("assets = %+v, want asset 21 with 4 downloads", a)
	}

	second, _, err := c.ListReleases(t.Context(), repo, first.Next, 2, Conditional{})
	if err != nil {
		t.Fatalf("ListReleases next: %v", err)
	}
	if len(second.Items) != 1 || !second.Items[0].Draft || second.Next != "" {
		t.Errorf("second page = %+v, want one draft and no next", second)
	}
}

func TestListReleasesNotModified(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") != `W/"l"` {
			t.Errorf("If-None-Match = %q, want the ETag", r.Header.Get("If-None-Match"))
		}
		w.WriteHeader(http.StatusNotModified)
	}))
	page, res, err := c.ListReleases(t.Context(), core.RepoRef{Owner: "o", Name: "r"}, "", 0, Conditional{ETag: `W/"l"`})
	if err != nil || !res.NotModified || len(page.Items) != 0 {
		t.Errorf("ListReleases = %+v, %+v, %v; want not modified", page, res, err)
	}
}

func TestReleaseAssetURL(t *testing.T) {
	const signed = "https://objects.example.com/signed?token=abc"
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/releases/assets/21" || r.Header.Get("Accept") != "application/octet-stream" {
			t.Errorf("request = %s, Accept %q", r.URL.Path, r.Header.Get("Accept"))
		}
		w.Header().Set("Location", signed)
		w.WriteHeader(http.StatusFound)
	}))
	got, err := c.ReleaseAssetURL(t.Context(), core.RepoRef{Owner: "o", Name: "r"}, 21)
	if err != nil || got != signed {
		t.Errorf("ReleaseAssetURL = %q, %v; want %q", got, err, signed)
	}
}

func TestReleaseAssetURLNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	if _, err := c.ReleaseAssetURL(t.Context(), core.RepoRef{Owner: "o", Name: "r"}, 9); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want core.ErrNotFound", err)
	}
}
