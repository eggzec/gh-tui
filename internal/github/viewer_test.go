package github

import (
	"errors"
	"io"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestViewerHeader(t *testing.T) {
	c, reqs := serveFixture(t, "viewer_header.json")

	got, err := c.ViewerHeader(t.Context())
	if err != nil {
		t.Fatalf("ViewerHeader: %v", err)
	}

	req := <-reqs
	if req.Query != viewerHeaderQuery {
		t.Errorf("query = %q, want viewerHeaderQuery", req.Query)
	}
	if v := req.Variables; v["pinned"] != 6.0 || v["orgs"] != 100.0 {
		t.Errorf("variables = %v, want pinned 6 and orgs 100", v)
	}
	want := core.Header{
		Profile: core.Profile{
			Login:     "octocat",
			Name:      "The Octocat",
			Company:   "@github",
			Location:  "San Francisco",
			Website:   "https://github.blog",
			URL:       "https://github.com/octocat",
			AvatarURL: "https://avatars.githubusercontent.com/u/583231?u=a59fef2a493e2b67dd13754231daf220c82ba84d&v=4",
			Followers: 24214,
			Following: 9,
			Repos:     8,
			Status:    core.Status{Emoji: ":octocat:", Message: "Shipping the dashboard", Busy: true},
			CreatedAt: time.Date(2011, 1, 25, 18, 44, 36, 0, time.UTC),
		},
		Pinned: []core.Repo{{
			ID:            "MDEwOlJlcG9zaXRvcnkxMzAwMTky",
			Ref:           core.RepoRef{Owner: "octocat", Name: "Spoon-Knife"},
			Description:   "This repo is for demonstration purposes only.",
			DefaultBranch: "main",
			Language:      "HTML",
			LanguageColor: "#e34c26",
			Stars:         14056,
			UpdatedAt:     time.Date(2026, 9, 24, 8, 23, 12, 0, time.UTC),
			URL:           "https://github.com/octocat/Spoon-Knife",
		}, {
			ID:            "MDEwOlJlcG9zaXRvcnkxMjk2MjY5",
			Ref:           core.RepoRef{Owner: "octocat", Name: "Hello-World"},
			Description:   "My first repository on GitHub!",
			DefaultBranch: "master",
			Stars:         3824,
			UpdatedAt:     time.Date(2026, 9, 24, 10, 49, 42, 0, time.UTC),
			URL:           "https://github.com/octocat/Hello-World",
		}},
		Orgs: []core.Org{
			{Login: "github", Name: "GitHub", URL: "https://github.com/github"},
			{Login: "octo-org", URL: "https://github.com/octo-org"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("header = %+v\nwant %+v", got, want)
	}
}

func TestViewerHeaderWithoutStatus(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"viewer":{"login":"octocat","status":null,"pinnedItems":{"nodes":[]},"organizations":{"nodes":[]}}}}`)
	}))

	got, err := c.ViewerHeader(t.Context())
	if err != nil {
		t.Fatalf("ViewerHeader: %v", err)
	}
	if got.Profile.Login != "octocat" || got.Profile.Status != (core.Status{}) || got.Pinned != nil || got.Orgs != nil {
		t.Errorf("header = %+v, want octocat with no status, pins or orgs", got)
	}
}

func TestViewerHeaderUnauthorized(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Bad credentials"}`)
	}))

	if _, err := c.ViewerHeader(t.Context()); !errors.Is(err, core.ErrUnauthorized) {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

// The logging transport names each GraphQL request after its operation.
func TestDashboardOperations(t *testing.T) {
	for query, want := range map[string]string{
		viewerHeaderQuery:        "ViewerHeader",
		viewerWorkQuery:          "ViewerWork",
		viewerContributionsQuery: "ViewerContributions",
	} {
		if got := operation(query); got != want {
			t.Errorf("operation = %q, want %q", got, want)
		}
	}
}
