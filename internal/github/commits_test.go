package github

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

var golang = core.RepoRef{Owner: "golang", Name: "go"}

func TestListCommits(t *testing.T) {
	c := serveIssueFixture(t, "commits_list.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/cli/go-gh/commits", map[string]string{"sha": "trunk", "per_page": "3"})
		if got := r.Header.Get("If-None-Match"); got != `W/"e0"` {
			t.Errorf("If-None-Match = %q, want the cached ETag", got)
		}
	})

	got, res, err := c.ListCommits(t.Context(), goGH, "trunk", "", 3, Conditional{ETag: `W/"e0"`})
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	if res.ETag != `W/"e1"` {
		t.Errorf("ETag = %q, want W/\"e1\"", res.ETag)
	}
	if len(got.Items) != 3 {
		t.Fatalf("got %d commits, want 3", len(got.Items))
	}
	first := got.Items[0]
	at := issueTime("2026-09-24T07:55:39Z")
	want := core.Commit{
		SHA:     "859e813416733b742d6fc21df59a2058e04f0f38",
		TreeSHA: "57ad014f45b76b43945e7af56a7315aa58a7647c",
		Parents: []string{"460f28c4ab0b28de1479b5c1195cf03dc0b31d57"},
		Subject: "chore(deps): Bump the codeql-actions group with 2 updates (#308)",
		Trailers: []core.Trailer{
			{Key: "Signed-off-by", Value: "dependabot[bot] <support@github.com>"},
			{Key: "Co-authored-by", Value: "dependabot[bot] <49699333+dependabot[bot]@users.noreply.github.com>"},
		},
		Author: core.Signature{
			Name: "dependabot[bot]", Email: "49699333+dependabot[bot]@users.noreply.github.com",
			Login: "dependabot[bot]", Date: at,
		},
		Committer:    core.Signature{Name: "GitHub", Email: "noreply@github.com", Login: "web-flow", Date: at},
		Verification: core.Verification{Verified: true, Signed: true, Reason: "valid", VerifiedAt: at},
		URL:          "https://github.com/cli/go-gh/commit/859e813416733b742d6fc21df59a2058e04f0f38",
	}
	if first.SHA != want.SHA || first.TreeSHA != want.TreeSHA || !slices.Equal(first.Parents, want.Parents) ||
		first.Subject != want.Subject || !slices.Equal(first.Trailers, want.Trailers) ||
		first.Author != want.Author || first.Committer != want.Committer ||
		first.Verification != want.Verification || first.URL != want.URL {
		t.Errorf("first commit = %+v\nwant %+v", first, want)
	}
	if !strings.HasPrefix(first.Body, "Bumps the codeql-actions group") || strings.Contains(first.Body, "Signed-off-by") {
		t.Errorf("body = %q, want the body without its trailers", first.Body)
	}
	if !strings.HasSuffix(strings.TrimSpace(first.Message), "noreply.github.com>") {
		t.Errorf("message = %q, want it raw, with its trailers", first.Message)
	}
	if a := got.Items[2].Author; a.Login != "ryux1" || a.Name != "Ryu" {
		t.Errorf("third author = %+v, want ryux1 (Ryu)", a)
	}
}

func TestListCommitsUnlinkedAndUnsigned(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `[{"sha":"abc","commit":{
			"author":{"name":"A","email":"a@example.com","date":"2026-01-02T03:04:05Z"},
			"committer":{"name":"C","email":"c@example.com","date":"2026-01-02T03:04:06Z"},
			"message":"fix: x",
			"verification":{"verified":false,"reason":"unsigned","signature":null,"payload":null,"verified_at":null}},
			"author":null,"committer":null,"parents":[]}]`)
	}))

	got, _, err := c.ListCommits(t.Context(), goGH, "", "", 0, Conditional{})
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	cm := got.Items[0]
	if cm.Author != (core.Signature{Name: "A", Email: "a@example.com", Date: issueTime("2026-01-02T03:04:05Z")}) {
		t.Errorf("author = %+v, want git's signature without a login", cm.Author)
	}
	if cm.Committer.Login != "" || cm.Committer.Name != "C" {
		t.Errorf("committer = %+v", cm.Committer)
	}
	if cm.Verification != (core.Verification{Reason: "unsigned"}) {
		t.Errorf("verification = %+v, want unsigned", cm.Verification)
	}
}

func TestListCommitsPinsNext(t *testing.T) {
	const head = "859e813416733b742d6fc21df59a2058e04f0f38"
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		switch q.Get("page") {
		case "":
			w.Header().Set("Link", `<http://`+r.Host+`/repositories/1/commits?sha=trunk&per_page=1&page=2>; rel="next"`)
			_, _ = io.WriteString(w, `[{"sha":"`+head+`","parents":[{"sha":"p1"}]}]`)
		case "2":
			if q.Get("sha") != head || q.Get("per_page") != "1" {
				t.Errorf("second page query = %v, want the history of %s", q, head)
			}
			w.Header().Set("Link", `<http://`+r.Host+`/repositories/1/commits?sha=`+head+`&per_page=1&page=3>; rel="next"`)
			_, _ = io.WriteString(w, `[{"sha":"p1","parents":[]}]`)
		}
	}))

	first, _, err := c.ListCommits(t.Context(), goGH, "trunk", "", 1, Conditional{})
	if err != nil {
		t.Fatalf("ListCommits: %v", err)
	}
	u, err := url.Parse(first.Next)
	if err != nil || u.Query().Get("sha") != head || u.Query().Get("page") != "2" {
		t.Fatalf("Next = %q, want page 2 of %s", first.Next, head)
	}
	second, _, err := c.ListCommits(t.Context(), goGH, "trunk", first.Next, 1, Conditional{})
	if err != nil || second.Items[0].SHA != "p1" || !strings.Contains(second.Next, "page=3") {
		t.Errorf("second page = %+v, %v; want p1 and a next page", second, err)
	}
}

func TestListCommitsDefaultBranch(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/cli/go-gh/commits", nil)
		_, _ = io.WriteString(w, `[]`)
	}))

	if _, _, err := c.ListCommits(t.Context(), goGH, "", "", 0, Conditional{}); err != nil {
		t.Errorf("ListCommits: %v", err)
	}
}

func TestListCommitsEscapesRef(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("sha"); got != "feat/x&y" {
			t.Errorf("sha = %q, want the ref as one value", got)
		}
		_, _ = io.WriteString(w, `[]`)
	}))

	if _, _, err := c.ListCommits(t.Context(), goGH, "feat/x&y", "", 0, Conditional{}); err != nil {
		t.Errorf("ListCommits: %v", err)
	}
}

func TestListCommitsErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		body    string
		wantErr error
	}{
		{name: "not modified", status: http.StatusNotModified},
		// GitHub refuses to list the commits of an empty repository.
		{name: "empty repository", status: http.StatusConflict, body: `{"message":"Git Repository is empty."}`},
		{name: "not found", status: http.StatusNotFound, body: `{"message":"Not Found"}`, wantErr: core.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			got, res, err := c.ListCommits(t.Context(), goGH, "main", "", 0, Conditional{ETag: `W/"e1"`})
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got.Items != nil || got.Next != "" {
				t.Errorf("page = %+v, want an empty one", got)
			}
			if res.NotModified != (tt.status == http.StatusNotModified) {
				t.Errorf("NotModified = %v", res.NotModified)
			}
		})
	}
}

func TestGetCommit(t *testing.T) {
	c := serveIssueFixture(t, "commits_get.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/cli/go-gh/commits/859e813416733b742d6fc21df59a2058e04f0f38", nil)
		if r.Header.Get("If-None-Match") != "" {
			t.Error("a commit by SHA was asked for conditionally")
		}
	})

	got, err := c.GetCommit(t.Context(), goGH, "859e813416733b742d6fc21df59a2058e04f0f38")
	if err != nil {
		t.Fatalf("GetCommit: %v", err)
	}
	if got.SHA != "859e813416733b742d6fc21df59a2058e04f0f38" || !slices.Equal(got.Parents, []string{"460f28c4ab0b28de1479b5c1195cf03dc0b31d57"}) {
		t.Errorf("commit = %s with parents %v", got.SHA, got.Parents)
	}
	if got.Stats != (core.CommitStats{Additions: 2, Deletions: 2, Total: 4}) {
		t.Errorf("stats = %+v", got.Stats)
	}
	if got.FilesNext != "" || got.FilesTruncated || len(got.Files) != 1 {
		t.Fatalf("files = %+v, next %q, truncated %v; want one file", got.Files, got.FilesNext, got.FilesTruncated)
	}
	f := got.Files[0]
	if f.Path != ".github/workflows/codeql-analysis.yml" || f.Status != core.FileModified || f.PreviousPath != "" ||
		f.SHA != "39a18ab9f78c694b5f518a2eec4f23e0e94eaa48" || f.Additions != 2 || f.Deletions != 2 || f.PatchTruncated {
		t.Errorf("file = %+v", f)
	}
	if !strings.HasPrefix(f.Patch, "@@ -36,12 +36,12 @@ jobs:\n") {
		t.Errorf("patch = %q, want the hunks", f.Patch)
	}
}

func TestGetCommitTruncatedPatch(t *testing.T) {
	const sha = "8621461b26086e9a83cf118264aab413a5ecb8a1"
	c := serveIssueFixture(t, "commits_get_truncated_patch.json", func(*http.Request) {})

	got, err := c.GetCommit(t.Context(), golang, sha)
	if err != nil {
		t.Fatalf("GetCommit: %v", err)
	}
	var truncated []string
	for _, f := range got.Files {
		if f.PatchTruncated {
			truncated = append(truncated, f.Path)
			if f.Patch != "" {
				t.Errorf("%s has a patch and is truncated", f.Path)
			}
		} else if f.Patch == "" {
			t.Errorf("%s has no patch and isn't truncated", f.Path)
		}
	}
	want := []string{
		"src/cmd/vendor/golang.org/x/arch/x86/x86asm/avx_tables.go",
		"src/cmd/vendor/golang.org/x/arch/x86/x86asm/tables.go",
	}
	if !slices.Equal(truncated, want) {
		t.Errorf("truncated patches = %v, want %v", truncated, want)
	}
}

func TestGetCommitFileStatuses(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"sha":"abc","parents":[],"files":[
			{"filename":"new.go","previous_filename":"old.go","status":"renamed","changes":0},
			{"filename":"logo.png","status":"added","changes":0}
		]}`)
	}))

	got, err := c.GetCommit(t.Context(), goGH, "abc")
	if err != nil {
		t.Fatalf("GetCommit: %v", err)
	}
	want := []core.CommitFile{
		{Path: "new.go", PreviousPath: "old.go", Status: core.FileRenamed},
		// A binary file has no patch, and nothing was left out.
		{Path: "logo.png", Status: core.FileAdded},
	}
	if !slices.Equal(got.Files, want) {
		t.Errorf("files = %+v\nwant %+v", got.Files, want)
	}
	if len(got.Parents) != 0 {
		t.Errorf("parents = %v, want none", got.Parents)
	}
}

func TestGetCommitFilePages(t *testing.T) {
	const sha = "d4b26382342c98a95b85140b2863bc30c48edd68"
	first, err := os.ReadFile(filepath.Join("testdata", "commits_get_300_files.json"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join("testdata", "commits_files_page.json"))
	if err != nil {
		t.Fatal(err)
	}
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next := `<http://` + r.Host + `/repositories/23096959/commits/` + sha + `?page=%d>; rel="next"`
		last := `<http://` + r.Host + `/repositories/23096959/commits/` + sha + `?page=3>; rel="last"`
		switch r.URL.Query().Get("page") {
		case "":
			w.Header().Set("Link", strings.Replace(next, "%d", "2", 1)+", "+last)
			_, _ = w.Write(first)
		case "2":
			w.Header().Set("Link", strings.Replace(next, "%d", "3", 1)+", "+last)
			_, _ = w.Write(second)
		default:
			t.Errorf("unexpected page %s", r.URL.Query().Get("page"))
		}
	}))

	d, err := c.GetCommit(t.Context(), golang, sha)
	if err != nil {
		t.Fatalf("GetCommit: %v", err)
	}
	if len(d.Files) != 300 || d.FilesNext == "" || d.FilesTruncated {
		t.Fatalf("detail has %d files, next %q, truncated %v; want 300, a next page and all of them", len(d.Files), d.FilesNext, d.FilesTruncated)
	}
	page, err := c.ListCommitFiles(t.Context(), golang, sha, d.FilesNext)
	if err != nil {
		t.Fatalf("ListCommitFiles: %v", err)
	}
	want := []string{"src/archive/tar/stat_unix.go", "src/bytes/boundary_test.go"}
	paths := make([]string, 0, len(page.Items))
	for _, f := range page.Items {
		paths = append(paths, f.Path)
	}
	if !slices.Equal(paths, want) || !strings.Contains(page.Next, "page=3") {
		t.Errorf("second page = %v next %q, want %v and page 3", paths, page.Next, want)
	}
}

func TestFilesTruncated(t *testing.T) {
	tests := []struct {
		last string
		want bool
	}{
		{last: "", want: false},
		{last: "https://api.github.com/repositories/1/commits/abc?page=3", want: false},
		{last: "https://api.github.com/repositories/1/commits/abc?page=10", want: true},
		{last: "https://api.github.com/repositories/1/commits/abc?per_page=100&page=29", want: false},
		{last: "https://api.github.com/repositories/1/commits/abc?per_page=100&page=30", want: true},
	}
	for _, tt := range tests {
		if got := filesTruncated(Response{Last: tt.last}); got != tt.want {
			t.Errorf("filesTruncated(%q) = %v, want %v", tt.last, got, tt.want)
		}
	}
}

func TestCompare(t *testing.T) {
	c := serveIssueFixture(t, "compare.json", func(r *http.Request) {
		if got := r.URL.EscapedPath(); got != "/repos/cli/go-gh/compare/v2.0.0...feat%2Fx" {
			t.Errorf("path = %s, want both refs as one segment each", got)
		}
		q := r.URL.Query()
		if q.Get("per_page") != "1" || q.Get("page") != "2" {
			t.Errorf("query = %v, want the second page of one commit", q)
		}
		if got := r.Header.Get("If-None-Match"); got != `W/"e0"` {
			t.Errorf("If-None-Match = %q, want the cached ETag", got)
		}
	})

	got, res, err := c.Compare(t.Context(), goGH, "v2.0.0", "feat/x", Conditional{ETag: `W/"e0"`})
	if err != nil {
		t.Fatalf("Compare: %v", err)
	}
	if want := (core.Compare{Status: core.CompareAhead, AheadBy: 242}); got != want {
		t.Errorf("Compare = %+v, want %+v", got, want)
	}
	if res.ETag != `W/"e1"` {
		t.Errorf("ETag = %q", res.ETag)
	}
}

func TestCompareErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		wantErr error
	}{
		{name: "not modified", status: http.StatusNotModified},
		{name: "unknown ref", status: http.StatusNotFound, wantErr: core.ErrNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
			}))
			got, res, err := c.Compare(t.Context(), goGH, "main", "nope", Conditional{ETag: `W/"e1"`})
			if !errors.Is(err, tt.wantErr) || (tt.wantErr == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if got != (core.Compare{}) || res.NotModified != (tt.status == http.StatusNotModified) {
				t.Errorf("Compare = %+v, %+v", got, res)
			}
		})
	}
}

func BenchmarkGetCommit300Files(b *testing.B) {
	body, err := os.ReadFile(filepath.Join("testdata", "commits_get_300_files.json"))
	if err != nil {
		b.Fatal(err)
	}
	var d restCommitDetail
	b.ReportAllocs()
	b.SetBytes(int64(len(body)))
	for b.Loop() {
		d = restCommitDetail{}
		if err := decode(bytes.NewReader(body), &d); err != nil {
			b.Fatal(err)
		}
		_ = convert(d.Files, commitFile.core)
	}
}
