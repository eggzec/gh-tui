package github

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestGetUserProfile(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/users/octo":
			_, _ = io.WriteString(w, `{"login":"octo","type":"User","name":"Octo Cat","bio":"Hi","followers":7,"created_at":"2011-01-25T18:44:36Z"}`)
		case "/users/acme":
			_, _ = io.WriteString(w, `{"login":"acme","type":"Organization","followers":2}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
		}
	}))

	got, err := c.GetUserProfile(t.Context(), "octo")
	want := core.UserProfile{Login: "octo", Name: "Octo Cat", Bio: "Hi", Followers: 7, CreatedAt: time.Date(2011, 1, 25, 18, 44, 36, 0, time.UTC)}
	if err != nil || got.Login != want.Login || got.Kind != core.OwnerUser || got.Name != want.Name || got.Bio != want.Bio ||
		got.Followers != 7 || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("GetUserProfile = %+v, %v, want %+v", got, err, want)
	}
	if org, err := c.GetUserProfile(t.Context(), "acme"); err != nil || org.Kind != core.OwnerOrg {
		t.Errorf("GetUserProfile of an organization = %+v, %v, want kind org", org, err)
	}
	if _, err := c.GetUserProfile(t.Context(), "none"); err == nil {
		t.Error("GetUserProfile of a missing account = nil error, want one")
	}
}

func TestCommitPatch(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != diffAccept {
			t.Errorf("Accept = %q, want %q", got, diffAccept)
		}
		switch r.URL.Path {
		case "/repos/o/r/commits/small":
			_, _ = io.WriteString(w, "diff --git a/x b/x\n+hi\n")
		case "/repos/o/r/commits/big":
			_, _ = io.WriteString(w, strings.Repeat("+", core.MaxCommitPatch+1))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
		}
	}))
	repo := core.RepoRef{Owner: "o", Name: "r"}

	got, big, err := c.CommitPatch(t.Context(), repo, "small")
	if err != nil || big || got != "diff --git a/x b/x\n+hi\n" {
		t.Errorf("CommitPatch = %q, %v, %v, want the patch", got, big, err)
	}
	got, big, err = c.CommitPatch(t.Context(), repo, "big")
	if err != nil || !big || got != "" {
		t.Errorf("CommitPatch over the cap = %d bytes, %v, %v, want none, too large and no error", len(got), big, err)
	}
	if _, _, err := c.CommitPatch(t.Context(), repo, "none"); err == nil {
		t.Error("CommitPatch of a missing commit = nil error, want one")
	}
}
