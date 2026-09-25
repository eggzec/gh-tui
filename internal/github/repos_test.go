package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

// gqlRequest is the body of a GraphQL request as the server sees it.
type gqlRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

// serveFixture answers GraphQL requests with testdata/fixture and passes
// each request it receives to the returned channel.
func serveFixture(t *testing.T, fixture string) (c *Client, reqs <-chan gqlRequest) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan gqlRequest, 1)
	c = newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			t.Errorf("request = %s %s, want POST /graphql", r.Method, r.URL.Path)
		}
		var req gqlRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		select {
		case ch <- req:
		default:
			t.Error("more requests than expected")
		}
		_, _ = w.Write(body)
	}))
	return c, ch
}

var ghTUI = core.Repo{
	ID:            "R_kgDOMbT0Qw",
	Ref:           core.RepoRef{Owner: "eggzec", Name: "gh-tui"},
	Description:   "A GitHub client for the terminal",
	DefaultBranch: "main",
	Language:      "Go",
	LanguageColor: "#00ADD8",
	Stars:         42,
	UpdatedAt:     time.Date(2026, 9, 21, 10, 0, 0, 0, time.UTC),
	URL:           "https://github.com/eggzec/gh-tui",
}

func TestListRepos(t *testing.T) {
	c, reqs := serveFixture(t, "repos_list.json")

	got, err := c.ListRepos(t.Context(), 30, "")
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}

	req := <-reqs
	if req.Query != listReposQuery {
		t.Errorf("query = %q, want listReposQuery", req.Query)
	}
	// JSON numbers decode as float64, and the first page has a null cursor.
	if v := req.Variables; len(v) != 2 || v["first"] != 30.0 || v["after"] != nil {
		t.Errorf("variables = %v, want first 30 and a null after", v)
	}

	starred := ghTUI
	starred.Starred = true
	want := core.Page[core.Repo]{
		Items: []core.Repo{starred, {
			ID:        "R_kgDOLx9a1A",
			Ref:       core.RepoRef{Owner: "octo-org", Name: "dotfiles"},
			Private:   true,
			Fork:      true,
			Archived:  true,
			Template:  true,
			Mirror:    true,
			UpdatedAt: time.Date(2026, 8, 2, 8, 30, 0, 0, time.UTC),
			URL:       "https://github.com/octo-org/dotfiles",
		}},
		Next: "Y3Vyc29yOnYyOpK5MjAyNi0wOC0wMlQwODozMDowMCswMDowMM4LfWrU",
	}
	if !slices.Equal(got.Items, want.Items) || got.Next != want.Next {
		t.Errorf("page = %+v\nwant %+v", got, want)
	}
}

func TestListReposLastPage(t *testing.T) {
	c, reqs := serveFixture(t, "repos_list_last.json")

	got, err := c.ListRepos(t.Context(), 50, "Y3Vyc29y")
	if err != nil {
		t.Fatalf("ListRepos: %v", err)
	}
	if v := (<-reqs).Variables; v["first"] != 50.0 || v["after"] != "Y3Vyc29y" {
		t.Errorf("variables = %v, want first 50 after Y3Vyc29y", v)
	}
	if got.Items != nil || !got.Last() {
		t.Errorf("page = %+v, want an empty last page", got)
	}
}

func TestGetRepo(t *testing.T) {
	c, reqs := serveFixture(t, "repos_get.json")

	got, err := c.GetRepo(t.Context(), ghTUI.Ref)
	if err != nil {
		t.Fatalf("GetRepo: %v", err)
	}
	req := <-reqs
	if req.Query != getRepoQuery {
		t.Errorf("query = %q, want getRepoQuery", req.Query)
	}
	if v := req.Variables; len(v) != 2 || v["owner"] != "eggzec" || v["name"] != "gh-tui" {
		t.Errorf("variables = %v, want owner eggzec and name gh-tui", v)
	}
	if got != ghTUI {
		t.Errorf("repo = %+v\nwant %+v", got, ghTUI)
	}
}

func TestGetRepoNotFound(t *testing.T) {
	c, _ := serveFixture(t, "repos_get_not_found.json")

	_, err := c.GetRepo(t.Context(), core.RepoRef{Owner: "eggzec", Name: "missing"})

	if !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
	if _, ok := errors.AsType[*GraphQLError](err); !ok {
		t.Errorf("error = %v, want a *GraphQLError", err)
	}
}

func TestGetRepoNullWithoutErrors(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"repository":null}}`)
	}))

	if _, err := c.GetRepo(t.Context(), ghTUI.Ref); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestReposHTTPError(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Bad credentials","documentation_url":"https://docs.github.com/rest"}`)
	}))

	if _, err := c.ListRepos(t.Context(), 30, ""); !errors.Is(err, core.ErrUnauthorized) {
		t.Errorf("ListRepos error = %v, want ErrUnauthorized", err)
	}
	if _, err := c.GetRepo(t.Context(), ghTUI.Ref); !errors.Is(err, core.ErrUnauthorized) {
		t.Errorf("GetRepo error = %v, want ErrUnauthorized", err)
	}
}

func TestStar(t *testing.T) {
	tests := []struct {
		name   string
		method string
		call   func(*Client, context.Context, core.RepoRef) error
	}{
		{"star", http.MethodPut, (*Client).Star},
		{"unstar", http.MethodDelete, (*Client).Unstar},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != "/user/starred/eggzec/gh-tui" {
					t.Errorf("request = %s %s, want %s /user/starred/eggzec/gh-tui", r.Method, r.URL.Path, tt.method)
				}
				// GitHub asks for an explicit zero length on PUT.
				if tt.method == http.MethodPut && r.Header.Get("Content-Length") != "0" {
					t.Errorf("Content-Length = %q, want 0", r.Header.Get("Content-Length"))
				}
				w.WriteHeader(http.StatusNoContent)
			}))

			if err := tt.call(c, t.Context(), ghTUI.Ref); err != nil {
				t.Errorf("error = %v", err)
			}
		})
	}
}

func TestStarErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   error
	}{
		{"missing repo", http.StatusNotFound, `{"message":"Not Found","documentation_url":"https://docs.github.com/rest/activity/starring#star-a-repository-for-the-authenticated-user"}`, core.ErrNotFound},
		{"bad token", http.StatusUnauthorized, `{"message":"Bad credentials"}`, core.ErrUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))

			if err := c.Star(t.Context(), ghTUI.Ref); !errors.Is(err, tt.want) {
				t.Errorf("Star error = %v, want %v", err, tt.want)
			}
			if err := c.Unstar(t.Context(), ghTUI.Ref); !errors.Is(err, tt.want) {
				t.Errorf("Unstar error = %v, want %v", err, tt.want)
			}
		})
	}
}
