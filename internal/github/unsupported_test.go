package github

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// missingFieldError is GitHub's answer to a query that selects field of
// typ, which its schema lacks, at the place in query where it is.
func missingFieldError(query, typ, field string) string {
	loc := regexp.MustCompile(`\b` + field + `\b`).FindStringIndex(query)
	line := strings.Count(query[:loc[0]], "\n") + 1
	column := loc[0] - strings.LastIndexByte(query[:loc[0]], '\n')
	return fmt.Sprintf(`{"message":"Field '%[2]s' doesn't exist on type '%[1]s'","locations":[{"line":%[3]d,"column":%[4]d}],`+
		`"path":["fragment x","%[2]s"],"extensions":{"code":"undefinedField","typeName":"%[1]s","fieldName":"%[2]s"}}`,
		typ, field, line, column)
}

// enterpriseServer is a GitHub Enterprise Server whose schema lacks the
// fields of lacks, by the type they are on, and that answers the GraphQL
// queries without them with data. It records the queries it gets.
type enterpriseServer struct {
	t     *testing.T
	lacks map[string]string
	data  []byte

	mu      sync.Mutex
	queries []string
}

func (s *enterpriseServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v3/repos/") {
		_, _ = w.Write([]byte("{}"))
		return
	}
	if r.URL.Path != "/api/graphql" {
		s.t.Errorf("request = %s %s", r.Method, r.URL.Path)
		return
	}
	var req gqlRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.t.Errorf("decode request: %v", err)
	}
	s.mu.Lock()
	s.queries = append(s.queries, req.Query)
	s.mu.Unlock()
	var errs []string
	for field, typ := range s.lacks {
		if regexp.MustCompile(`\b` + field + `\b`).MatchString(req.Query) {
			errs = append(errs, missingFieldError(req.Query, typ, field))
		}
	}
	if len(errs) > 0 {
		_, _ = w.Write([]byte(`{"errors":[` + strings.Join(errs, ",") + `]}`))
		return
	}
	_, _ = w.Write(s.data)
}

func (s *enterpriseServer) sent() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.queries...)
}

// newEnterpriseClient returns a client of srv as an Enterprise Server,
// whose API is below /api/v3.
func newEnterpriseClient(t *testing.T, srv http.Handler) *Client {
	t.Helper()
	ts := httptest.NewServer(srv)
	t.Cleanup(ts.Close)
	c, err := New(WithBaseURL(ts.URL+"/api/v3/"), WithToken("test-token"), WithHTTPClient(ts.Client()))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return retryAtOnce(c)
}

// TestQueryWithoutMissingFields reads a repository from a server whose
// schema lacks two of the fields the read selects: the read is sent again
// without them, which then read as unset, and later reads leave them out
// from the start.
func TestQueryWithoutMissingFields(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "repos_get_admin.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := &enterpriseServer{t: t, data: data, lacks: map[string]string{
		"hasDiscussionsEnabled": "Repository", "autoMergeAllowed": "Repository",
	}}
	c := newEnterpriseClient(t, srv)
	ref := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}

	for i := range 2 {
		got, err := c.GetRepo(t.Context(), ref)
		if err != nil {
			t.Fatalf("GetRepo %d: %v", i, err)
		}
		if !got.Caps.Known || got.Caps.Permission != core.PermissionAdmin || got.Caps.Discussions || got.Caps.AutoMerge {
			t.Errorf("GetRepo %d caps = %+v, want the admin's, with no discussions or auto-merge", i, got.Caps)
		}
	}
	sent := srv.sent()
	if len(sent) != 3 {
		t.Fatalf("sent %d queries, want the first, it again without the fields, and the second read without them:\n%q", len(sent), sent)
	}
	if sent[0] != getRepoQuery {
		t.Errorf("first query = %q, want getRepoQuery", sent[0])
	}
	for _, q := range sent[1:] {
		if strings.Contains(q, "hasDiscussionsEnabled") || strings.Contains(q, "autoMergeAllowed") ||
			!strings.Contains(q, "hasWikiEnabled") {
			t.Errorf("query = %q, want getRepoQuery without the missing fields only", q)
		}
	}
}

// TestQueryUnsupported fails a query whose missing field can't be left
// out, as the only one of its selection or the only spread of a fragment,
// with core.ErrUnsupported on an Enterprise Server. On github.com, which
// lacks no field the client selects, a missing field is never left out,
// and the query fails as it is, as the bug it is.
func TestQueryUnsupported(t *testing.T) {
	for _, tt := range []struct {
		name, query string
		enterprise  bool
	}{
		{"the only field of its selection", `query Q { viewer { status { novel } } }`, true},
		{"the only spread of a fragment", "query Q { viewer { login novel { ...more } } }\nfragment more on Novel { title }", true},
		{"github.com", `query Q { viewer { status { novel } } }`, false},
		{"github.com, a field that could be left out", `query Q { viewer { login novel } }`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			srv := &enterpriseServer{t: t, lacks: map[string]string{"novel": "User"}}
			h := http.Handler(srv)
			if !tt.enterprise {
				// github.com's API is at the root.
				h = http.StripPrefix("", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r.URL.Path = "/api" + r.URL.Path
					srv.ServeHTTP(w, r)
				}))
			}
			var c *Client
			if tt.enterprise {
				c = newEnterpriseClient(t, h)
			} else {
				c = newTestClient(t, h)
			}
			err := c.Query(t.Context(), tt.query, nil, nil)
			if err == nil {
				t.Fatal("Query succeeded, want it to fail")
			}
			if errors.Is(err, core.ErrUnsupported) != tt.enterprise {
				t.Errorf("Query = %v, unsupported %v, want %v", err, errors.Is(err, core.ErrUnsupported), tt.enterprise)
			}
			if n := len(srv.sent()); n != 1 {
				t.Errorf("sent %d queries, want 1: nothing may be left out", n)
			}
		})
	}
}

// TestQueryExtraFields decodes an answer with fields that the client
// doesn't read, as a newer GitHub may send, and leaves them be.
func TestQueryExtraFields(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"viewer":{"login":"octocat","novel":{"deep":[1,2]}},"extra":true}}`))
	}))
	var data struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
	}
	if err := c.Query(t.Context(), `query { viewer { login } }`, nil, &data); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if data.Viewer.Login != "octocat" {
		t.Errorf("login = %q, want octocat", data.Viewer.Login)
	}
}

func TestWithoutMissing(t *testing.T) {
	missing := func(query, typ, field string) error {
		var e GraphQLError
		if err := json.Unmarshal([]byte(`{"Errors":[`+missingFieldError(query, typ, field)+`]}`), &e); err != nil {
			t.Fatal(err)
		}
		return &e
	}
	tests := []struct {
		name, query, typ, field string
		want                    string
		ok                      bool
	}{
		{
			name: "leaf", query: "query Q {\n  viewer {\n    login\n    novel\n  }\n}",
			typ: "User", field: "novel", want: "query Q {\n  viewer {\n    login\n    }\n}", ok: true,
		},
		{
			name: "arguments, directives and selection", query: "query Q($n: Int!) { viewer { login novel(first: $n, q: \"a)b\") @include(if: true) { nodes { x } } } more(n: $n) }",
			typ: "User", field: "novel", want: "query Q($n: Int!) { viewer { login  } more(n: $n) }", ok: true,
		},
		{
			name: "alias", query: "{ viewer { login fresh: novel } }",
			typ: "User", field: "novel", want: "{ viewer { login } }", ok: true,
		},
		{
			name: "the only field of its selection", query: "{ viewer { status { novel } } }",
			typ: "UserStatus", field: "novel",
		},
		{
			name: "the only spread of a fragment", query: "query Q { viewer { login novel { ...more } } }\nfragment more on Novel { title }",
			typ: "User", field: "novel",
		},
		{
			name: "a fragment spread elsewhere too", query: "query Q { viewer { login novel { ...more } } node { ...more } }\nfragment more on Novel { title }",
			typ: "User", field: "novel", want: "query Q { viewer { login  } node { ...more } }\nfragment more on Novel { title }", ok: true,
		},
		{
			name: "the only use of a variable", query: "query Q($n: Int!) { viewer { login novel(first: $n) } }",
			typ: "User", field: "novel",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, fields, ok := withoutMissing(tt.query, missing(tt.query, tt.typ, tt.field))
			if ok != tt.ok || got != tt.want {
				t.Fatalf("withoutMissing = %q, %v, want %q, %v", got, ok, tt.want, tt.ok)
			}
			if ok && (len(fields) != 1 || fields[0] != tt.typ+"."+tt.field) {
				t.Errorf("fields = %v, want %s.%s", fields, tt.typ, tt.field)
			}
		})
	}

	// Another error than a missing field can't be helped.
	other := &GraphQLError{Errors: []GraphQLErrorItem{{Type: "NOT_FOUND", Message: "Could not resolve"}}}
	if _, _, ok := withoutMissing("{ viewer { login } }", other); ok {
		t.Error("withoutMissing left something out of a query that failed otherwise")
	}
}

// TestListPullRequestsWithoutLabelCaps lists the pull requests of a
// server older than 3.15, whose schema lacks viewerCanLabel: the list
// comes without it, and says nothing of who labels, which triage access
// then decides, rather than that nobody does.
func TestListPullRequestsWithoutLabelCaps(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "pulls_list.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The server answers as it would without the field.
	data = regexp.MustCompile(`\s*"viewerCanLabel": \w+,`).ReplaceAll(data, nil)
	srv := &enterpriseServer{t: t, data: data, lacks: map[string]string{"viewerCanLabel": "PullRequest"}}
	c := newEnterpriseClient(t, srv)
	for i := range 2 {
		page, err := c.ListPullRequests(t.Context(), pullsRepo, core.StateOpen, "", 30)
		if err != nil {
			t.Fatalf("ListPullRequests %d: %v", i, err)
		}
		for _, pr := range page.Items {
			if !pr.Caps.Known || pr.Caps.LabelKnown || pr.Caps.Label {
				t.Errorf("#%d caps = %+v, want known, with nothing said of labeling", pr.Number, pr.Caps)
			}
		}
	}
	if n := len(srv.sent()); n != 3 {
		t.Errorf("sent %d queries, want the first, it again without viewerCanLabel, and the second list without it", n)
	}
}
