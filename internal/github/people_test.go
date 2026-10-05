package github

import (
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestUserFollowers(t *testing.T) {
	c, reqs := serveFixture(t, "user_followers.json")

	got, err := c.UserFollowers(t.Context(), "octocat", 2, "")
	if err != nil {
		t.Fatalf("UserFollowers: %v", err)
	}
	req := <-reqs
	if req.Query != userFollowersQuery {
		t.Errorf("query = %q, want userFollowersQuery", req.Query)
	}
	if v := req.Variables; len(v) != 3 || v["login"] != "octocat" || v["first"] != 2.0 || v["after"] != nil {
		t.Errorf("variables = %v, want octocat, first 2 and a null cursor", v)
	}
	want := []core.Person{
		{Login: "hubot", Name: "Hubot", Bio: "Beep"},
		{Login: "monalisa"},
	}
	if len(got.Items) != 2 || got.Items[0] != want[0] || got.Items[1] != want[1] || got.Next != "Y3Vyc29yOjI=" {
		t.Errorf("page = %+v, want %+v and a next cursor", got, want)
	}
}

func TestUserFollowingEmpty(t *testing.T) {
	c, reqs := serveFixture(t, "user_following_empty.json")

	got, err := c.UserFollowing(t.Context(), "octocat", 2, "Y3Vyc29y")
	if err != nil {
		t.Fatalf("UserFollowing: %v", err)
	}
	if req := <-reqs; req.Query != userFollowingQuery || req.Variables["after"] != "Y3Vyc29y" {
		t.Errorf("request = %+v, want userFollowingQuery after Y3Vyc29y", req)
	}
	if len(got.Items) != 0 || got.Next != "" {
		t.Errorf("page = %+v, want an empty last page", got)
	}
}

func TestUserOrgs(t *testing.T) {
	c, reqs := serveFixture(t, "user_orgs.json")

	got, err := c.UserOrgs(t.Context(), "octocat", 2, "")
	if err != nil {
		t.Fatalf("UserOrgs: %v", err)
	}
	if req := <-reqs; req.Query != userOrgsQuery {
		t.Errorf("query = %q, want userOrgsQuery", req.Query)
	}
	want := core.Person{Kind: core.OwnerOrg, Login: "github", Name: "GitHub", Bio: "How people build software."}
	if len(got.Items) != 1 || got.Items[0] != want || got.Next != "" {
		t.Errorf("page = %+v, want only %+v", got, want)
	}
}

func TestUserPeopleNotFound(t *testing.T) {
	for name, list := range map[string]func(*Client) error{
		"followers": func(c *Client) error { _, err := c.UserFollowers(t.Context(), "nope", 2, ""); return err },
		"following": func(c *Client) error { _, err := c.UserFollowing(t.Context(), "nope", 2, ""); return err },
		"orgs":      func(c *Client) error { _, err := c.UserOrgs(t.Context(), "nope", 2, ""); return err },
		"stars":     func(c *Client) error { _, err := c.UserStars(t.Context(), "nope", 2, ""); return err },
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := serveFixture(t, "user_people_null.json")
			if err := list(c); !errors.Is(err, core.ErrNotFound) {
				t.Errorf("error = %v, want ErrNotFound", err)
			}
		})
	}
}

func TestUserFollowersForbidden(t *testing.T) {
	c, _ := serveFixture(t, "user_followers_forbidden.json")

	if _, err := c.UserFollowers(t.Context(), "octocat", 2, ""); !errors.Is(err, core.ErrForbidden) || errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrForbidden", err)
	}
}

func TestOrgMembers(t *testing.T) {
	c, reqs := serveFixture(t, "org_members.json")

	got, err := c.OrgMembers(t.Context(), "github", 3, "")
	if err != nil {
		t.Fatalf("OrgMembers: %v", err)
	}
	if req := <-reqs; req.Query != orgMembersQuery || req.Variables["login"] != "github" {
		t.Errorf("request = %+v, want orgMembersQuery of github", req)
	}
	// An outsider sees no role.
	want := []core.Person{
		{Login: "mona", Name: "Mona", Role: core.MemberRoleAdmin},
		{Login: "hubot", Name: "Hubot", Bio: "Beep", Role: core.MemberRoleMember},
		{Login: "octocat", Role: core.MemberRoleUnknown},
	}
	if len(got.Items) != len(want) {
		t.Fatalf("members = %+v, want %+v", got.Items, want)
	}
	for i := range want {
		if got.Items[i] != want[i] {
			t.Errorf("member %d = %+v, want %+v", i, got.Items[i], want[i])
		}
	}
}

func TestOrgMembersEmpty(t *testing.T) {
	c, _ := serveFixture(t, "org_members_empty.json")

	got, err := c.OrgMembers(t.Context(), "github", 3, "")
	if err != nil || len(got.Items) != 0 || got.Next != "" {
		t.Errorf("OrgMembers = %+v, %v, want an empty last page", got, err)
	}
}

func TestOrgListsNotFound(t *testing.T) {
	c, _ := serveFixture(t, "org_null.json")
	if _, err := c.OrgMembers(t.Context(), "nope", 3, ""); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("OrgMembers error = %v, want ErrNotFound", err)
	}
	c, _ = serveFixture(t, "org_null.json")
	if _, err := c.OrgTeams(t.Context(), "nope", 3, ""); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("OrgTeams error = %v, want ErrNotFound", err)
	}
}

func TestOrgTeams(t *testing.T) {
	c, reqs := serveFixture(t, "org_teams.json")

	got, err := c.OrgTeams(t.Context(), "github", 2, "")
	if err != nil {
		t.Fatalf("OrgTeams: %v", err)
	}
	if req := <-reqs; req.Query != orgTeamsQuery {
		t.Errorf("query = %q, want orgTeamsQuery", req.Query)
	}
	want := []core.Team{
		{Name: "Core", Slug: "core", Description: "The core team", Members: 4, URL: "https://github.com/orgs/github/teams/core"},
		{Name: "Security", Slug: "security", Secret: true, Members: 2, URL: "https://github.com/orgs/github/teams/security"},
	}
	if len(got.Items) != 2 || got.Items[0] != want[0] || got.Items[1] != want[1] || got.Next != "Y3Vyc29yOjI=" {
		t.Errorf("page = %+v, want %+v and a next cursor", got, want)
	}
}

func TestOrgTeamsEmpty(t *testing.T) {
	c, _ := serveFixture(t, "org_teams_empty.json")

	got, err := c.OrgTeams(t.Context(), "github", 2, "")
	if err != nil || len(got.Items) != 0 || got.Next != "" {
		t.Errorf("OrgTeams = %+v, %v, want an empty last page", got, err)
	}
}

// Someone outside the organization sees no teams, which is a refusal, not
// an empty list.
func TestOrgTeamsOutsider(t *testing.T) {
	c, _ := serveFixture(t, "org_teams_outsider.json")

	if _, err := c.OrgTeams(t.Context(), "github", 2, ""); !errors.Is(err, core.ErrForbidden) || !errors.Is(err, errMembersOnly) {
		t.Errorf("error = %v, want the members-only refusal", err)
	}
}

func TestUserStars(t *testing.T) {
	c, reqs := serveFixture(t, "user_stars.json")

	got, err := c.UserStars(t.Context(), "octocat", 1, "")
	if err != nil {
		t.Fatalf("UserStars: %v", err)
	}
	if req := <-reqs; req.Query != userStarsQuery {
		t.Errorf("query = %q, want userStarsQuery", req.Query)
	}
	if len(got.Items) != 1 || got.Items[0].Ref != (core.RepoRef{Owner: "octocat", Name: "zeta"}) || got.Next != "c3Rhcg==" {
		t.Errorf("page = %+v, want octocat/zeta and a next cursor", got)
	}
}

func TestUserStarsEmpty(t *testing.T) {
	c, _ := serveFixture(t, "user_stars_empty.json")

	got, err := c.UserStars(t.Context(), "octocat", 1, "")
	if err != nil || len(got.Items) != 0 || got.Next != "" {
		t.Errorf("UserStars = %+v, %v, want an empty last page", got, err)
	}
}

func TestOwnerSponsors(t *testing.T) {
	c, reqs := serveFixture(t, "owner_sponsors.json")

	got, err := c.OwnerSponsors(t.Context(), "octocat", 2, "")
	if err != nil {
		t.Fatalf("OwnerSponsors: %v", err)
	}
	if req := <-reqs; req.Query != ownerSponsorsQuery {
		t.Errorf("query = %q, want ownerSponsorsQuery", req.Query)
	}
	want := []core.Person{
		{Login: "hubot", Name: "Hubot", Bio: "Beep"},
		{Kind: core.OwnerOrg, Login: "github", Name: "GitHub", Bio: "How people build software."},
	}
	if len(got.Items) != 2 || got.Items[0] != want[0] || got.Items[1] != want[1] {
		t.Errorf("page = %+v, want %+v", got, want)
	}
}

func TestOwnerSponsoringEmpty(t *testing.T) {
	c, reqs := serveFixture(t, "owner_sponsoring_empty.json")

	got, err := c.OwnerSponsoring(t.Context(), "octocat", 2, "")
	if err != nil || len(got.Items) != 0 || got.Next != "" {
		t.Errorf("OwnerSponsoring = %+v, %v, want an empty last page", got, err)
	}
	if req := <-reqs; req.Query != ownerSponsoringQuery {
		t.Errorf("query = %q, want ownerSponsoringQuery", req.Query)
	}
}

func TestOwnerSponsorsNotFound(t *testing.T) {
	c, _ := serveFixture(t, "owner_sponsors_null.json")

	if _, err := c.OwnerSponsors(t.Context(), "nope", 2, ""); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

// An Enterprise Server has no GitHub Sponsors, so nothing is asked.
func TestOwnerSponsorsEnterprise(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		t.Errorf("request %s %s, want none", r.Method, r.URL.Path)
	}))
	c.enterprise.Store(true)

	if _, err := c.OwnerSponsors(t.Context(), "octocat", 2, ""); !errors.Is(err, core.ErrUnsupported) {
		t.Errorf("error = %v, want ErrUnsupported", err)
	}
}

// serveReadmes answers each README path with its status and body, and
// any other with a 404. A request with the ETag "r1" gets a 304.
func serveReadmes(t *testing.T, readmes map[string]string, status map[string]int) (c *Client, asked *atomic.Int32) {
	t.Helper()
	asked = new(atomic.Int32)
	c = newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		asked.Add(1)
		if r.Header.Get("Accept") != rawAccept {
			t.Errorf("Accept = %q, want %q", r.Header.Get("Accept"), rawAccept)
		}
		if code, ok := status[r.URL.Path]; ok {
			w.WriteHeader(code)
			_, _ = io.WriteString(w, `{"message":"Resource not accessible"}`)
			return
		}
		body, ok := readmes[r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"message":"Not Found"}`)
			return
		}
		w.Header().Set("ETag", `"r1"`)
		if r.Header.Get("If-None-Match") == `"r1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = io.WriteString(w, body)
	}))
	return c, asked
}

func TestProfileReadmeUser(t *testing.T) {
	c, asked := serveReadmes(t, map[string]string{"/repos/octocat/octocat/readme": "# Hi"}, nil)

	got, res, err := c.ProfileReadme(t.Context(), "octocat", core.OwnerUser, false, Conditional{})
	if err != nil {
		t.Fatalf("ProfileReadme: %v", err)
	}
	want := core.Readme{Markdown: "# Hi", Source: core.RepoRef{Owner: "octocat", Name: "octocat"}}
	if got != want || res.ETag != `"r1"` {
		t.Errorf("README = %+v, ETag %q, want %+v and r1", got, res.ETag, want)
	}
	if asked.Load() != 1 {
		t.Errorf("asked %d times, want once, of the user's repository", asked.Load())
	}
	// Revalidated, it is not modified.
	got, res, err = c.ProfileReadme(t.Context(), "octocat", core.OwnerUser, false, Conditional{ETag: res.ETag})
	if err != nil || !res.NotModified || got != (core.Readme{}) {
		t.Errorf("revalidated = %+v, %+v, %v, want not modified", got, res, err)
	}
}

func TestProfileReadmeOrg(t *testing.T) {
	readmes := map[string]string{
		"/repos/github/.github/readme/profile":         "public",
		"/repos/github/.github-private/readme/profile": "members",
	}
	c, asked := serveReadmes(t, readmes, nil)

	got, _, err := c.ProfileReadme(t.Context(), "github", core.OwnerOrg, false, Conditional{})
	if err != nil || got.Markdown != "public" || got.MembersOnly {
		t.Errorf("outsider's README = %+v, %v, want the public one", got, err)
	}
	if asked.Load() != 1 {
		t.Errorf("asked %d times, want once, of .github", asked.Load())
	}
	got, _, err = c.ProfileReadme(t.Context(), "github", core.OwnerOrg, true, Conditional{})
	want := core.Readme{Markdown: "members", Source: core.RepoRef{Owner: "github", Name: ".github-private"}, MembersOnly: true}
	if err != nil || got != want {
		t.Errorf("member's README = %+v, %v, want %+v", got, err, want)
	}
}

// A member of an organization without a members-only README gets the
// public one.
func TestProfileReadmeMemberFallsBack(t *testing.T) {
	c, asked := serveReadmes(t, map[string]string{"/repos/github/.github/readme/profile": "public"}, nil)

	got, _, err := c.ProfileReadme(t.Context(), "github", core.OwnerOrg, true, Conditional{})
	if err != nil || got.Markdown != "public" || got.MembersOnly {
		t.Errorf("README = %+v, %v, want the public one", got, err)
	}
	if asked.Load() != 2 {
		t.Errorf("asked %d times, want twice, .github-private then .github", asked.Load())
	}
}

func TestProfileReadmeNone(t *testing.T) {
	c, _ := serveReadmes(t, nil, nil)

	got, _, err := c.ProfileReadme(t.Context(), "github", core.OwnerOrg, true, Conditional{})
	if err != nil || got != (core.Readme{}) {
		t.Errorf("README = %+v, %v, want none and no error", got, err)
	}
}

func TestProfileReadmeEmpty(t *testing.T) {
	c, _ := serveReadmes(t, map[string]string{"/repos/octocat/octocat/readme": ""}, nil)

	got, _, err := c.ProfileReadme(t.Context(), "octocat", core.OwnerUser, false, Conditional{})
	if err != nil || got.Markdown != "" || got.Source.Name != "octocat" {
		t.Errorf("README = %+v, %v, want an empty one from octocat/octocat", got, err)
	}
}

func TestProfileReadmeForbidden(t *testing.T) {
	c, _ := serveReadmes(t, nil, map[string]int{"/repos/github/.github-private/readme/profile": http.StatusForbidden})

	if _, _, err := c.ProfileReadme(t.Context(), "github", core.OwnerOrg, true, Conditional{}); !errors.Is(err, core.ErrForbidden) {
		t.Errorf("error = %v, want ErrForbidden", err)
	}
}

func serveFollowers(t *testing.T, code int, body string) *Client {
	t.Helper()
	return newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/users/github" {
			t.Errorf("request = %s %s, want GET /users/github", r.Method, r.URL.Path)
		}
		w.WriteHeader(code)
		_, _ = io.WriteString(w, body)
	}))
}

func TestOrgFollowers(t *testing.T) {
	c := serveFollowers(t, http.StatusOK, `{"login":"github","type":"Organization","followers":51234}`)
	if n, err := c.OrgFollowers(t.Context(), "github"); err != nil || n != 51234 {
		t.Errorf("OrgFollowers = %d, %v, want 51234", n, err)
	}
}

func TestOrgFollowersNone(t *testing.T) {
	c := serveFollowers(t, http.StatusOK, `{"login":"github","followers":0}`)
	if n, err := c.OrgFollowers(t.Context(), "github"); err != nil || n != 0 {
		t.Errorf("OrgFollowers = %d, %v, want 0", n, err)
	}
}

func TestOrgFollowersErrors(t *testing.T) {
	for code, want := range map[int]error{http.StatusNotFound: core.ErrNotFound, http.StatusForbidden: core.ErrForbidden} {
		c := serveFollowers(t, code, `{"message":"no"}`)
		if _, err := c.OrgFollowers(t.Context(), "github"); !errors.Is(err, want) {
			t.Errorf("status %d: error = %v, want %v", code, err, want)
		}
	}
}
