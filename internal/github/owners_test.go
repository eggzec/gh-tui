package github

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestOwnerHeaderUser(t *testing.T) {
	c, reqs := serveFixture(t, "owner_header_user.json")

	got, err := c.OwnerHeader(t.Context(), "octocat")
	if err != nil {
		t.Fatalf("OwnerHeader: %v", err)
	}
	req := <-reqs
	if req.Query != ownerHeaderQuery {
		t.Errorf("query = %q, want ownerHeaderQuery", req.Query)
	}
	if v := req.Variables; len(v) != 2 || v["login"] != "octocat" || v["pinned"] != 6.0 {
		t.Errorf("variables = %v, want login octocat and pinned 6", v)
	}
	want := core.Profile{
		Login:     "octocat",
		Name:      "The Octocat",
		Bio:       "Mona's friend",
		Company:   "@github",
		Location:  "San Francisco",
		Website:   "https://github.blog",
		URL:       "https://github.com/octocat",
		AvatarURL: "https://avatars.githubusercontent.com/u/583231?v=4",
		Followers: 24214,
		Following: 9,
		Repos:     8,
		Status:    core.Status{Emoji: ":octocat:", Message: "Shipping"},
		CreatedAt: time.Date(2011, 1, 25, 18, 44, 36, 0, time.UTC),
	}
	if got.Profile != want {
		t.Errorf("profile = %+v\nwant %+v", got.Profile, want)
	}
	if got.Kind != core.OwnerUser || got.ID != "MDQ6VXNlcjU4MzIzMQ==" || got.Pronouns != "they/them" || got.Stars != 3 || got.Twitter != "github" {
		t.Errorf("owner = %+v, want user octocat with pronouns, 3 stars and a twitter name", got)
	}
	if rel := (core.Relation{Following: true, CanFollow: true}); got.Viewer != rel {
		t.Errorf("relation = %+v, want %+v", got.Viewer, rel)
	}
	if len(got.Pinned) != 1 || got.Pinned[0].Ref != (core.RepoRef{Owner: "octocat", Name: "Spoon-Knife"}) || got.HiddenPins {
		t.Errorf("pinned = %+v, hidden %v; want octocat/Spoon-Knife alone", got.Pinned, got.HiddenPins)
	}
}

func TestOwnerHeaderOrg(t *testing.T) {
	c, _ := serveFixture(t, "owner_header_org.json")

	got, err := c.OwnerHeader(t.Context(), "github")
	if err != nil {
		t.Fatalf("OwnerHeader: %v", err)
	}
	if got.Kind != core.OwnerOrg || got.Profile.Login != "github" || got.Profile.Name != "GitHub" {
		t.Errorf("owner = %+v, want the organization github", got)
	}
	// The description stands in for the bio.
	if got.Profile.Bio != "How people build software." || got.Profile.Repos != 520 || got.Profile.Followers != 0 {
		t.Errorf("profile = %+v, want the description, 520 repos and no follower count", got.Profile)
	}
	if got.Email != "support@github.com" || !got.Verified || got.Members != 2600 || got.Teams != 0 {
		t.Errorf("owner = %+v, want the email, verified, 2600 members and no teams", got)
	}
	if got.Viewer != (core.Relation{}) || len(got.Pinned) != 2 || got.Pinned[1].Language != "" {
		t.Errorf("owner = %+v, want no relation and two pins, the second without a language", got)
	}
}

func TestOwnerHeaderNotFound(t *testing.T) {
	c, _ := serveFixture(t, "owner_header_null.json")

	if _, err := c.OwnerHeader(t.Context(), "nope"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// A pinned repository hidden from the token comes as a null with an error,
// and the rest of the header is kept.
func TestOwnerHeaderPartial(t *testing.T) {
	c, _ := serveFixture(t, "owner_header_partial.json")

	got, err := c.OwnerHeader(t.Context(), "octocat")
	var gqlErr *GraphQLError
	if !errors.As(err, &gqlErr) || errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want a GraphQLError other than ErrNotFound", err)
	}
	if got.Profile.Login != "octocat" || len(got.Pinned) != 1 || got.Pinned[0].Ref.Name != "Spoon-Knife" || !got.HiddenPins {
		t.Errorf("owner = %+v, want octocat with the one visible pin, marked as hiding some", got)
	}
}

// A pin that GitHub failed to read for another reason than the token,
// such as one gone a moment, isn't marked as hidden.
func TestOwnerHeaderPinNotFound(t *testing.T) {
	body := fixtureWithout(t, "owner_header_partial.json", `"type": "FORBIDDEN",`)
	body = []byte(strings.Replace(string(body), `"errors": [
    {`, `"errors": [
    {
      "type": "NOT_FOUND",`, 1))
	if !strings.Contains(string(body), "NOT_FOUND") {
		t.Fatal("the fixture's error has moved")
	}
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))

	got, err := c.OwnerHeader(t.Context(), "octocat")
	if err == nil || got.Profile.Login != "octocat" || len(got.Pinned) != 1 || got.HiddenPins {
		t.Errorf("owner = %+v, %v; want octocat's one pin and the error, unmarked", got, err)
	}
}

func TestOwnerHeaderUnauthorized(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Bad credentials"}`)
	}))

	if _, err := c.OwnerHeader(t.Context(), "octocat"); !errors.Is(err, core.ErrUnauthorized) || errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

func TestUserRepos(t *testing.T) {
	c, reqs := serveFixture(t, "user_repos.json")

	got, err := c.UserRepos(t.Context(), "octocat", core.RepoOrder{Field: core.RepoOrderName, Ascending: true}, 2, "Y3Vyc29y")
	if err != nil {
		t.Fatalf("UserRepos: %v", err)
	}
	req := <-reqs
	if req.Query != userReposQuery {
		t.Errorf("query = %q, want userReposQuery", req.Query)
	}
	v := req.Variables
	if len(v) != 5 || v["login"] != "octocat" || v["first"] != 2.0 || v["after"] != "Y3Vyc29y" || v["field"] != "NAME" || v["direction"] != "ASC" {
		t.Errorf("variables = %v, want octocat, first 2 after Y3Vyc29y, by NAME ASC", v)
	}
	if got.Next != "Y3Vyc29yOjI=" || len(got.Items) != 2 || got.Items[0].Ref != (core.RepoRef{Owner: "octocat", Name: "zeta"}) {
		t.Errorf("page = %+v, want two repos starting with octocat/zeta and a next cursor", got)
	}
}

// Every order field has a GitHub name, and the zero order is the most
// recently updated first.
func TestUserReposOrder(t *testing.T) {
	for f := core.RepoOrderUpdated; f <= core.RepoOrderStars; f++ {
		if repoOrderFields[f] == "" {
			t.Errorf("order field %d has no GitHub name", f)
		}
	}
	c, reqs := serveFixture(t, "user_repos.json")
	if _, err := c.UserRepos(t.Context(), "octocat", core.RepoOrder{}, 100, ""); err != nil {
		t.Fatalf("UserRepos: %v", err)
	}
	if v := (<-reqs).Variables; v["field"] != "UPDATED_AT" || v["direction"] != "DESC" || v["after"] != nil {
		t.Errorf("variables = %v, want UPDATED_AT DESC and a null after", v)
	}
	if _, err := c.UserRepos(t.Context(), "octocat", core.RepoOrder{Field: 99}, 100, ""); err == nil {
		t.Error("an unknown order field gave no error")
	}
}

func TestUserReposNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"user":null},"errors":[{"type":"NOT_FOUND","path":["user"],"message":"Could not resolve to a User with the login of 'nope'."}]}`)
	}))

	if _, err := c.UserRepos(t.Context(), "nope", core.RepoOrder{}, 100, ""); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestUserContributions(t *testing.T) {
	c, reqs := serveFixture(t, "user_contributions.json")

	got, err := c.UserContributions(t.Context(), "octocat")
	if err != nil {
		t.Fatalf("UserContributions: %v", err)
	}
	if req := <-reqs; req.Query != userContributionsQuery || len(req.Variables) != 1 || req.Variables["login"] != "octocat" {
		t.Errorf("request = %q with %v, want userContributionsQuery for octocat", req.Query, req.Variables)
	}
	if got.Total != 4246 || len(got.Weeks) != 53 {
		t.Errorf("calendar = %d contributions in %d weeks, want 4246 in 53", got.Total, len(got.Weeks))
	}
}

func TestUserContributionsNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"user":null},"errors":[{"type":"NOT_FOUND","path":["user"],"message":"Could not resolve to a User with the login of 'nope'."}]}`)
	}))

	if _, err := c.UserContributions(t.Context(), "nope"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// The logging transport names each GraphQL request after its operation.
func TestOwnerOperations(t *testing.T) {
	for query, want := range map[string]string{
		ownerHeaderQuery:       "OwnerHeader",
		userReposQuery:         "UserRepos",
		userContributionsQuery: "UserContributions",
	} {
		if got := operation(query); got != want {
			t.Errorf("operation = %q, want %q", got, want)
		}
	}
}

// fixtureWithout returns testdata/fixture without the text of each of
// cut, as an older server that lacks a field leaves it out of its answer.
func fixtureWithout(t *testing.T, fixture string, cut ...string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, c := range cut {
		if !strings.Contains(s, c) {
			t.Fatalf("%s has no %q", fixture, c)
		}
		s = strings.Replace(s, c, "", 1)
	}
	return []byte(s)
}

// TestOwnerPageOnEnterprise reads what the page of a user and of an
// organization reads from an Enterprise Server, whose GraphQL API is at
// /api/graphql and whose REST API is below /api/v3, and whose schema may
// lack a field that github.com has: the header is sent again without
// it, and reads it as unset, the lists of people read as on github.com,
// an organization's follower count comes from the REST API, and the lists
// of sponsors, which only github.com has, ask nothing.
func TestOwnerPageOnEnterprise(t *testing.T) {
	t.Run("user", func(t *testing.T) {
		srv := &enterpriseServer{t: t, lacks: map[string]string{"pronouns": "User"},
			data: fixtureWithout(t, "owner_header_user.json", `"pronouns": "they/them",`)}
		c := newEnterpriseClient(t, srv)
		for i := range 2 {
			got, err := c.OwnerHeader(t.Context(), "octocat")
			if err != nil {
				t.Fatalf("OwnerHeader %d: %v", i, err)
			}
			if got.Kind != core.OwnerUser || got.Profile.Login != "octocat" || got.Pronouns != "" || got.Stars != 3 || len(got.Pinned) != 1 {
				t.Errorf("OwnerHeader %d = %+v, want octocat without pronouns", i, got)
			}
		}
		if sent := srv.sent(); len(sent) != 3 || strings.Contains(sent[1], "pronouns") || strings.Contains(sent[2], "pronouns") {
			t.Errorf("sent %d queries, want the header, it again without pronouns, and the second read without them", len(sent))
		}
		if _, err := c.OwnerSponsors(t.Context(), "octocat", 2, ""); !errors.Is(err, core.ErrUnsupported) {
			t.Errorf("OwnerSponsors = %v, want ErrUnsupported", err)
		}
		if n := len(srv.sent()); n != 3 {
			t.Errorf("sent %d queries after the sponsors, want none for them", n)
		}
	})
	t.Run("organization", func(t *testing.T) {
		srv := &enterpriseServer{t: t, lacks: map[string]string{"isVerified": "Organization"},
			data: fixtureWithout(t, "owner_header_org.json", `"isVerified": true,`)}
		c := newEnterpriseClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/api/v3/users/github" {
				_, _ = io.WriteString(w, `{"login": "github", "followers": 41000}`)
				return
			}
			srv.ServeHTTP(w, r)
		}))
		got, err := c.OwnerHeader(t.Context(), "github")
		if err != nil {
			t.Fatalf("OwnerHeader: %v", err)
		}
		if got.Kind != core.OwnerOrg || got.Profile.Login != "github" || got.Verified || got.Members != 2600 {
			t.Errorf("OwnerHeader = %+v, want github, unverified, with 2600 members", got)
		}
		if n, err := c.OrgFollowers(t.Context(), "github"); err != nil || n != 41000 {
			t.Errorf("OrgFollowers = %d, %v, want 41000", n, err)
		}
	})
	people := []struct {
		name, fixture string
		list          func(*Client) (core.Page[core.Person], error)
		want          string
	}{
		{"followers", "user_followers.json", func(c *Client) (core.Page[core.Person], error) {
			return c.UserFollowers(t.Context(), "octocat", 2, "")
		}, "hubot"},
		{"organizations", "user_orgs.json", func(c *Client) (core.Page[core.Person], error) {
			return c.UserOrgs(t.Context(), "octocat", 2, "")
		}, "github"},
		{"members", "org_members.json", func(c *Client) (core.Page[core.Person], error) {
			return c.OrgMembers(t.Context(), "github", 3, "")
		}, "mona"},
	}
	for _, tt := range people {
		t.Run(tt.name, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata", tt.fixture))
			if err != nil {
				t.Fatal(err)
			}
			srv := &enterpriseServer{t: t, data: b}
			got, err := tt.list(newEnterpriseClient(t, srv))
			if err != nil || len(got.Items) == 0 || got.Items[0].Login != tt.want {
				t.Errorf("%s = %+v, %v; want the fixture's", tt.name, got, err)
			}
			if n := len(srv.sent()); n != 1 {
				t.Errorf("sent %d queries, want 1", n)
			}
		})
	}
	t.Run("teams", func(t *testing.T) {
		b, err := os.ReadFile(filepath.Join("testdata", "org_teams.json"))
		if err != nil {
			t.Fatal(err)
		}
		got, err := newEnterpriseClient(t, &enterpriseServer{t: t, data: b}).OrgTeams(t.Context(), "github", 2, "")
		if err != nil || len(got.Items) != 2 || got.Items[0].Slug != "core" {
			t.Errorf("OrgTeams = %+v, %v; want the fixture's", got, err)
		}
	})
}
