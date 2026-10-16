package github

import (
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestReviewerRequests(t *testing.T) {
	const path = "/repos/charmbracelet/bubbletea/pulls/7/requested_reviewers"
	tests := []struct {
		name   string
		method string
		call   func(c *Client) error
		body   string
		status int
	}{
		{
			name: "request both", method: http.MethodPost, status: http.StatusCreated,
			call: func(c *Client) error {
				return c.RequestReviewers(t.Context(), bubbletea, 7, []string{"ana", "bo"}, []string{"core"})
			},
			body: `{"reviewers":["ana","bo"],"team_reviewers":["core"]}`,
		},
		{
			name: "request a user", method: http.MethodPost, status: http.StatusCreated,
			call: func(c *Client) error {
				return c.RequestReviewers(t.Context(), bubbletea, 7, []string{"ana"}, nil)
			},
			body: `{"reviewers":["ana"]}`,
		},
		{
			name: "withdraw both", method: http.MethodDelete, status: http.StatusOK,
			call: func(c *Client) error {
				return c.RemoveReviewers(t.Context(), bubbletea, 7, []string{"ana"}, []string{"core"})
			},
			body: `{"reviewers":["ana"],"team_reviewers":["core"]}`,
		},
		{
			name: "withdraw a team keeps reviewers", method: http.MethodDelete, status: http.StatusOK,
			call: func(c *Client) error {
				return c.RemoveReviewers(t.Context(), bubbletea, 7, nil, []string{"core"})
			},
			body: `{"reviewers":[],"team_reviewers":["core"]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != tt.method || r.URL.Path != path {
					t.Errorf("request = %s %s, want %s %s", r.Method, r.URL.Path, tt.method, path)
				}
				b, _ := io.ReadAll(r.Body)
				if string(b) != tt.body {
					t.Errorf("body = %s, want %s", b, tt.body)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"number":7}`))
			}))
			if err := tt.call(c); err != nil {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestReviewerRequestsRefused(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"message":"Review cannot be requested from pull request author."}`))
	}))
	for name, call := range map[string]func() error{
		"request":  func() error { return c.RequestReviewers(t.Context(), bubbletea, 7, []string{"me"}, nil) },
		"withdraw": func() error { return c.RemoveReviewers(t.Context(), bubbletea, 7, []string{"me"}, nil) },
	} {
		err := call()
		if e, ok := errors.AsType[*Error](err); !ok || e.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s: err = %v, want the 422 returned", name, err)
		}
	}
}

func TestReviewerTeams(t *testing.T) {
	c, reqs := serveFixture(t, "graphql_reviewer_teams.json")

	got, err := c.ReviewerTeams(t.Context(), "charmbracelet", "co", 20)
	if err != nil {
		t.Fatalf("ReviewerTeams: %v", err)
	}
	req := <-reqs
	if req.Query != reviewerTeamsQuery {
		t.Errorf("query = %q, want reviewerTeamsQuery", req.Query)
	}
	if v := req.Variables; v["login"] != "charmbracelet" || v["query"] != "co" || v["first"] != float64(20) {
		t.Errorf("variables = %v, want login, query co and first 20", v)
	}
	want := []core.Team{{Name: "Core", Slug: "core"}, {Name: "Core Docs", Slug: "core-docs"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("teams = %+v, want %+v", got, want)
	}
}

func TestReviewerTeamsRefused(t *testing.T) {
	c, _ := serveFixture(t, "org_teams_outsider.json")
	if _, err := c.ReviewerTeams(t.Context(), "github", "", 20); !errors.Is(err, core.ErrForbidden) {
		t.Errorf("outsider: err = %v, want ErrForbidden", err)
	}
	c, _ = serveFixture(t, "org_null.json")
	if _, err := c.ReviewerTeams(t.Context(), "nope", "", 20); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("no organization: err = %v, want ErrNotFound", err)
	}
}

// A token without read:org is refused, whether GitHub answers with the error
// alone or, for the non-null teams, with the data that is left and the error
// on the field below the root.
func TestReviewerTeamsScopeRefused(t *testing.T) {
	for _, fixture := range []string{"org_teams_scope_root.json", "org_teams_scope_below.json"} {
		c, _ := serveFixture(t, fixture)
		_, err := c.ReviewerTeams(t.Context(), "github", "", 20)
		if !errors.Is(err, core.ErrForbidden) {
			t.Errorf("%s: err = %v, want ErrForbidden", fixture, err)
		}
		if errors.Is(err, core.ErrNotFound) {
			t.Errorf("%s: err = %v matches ErrNotFound", fixture, err)
		}
	}
}
