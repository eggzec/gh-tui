package github

import (
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestGetRelease(t *testing.T) {
	body := fixture(t, "releases_get.json")
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/repos/charmbracelet/glow/releases/368759772" {
			t.Errorf("request = %s %s, want GET /repos/charmbracelet/glow/releases/368759772", r.Method, r.URL.Path)
		}
		if r.Header.Get("If-None-Match") == `W/"r1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `W/"r1"`)
		_, _ = w.Write(body)
	}))
	repo := core.RepoRef{Owner: "charmbracelet", Name: "glow"}

	got, res, err := c.GetRelease(t.Context(), repo, 368759772, Conditional{})
	if err != nil {
		t.Fatalf("GetRelease: %v", err)
	}
	if res.ETag != `W/"r1"` {
		t.Errorf("ETag = %q, want the response's", res.ETag)
	}
	if got.ID != 368759772 || got.Tag != "v3.0.0" || got.Name != "v3.0.0" || got.Author.Login != "github-actions[bot]" ||
		got.URL != "https://github.com/charmbracelet/glow/releases/tag/v3.0.0" || got.Draft || got.Prerelease ||
		!got.PublishedAt.Equal(time.Date(2026, 8, 11, 18, 9, 45, 0, time.UTC)) {
		t.Errorf("release = %+v", got)
	}
	if got.Body == "" {
		t.Error("release has no body")
	}
	wantAssets := []core.ReleaseAsset{
		{Name: "checksums.txt", Size: 5300, Downloads: 12662, URL: "https://github.com/charmbracelet/glow/releases/download/v3.0.0/checksums.txt"},
		{Name: "checksums.txt.sigstore.json", Size: 10256, Downloads: 9183, URL: "https://github.com/charmbracelet/glow/releases/download/v3.0.0/checksums.txt.sigstore.json"},
		{Name: "glow-3.0.0-1.aarch64.rpm", Size: 6200748, Downloads: 244, URL: "https://github.com/charmbracelet/glow/releases/download/v3.0.0/glow-3.0.0-1.aarch64.rpm"},
	}
	if !slices.Equal(got.Assets, wantAssets) {
		t.Errorf("assets = %+v, want %+v", got.Assets, wantAssets)
	}

	_, res, err = c.GetRelease(t.Context(), repo, 368759772, Conditional{ETag: res.ETag})
	if err != nil || !res.NotModified {
		t.Errorf("conditional GetRelease = %+v, %v, want not modified", res, err)
	}
}

func TestGetReleaseNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Not Found"}`))
	}))
	_, _, err := c.GetRelease(t.Context(), core.RepoRef{Owner: "o", Name: "r"}, 1, Conditional{})
	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want core.ErrNotFound", err)
	}
}
