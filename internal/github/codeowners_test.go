package github

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestGetCodeowners(t *testing.T) {
	const base = "0123456789abcdef0123456789abcdef01234567"
	tests := []struct {
		name    string
		fixture string
		want    core.CodeownersFile
	}{
		{"the .github one wins over the others", "graphql_codeowners_all.json", core.CodeownersFile{
			Path: ".github/CODEOWNERS",
			Text: "# in .github\n*       @octo/reviewers\n/docs/  docs@example.com\n",
		}},
		{"a blob without text is skipped", "graphql_codeowners_root.json", core.CodeownersFile{Path: "docs/CODEOWNERS", Text: "* @docs-owner\n"}},
		{"none", "graphql_codeowners_none.json", core.CodeownersFile{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, reqs := serveFixture(t, tt.fixture)
			got, err := c.GetCodeowners(t.Context(), octoHello, base)
			if err != nil {
				t.Fatalf("GetCodeowners: %v", err)
			}
			if got != tt.want {
				t.Errorf("file = %+v, want %+v", got, tt.want)
			}
			req := <-reqs
			if req.Query != codeownersQuery {
				t.Errorf("query = %q, want codeownersQuery", req.Query)
			}
			want := map[string]any{
				"owner": "octo", "name": "hello",
				"github": base + ":.github/CODEOWNERS",
				"root":   base + ":CODEOWNERS",
				"docs":   base + ":docs/CODEOWNERS",
			}
			if len(req.Variables) != len(want) {
				t.Errorf("variables = %v, want %v", req.Variables, want)
			}
			for k, v := range want {
				if req.Variables[k] != v {
					t.Errorf("variable %s = %v, want %v", k, req.Variables[k], v)
				}
			}
		})
	}
}

func TestGetCodeownersErrors(t *testing.T) {
	c, _ := serveFixture(t, "graphql_codeowners_missing.json")
	if _, err := c.GetCodeowners(t.Context(), octoHello, "abc"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("a missing repository gave %v, want core.ErrNotFound", err)
	}
	// Without a commit the expressions would name the work tree's files.
	c = newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("a request was sent without a commit")
	}))
	if _, err := c.GetCodeowners(t.Context(), octoHello, ""); err == nil {
		t.Error("no commit gave no error")
	}
}

func TestListTeamMembers(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "team_members.json"))
	if err != nil {
		t.Fatal(err)
	}
	var path string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.EscapedPath() + "?" + r.URL.RawQuery
		_, _ = w.Write(body)
	}))
	got, err := c.ListTeamMembers(t.Context(), "octo", "core-team")
	if err != nil {
		t.Fatalf("ListTeamMembers: %v", err)
	}
	if want := "/orgs/octo/teams/core-team/members?per_page=100"; path != want {
		t.Errorf("request = %s, want %s", path, want)
	}
	if len(got.Items) != 2 || got.Items[0].Login != "octocat" || got.Items[1].Login != "hubot" || got.Truncated || got.Next != "" {
		t.Errorf("page = %+v, want octocat and hubot", got)
	}
}

// membersServer serves total members of a team in pages of 100, linked as
// GitHub does, and counts the requests.
func membersServer(t *testing.T, total int) (*Client, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	var base string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		if base == "" {
			base = "http://" + r.Host
		}
		page := 1
		if p := r.URL.Query().Get("page"); p != "" {
			page, _ = strconv.Atoi(p)
		}
		first := (page - 1) * 100
		count := min(100, total-first)
		if first+count < total {
			w.Header().Set("Link", fmt.Sprintf(`<%s/orgs/octo/teams/t/members?per_page=100&page=%d>; rel="next"`, base, page+1))
		}
		items := make([]string, max(count, 0))
		for i := range items {
			items[i] = fmt.Sprintf(`{"login":"user%d"}`, first+i)
		}
		_, _ = io.WriteString(w, "["+strings.Join(items, ",")+"]")
	}))
	return c, &n
}

func TestListTeamMembersPages(t *testing.T) {
	tests := []struct {
		name      string
		total     int
		items     int
		truncated bool
		requests  int32
	}{
		{"three pages", 230, 230, false, 3},
		{"exactly the limit", MaxTeamMembers, MaxTeamMembers, false, 5},
		{"one over the limit", MaxTeamMembers + 1, MaxTeamMembers, true, 5},
		{"far over the limit stops reading", 2000, MaxTeamMembers, true, 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, n := membersServer(t, tt.total)
			got, err := c.ListTeamMembers(t.Context(), "octo", "t")
			if err != nil {
				t.Fatalf("ListTeamMembers: %v", err)
			}
			if len(got.Items) != tt.items || got.Truncated != tt.truncated || got.Next != "" {
				t.Errorf("got %d members, Truncated %v, Next %q; want %d, %v", len(got.Items), got.Truncated, got.Next, tt.items, tt.truncated)
			}
			if got.Items[0].Login != "user0" || got.Items[tt.items-1].Login != "user"+strconv.Itoa(tt.items-1) {
				t.Errorf("members run from %s to %s, want user0 to user%d", got.Items[0].Login, got.Items[tt.items-1].Login, tt.items-1)
			}
			if n.Load() != tt.requests {
				t.Errorf("requests = %d, want %d", n.Load(), tt.requests)
			}
		})
	}
}

func TestListTeamMembersNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))
	if _, err := c.ListTeamMembers(t.Context(), "octo", "t"); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("a team the token can't see gave %v, want core.ErrNotFound", err)
	}
}
