//go:build schemacheck

package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/vektah/gqlparser/v2"
	gqlast "github.com/vektah/gqlparser/v2/ast"

	"github.com/eggzec/gh-tui/internal/core"
)

// TestSchema checks what the client sends against GitHub's published
// schemas of the oldest supported GitHub Enterprise Server and of
// github.com, which it downloads to $GH_TUI_SCHEMAS, or to a directory of
// the user's cache, and keeps for a week:
//
//	go test -tags schemacheck -run TestSchema ./internal/github/
//
// GITHUB_TOKEN, if set, lifts the limits of the API calls that find a
// schema GitHub has since removed from its repositories.
func TestSchema(t *testing.T) {
	ghes := "ghes-" + core.MinEnterprise
	schemas := []struct {
		name                       string
		graphql, upcoming, openapi string
		dotcom                     bool
	}{
		{
			name:     "github.com",
			dotcom:   true,
			graphql:  "src/graphql/data/fpt/schema.docs.graphql",
			upcoming: "src/graphql/data/fpt/graphql_upcoming_changes.public.yml",
			openapi:  "descriptions/api.github.com/api.github.com.json",
		},
		{
			name:     "GitHub Enterprise Server " + core.MinEnterprise,
			graphql:  "src/graphql/data/" + ghes + "/schema.docs-enterprise.graphql",
			upcoming: "src/graphql/data/" + ghes + "/graphql_upcoming_changes.public-enterprise.yml",
			openapi:  "descriptions/" + ghes + "/" + ghes + ".json",
		},
	}
	for _, s := range schemas {
		t.Run(s.name, func(t *testing.T) {
			t.Run("GraphQL", func(t *testing.T) {
				sdl := fetchSchema(t, "github/docs", s.graphql)
				schema, err := gqlparser.LoadSchema(&gqlast.Source{Name: s.graphql, Input: string(sdl)})
				if err != nil {
					t.Fatalf("load %s: %v", s.graphql, err)
				}
				upcoming, err := upcomingChanges(fetchSchema(t, "github/docs", s.upcoming))
				if err != nil {
					t.Fatalf("read %s: %v", s.upcoming, err)
				}
				for _, p := range checkGraphQL(schema, graphqlOptional, upcoming, graphqlOperationsFor(s.dotcom)) {
					t.Error(p)
				}
			})
			t.Run("REST", func(t *testing.T) {
				doc, err := openapi3.NewLoader().LoadFromData(fetchSchema(t, "github/rest-api-description", s.openapi))
				if err != nil {
					t.Fatalf("load %s: %v", s.openapi, err)
				}
				for _, p := range checkREST(doc, restCalls) {
					t.Error(p)
				}
			})
		})
	}
}

// schemaMaxAge is how long a downloaded schema is used before it is
// downloaded again.
const schemaMaxAge = 7 * 24 * time.Hour

// fetchSchema returns the file at path of GitHub's repo, from the
// schemas' directory if it was downloaded there within schemaMaxAge.
func fetchSchema(t *testing.T, repo, path string) []byte {
	t.Helper()
	dir := os.Getenv("GH_TUI_SCHEMAS")
	if dir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			t.Fatal(err)
		}
		dir = filepath.Join(cache, "gh-tui", "schemas")
	}
	file := filepath.Join(dir, repo, path)
	if fi, err := os.Stat(file); err == nil && time.Since(fi.ModTime()) < schemaMaxAge {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	// A download that hangs fails well within go test's own timeout.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	b, err := download(ctx, repo, path)
	if err != nil {
		t.Fatalf("download %s of %s: %v", path, repo, err)
	}
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return b
}

var errMissing = errors.New("not found")

// download returns the file at path of repo on its main branch, or, once
// GitHub removed it there, as a release it no longer supports is, as it
// was before the commit that removed it.
func download(ctx context.Context, repo, path string) ([]byte, error) {
	b, err := fetchURL(ctx, "https://raw.githubusercontent.com/"+repo+"/main/"+path)
	if !errors.Is(err, errMissing) {
		return b, err
	}
	commits, err := fetchURL(ctx, "https://api.github.com/repos/"+repo+"/commits?per_page=1&path="+path)
	if err != nil {
		return nil, err
	}
	var last []struct {
		Parents []struct {
			SHA string `json:"sha"`
		} `json:"parents"`
	}
	if err := json.Unmarshal(commits, &last); err != nil {
		return nil, err
	}
	if len(last) == 0 || len(last[0].Parents) == 0 {
		return nil, fmt.Errorf("%s: %w in its history", path, errMissing)
	}
	return fetchURL(ctx, "https://raw.githubusercontent.com/"+repo+"/"+last[0].Parents[0].SHA+"/"+path)
}

func fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, err
	}
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" && strings.HasPrefix(url, "https://api.github.com/") {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return nil, fmt.Errorf("%s: %w", url, errMissing)
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
