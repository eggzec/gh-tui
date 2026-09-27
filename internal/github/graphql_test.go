package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
)

const repoQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) { name }
}`

type repoData struct {
	Repository *repo `json:"repository"`
}

// graphqlHandler checks that each request is a GraphQL POST and answers
// with body.
func graphqlHandler(t *testing.T, header http.Header, body string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/graphql" {
			t.Errorf("request = %s %s, want POST /graphql", r.Method, r.URL.Path)
		}
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if req.Query != repoQuery || req.Variables["owner"] != "o" || req.Variables["name"] != "r" {
			t.Errorf("request = %+v, want repoQuery for o/r", req)
		}
		maps.Copy(w.Header(), header)
		_, _ = io.WriteString(w, body)
	})
}

var repoVars = map[string]any{"owner": "o", "name": "r"}

func TestQuery(t *testing.T) {
	c := newTestClient(t, graphqlHandler(t, nil, `{"data":{"repository":{"name":"r"}}}`))

	var got repoData
	if err := c.Query(t.Context(), repoQuery, repoVars, &got); err != nil {
		t.Fatalf("Query: %v", err)
	}
	if got.Repository == nil || got.Repository.Name != "r" {
		t.Errorf("data = %+v, want repository r", got)
	}
}

func TestQueryPartialData(t *testing.T) {
	c := newTestClient(t, graphqlHandler(t, nil, `{
		"data": {"repository": {"name": "r"}},
		"errors": [{"type": "FORBIDDEN", "message": "Resource not accessible", "path": ["repository", "issues", 0]}]
	}`))

	var got repoData
	err := c.Query(t.Context(), repoQuery, repoVars, &got)

	var gqlErr *GraphQLError
	if !errors.As(err, &gqlErr) {
		t.Fatalf("error %v is not a *GraphQLError", err)
	}
	if len(gqlErr.Errors) != 1 || gqlErr.Errors[0].Type != "FORBIDDEN" || gqlErr.Errors[0].Message != "Resource not accessible" {
		t.Errorf("errors = %+v, want one FORBIDDEN", gqlErr.Errors)
	}
	if errors.Is(err, core.ErrNotFound) {
		t.Error("FORBIDDEN matches ErrNotFound")
	}
	if got.Repository == nil || got.Repository.Name != "r" {
		t.Errorf("partial data = %+v, want repository r", got)
	}
}

func TestQueryNotFound(t *testing.T) {
	c := newTestClient(t, graphqlHandler(t, nil, `{
		"data": {"repository": null},
		"errors": [{"type": "NOT_FOUND", "path": ["repository"], "message": "Could not resolve to a Repository with the name 'o/r'."}]
	}`))

	var got repoData
	err := c.Query(t.Context(), repoQuery, repoVars, &got)

	if !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("error %v is not ErrNotFound", err)
	}
	var gqlErr *GraphQLError
	if !errors.As(err, &gqlErr) || gqlErr.Errors[0].Message != "Could not resolve to a Repository with the name 'o/r'." {
		t.Errorf("error %v does not keep GitHub's message", err)
	}
	if got.Repository != nil {
		t.Errorf("repository = %+v, want nil", got.Repository)
	}
}

func TestQueryUnprocessable(t *testing.T) {
	c := newTestClient(t, graphqlHandler(t, nil, `{
		"data": {"mergePullRequest": null},
		"errors": [{"type": "UNPROCESSABLE", "path": ["mergePullRequest"], "message": "Pull Request is not mergeable"}]
	}`))

	err := c.Query(t.Context(), repoQuery, repoVars, nil)

	if !errors.Is(err, core.ErrConflict) {
		t.Errorf("error %v is not ErrConflict", err)
	}
	if errors.Is(err, core.ErrNotFound) {
		t.Errorf("error %v matches ErrNotFound", err)
	}
}

func TestQueryRateLimited(t *testing.T) {
	header := http.Header{
		"X-Ratelimit-Limit":     {"5000"},
		"X-Ratelimit-Remaining": {"0"},
		"X-Ratelimit-Reset":     {"1790000000"},
		"X-Ratelimit-Resource":  {"graphql"},
	}
	reset := time.Unix(1790000000, 0)
	c := newTestClientAt(t, reset.Add(-time.Minute), graphqlHandler(t, header, `{"errors": [{"type": "RATE_LIMITED", "message": "API rate limit exceeded"}]}`))

	err := c.Query(t.Context(), repoQuery, repoVars, nil)

	var rl *core.RateLimitError
	if !errors.As(err, &rl) || !errors.Is(err, core.ErrRateLimited) {
		t.Fatalf("error %v is not a rate limit", err)
	}
	if want := reset.Add(minGuard); !rl.Reset.Equal(want) {
		t.Errorf("Reset = %v, want %v", rl.Reset, want)
	}
	if got := c.RateLimit(resourceGraphQL); got.Resource != "graphql" || got.Remaining != 0 {
		t.Errorf("RateLimit = %+v, want graphql with 0 remaining", got)
	}
}

func TestQueryHTTPError(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"message":"Bad credentials"}`)
	}))

	err := c.Query(t.Context(), repoQuery, repoVars, nil)

	var apiErr *Error
	if !errors.Is(err, core.ErrUnauthorized) || !errors.As(err, &apiErr) || apiErr.Message != "Bad credentials" {
		t.Errorf("error = %v, want *Error for ErrUnauthorized", err)
	}
}

func TestQueryContextCanceled(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("request reached the server")
	}))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	err := c.Query(ctx, repoQuery, repoVars, nil)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Query error = %v, want context.Canceled", err)
	}
}
