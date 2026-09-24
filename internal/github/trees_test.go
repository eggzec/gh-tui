package github

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

var gitRepo = core.RepoRef{Owner: "git", Name: "git"}

func TestGetTree(t *testing.T) {
	c := serveIssueFixture(t, "trees_get.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/git/git/git/trees/master", nil)
		if got := r.Header.Get("If-None-Match"); got != `W/"e0"` {
			t.Errorf("If-None-Match = %q, want the cached ETag", got)
		}
	})

	got, res, err := c.GetTree(t.Context(), gitRepo, "master", Conditional{ETag: `W/"e0"`})
	if err != nil {
		t.Fatalf("GetTree: %v", err)
	}
	if res.ETag != `W/"e1"` {
		t.Errorf("ETag = %q, want W/\"e1\"", res.ETag)
	}
	want := core.Tree{
		SHA: "0f8e75abebff0877cae681a3d5ff31ac47f54220",
		Entries: []core.TreeEntry{
			{Path: ".gitignore", Name: ".gitignore", Type: core.EntryBlob, Mode: "100644", SHA: "4da58c6754899e95a8d698f40d12e56ecf5ab728", Size: 3742},
			{Path: "Documentation", Name: "Documentation", Type: core.EntryTree, Mode: "040000", SHA: "a6810531ea06dc8d46500967c6a4d2b13167e4ed"},
			{Path: "GIT-VERSION-GEN", Name: "GIT-VERSION-GEN", Type: core.EntryBlob, Mode: "100755", SHA: "3b5fe08c19cc40988a6fd5fb8e5ce661528a304e", Size: 2455},
			{Path: "README.md", Name: "README.md", Type: core.EntryBlob, Mode: "100644", SHA: "46489b0971d04d02c1ba3eea5cd5c134e60c4f77", Size: 3808},
			{Path: "RelNotes", Name: "RelNotes", Type: core.EntryBlob, Mode: core.ModeSymlink, SHA: "752580e69384bae00cee646d3899a0aed36e1a92", Size: 34},
			{Path: "builtin", Name: "builtin", Type: core.EntryTree, Mode: "040000", SHA: "8788aa1198f74ca27ad7009c12afaced311b75b0"},
			{Path: "sha1collisiondetection", Name: "sha1collisiondetection", Type: core.EntryCommit, Mode: "160000", SHA: "855827c583bc30645ba427885caa40c5b81764d2"},
		},
	}
	if got.SHA != want.SHA || got.Truncated || !slices.Equal(got.Entries, want.Entries) {
		t.Errorf("tree = %+v\nwant %+v", got, want)
	}
}

func TestGetTreeEscapesRef(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.EscapedPath(); got != "/repos/git/git/git/trees/feat%2Fx" {
			t.Errorf("path = %s, want the ref as one segment", got)
		}
		_, _ = io.WriteString(w, `{"sha":"abc","tree":[],"truncated":false}`)
	}))

	if _, _, err := c.GetTree(t.Context(), gitRepo, "feat/x", Conditional{}); err != nil {
		t.Errorf("GetTree: %v", err)
	}
}

func TestGetTreeNotModified(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotModified)
	}))

	got, res, err := c.GetTree(t.Context(), gitRepo, "HEAD", Conditional{ETag: `W/"e1"`})
	if err != nil || !res.NotModified || got.SHA != "" || got.Entries != nil {
		t.Errorf("GetTree = %+v, %+v, %v; want an empty not-modified result", got, res, err)
	}
}

func TestGetTreeRecursive(t *testing.T) {
	c := serveIssueFixture(t, "trees_recursive.json", func(r *http.Request) {
		checkIssueRequest(t, r, http.MethodGet, "/repos/git/git/git/trees/59d5ff7a8e9c807adb147d06fdf6241cc6688825",
			map[string]string{"recursive": "1"})
	})

	got, _, err := c.GetTreeRecursive(t.Context(), gitRepo, "59d5ff7a8e9c807adb147d06fdf6241cc6688825", Conditional{})
	if err != nil {
		t.Fatalf("GetTreeRecursive: %v", err)
	}
	paths := make([]string, 0, len(got.Entries))
	for _, e := range got.Entries {
		paths = append(paths, e.Path)
	}
	wantPaths := []string{
		"libsecret", "libsecret/.gitignore", "libsecret/Makefile", "libsecret/git-credential-libsecret.c",
		"libsecret/meson.build", "meson.build", "netrc", "netrc/Makefile",
	}
	if !slices.Equal(paths, wantPaths) {
		t.Errorf("paths = %q\nwant %q", paths, wantPaths)
	}
	want := core.TreeEntry{
		Path: "libsecret/Makefile", Name: "Makefile", Type: core.EntryBlob, Mode: "100644",
		SHA: "9309cfb78c7f6c7336772620739d29f45fc72569", Size: 791,
	}
	if got.Entries[2] != want {
		t.Errorf("entry = %+v\nwant %+v", got.Entries[2], want)
	}
}

func TestGetTreeTruncated(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"sha":"abc","tree":[{"path":"a","mode":"100644","type":"blob","sha":"def","size":1}],"truncated":true}`)
	}))

	got, _, err := c.GetTreeRecursive(t.Context(), gitRepo, "HEAD", Conditional{})
	if err != nil || !got.Truncated || len(got.Entries) != 1 {
		t.Errorf("GetTreeRecursive = %+v, %v; want one entry of a truncated tree", got, err)
	}
}

func TestGetTreeErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"missing ref", http.StatusNotFound, core.ErrNotFound},
		// GitHub answers 409 for an empty repository.
		{"empty repository", http.StatusConflict, core.ErrConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, `{"message":"nope"}`)
			}))
			if _, _, err := c.GetTree(t.Context(), gitRepo, "HEAD", Conditional{}); !errors.Is(err, tt.want) {
				t.Errorf("GetTree error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestGetBlob(t *testing.T) {
	const sha = "752580e69384bae00cee646d3899a0aed36e1a92"
	tests := []struct {
		name     string
		body     []byte
		binary   bool
		noLength bool
		wantErr  error
		wantSize int64
	}{
		{name: "text", body: []byte("Documentation/RelNotes/2.56.0.adoc")},
		{name: "binary", body: []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), binary: true},
		{name: "empty", body: []byte{}},
		{name: "at the limit", body: bytes.Repeat([]byte("a"), 64)},
		{name: "too large", body: bytes.Repeat([]byte("a"), 65), wantErr: core.ErrTooLarge, wantSize: 65},
		{name: "too large without a length", body: bytes.Repeat([]byte("a"), 100), noLength: true, wantErr: core.ErrTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				checkIssueRequest(t, r, http.MethodGet, "/repos/git/git/git/blobs/"+sha, nil)
				if got := r.Header.Get("Accept"); got != "application/vnd.github.raw+json" {
					t.Errorf("Accept = %q, want the raw media type", got)
				}
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				if tt.noLength {
					// Flushing before the body is written makes the
					// response chunked, with no Content-Length.
					w.(http.Flusher).Flush()
				} else {
					w.Header().Set("Content-Length", strconv.Itoa(len(tt.body)))
				}
				_, _ = w.Write(tt.body)
			}))

			got, err := c.GetBlob(t.Context(), gitRepo, sha, 64)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("GetBlob error = %v, want %v", err, tt.wantErr)
				}
				if e, _ := errors.AsType[*core.TooLargeError](err); e == nil || e.Size != tt.wantSize || e.Limit != 64 {
					t.Errorf("error = %#v, want size %d and limit 64", e, tt.wantSize)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetBlob: %v", err)
			}
			want := core.Blob{SHA: sha, Size: int64(len(tt.body)), Content: tt.body, Binary: tt.binary}
			if got.SHA != want.SHA || got.Size != want.Size || !bytes.Equal(got.Content, want.Content) || got.Binary != want.Binary {
				t.Errorf("blob = %+v\nwant %+v", got, want)
			}
		})
	}
}

func TestGetBlobNotFound(t *testing.T) {
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"Not Found"}`)
	}))

	if _, err := c.GetBlob(t.Context(), gitRepo, "abc", 1<<20); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("GetBlob error = %v, want ErrNotFound", err)
	}
}
