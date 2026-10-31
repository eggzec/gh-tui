package github

import (
	"context"
	"net/http"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestVerifyReads(t *testing.T) {
	t.Parallel()
	const etag = `"abc"`
	var paths []string
	c := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/repos/o/r/issues/1":
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			w.Header().Set("ETag", etag)
			_, _ = w.Write([]byte(`{"html_url":"https://github.com/o/r/issues/1"}`))
		case "/repos/o/r/issues/2":
			_, _ = w.Write([]byte(`{"html_url":"https://github.com/o/r/pull/2","pull_request":{"url":"x"}}`))
		case "/users/octo":
			_, _ = w.Write([]byte(`{"html_url":"https://github.com/octo","type":"User"}`))
		case "/users/acme":
			_, _ = w.Write([]byte(`{"html_url":"https://github.com/acme","type":"Organization"}`))
		case "/repos/o/r/commits/abcdef1":
			_, _ = w.Write([]byte(`{"html_url":"https://github.com/o/r/commit/abcdef1234"}`))
		case "/repos/o/r/issues/4":
			_, _ = w.Write([]byte(`{"html_url":"https://github.com/o/r/issues/4","type":{"id":1,"name":"Bug"}}`))
		case "/repos/o/r/commits/deadbeef":
			http.Error(w, `{"message":"No commit found for SHA: deadbeef"}`, http.StatusUnprocessableEntity)
		case "/repos/o/r/issues/500":
			http.Error(w, `{"message":"boom"}`, http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	ctx := context.Background()
	repo := core.RepoRef{Owner: "o", Name: "r"}

	v, res, err := c.VerifyIssue(ctx, repo, 1, Conditional{})
	if err != nil || !v.Exists || v.Kind != core.VerifiedIssue || v.URL != "https://github.com/o/r/issues/1" || res.ETag != etag {
		t.Fatalf("issue: %+v %+v %v", v, res, err)
	}
	if _, res, err = c.VerifyIssue(ctx, repo, 1, Conditional{ETag: etag}); err != nil || !res.NotModified {
		t.Fatalf("304: %+v %v", res, err)
	}
	if v, _, err = c.VerifyIssue(ctx, repo, 2, Conditional{}); err != nil || v.Kind != core.VerifiedPull {
		t.Fatalf("pull: %+v %v", v, err)
	}
	if v, _, err = c.VerifyIssue(ctx, repo, 3, Conditional{}); err != nil || v.Exists {
		t.Fatalf("miss: %+v %v", v, err)
	}
	if v, _, err = c.VerifyUser(ctx, "octo", Conditional{}); err != nil || v.Kind != core.VerifiedUser || !v.Exists {
		t.Fatalf("user: %+v %v", v, err)
	}
	if v, _, err = c.VerifyUser(ctx, "acme", Conditional{}); err != nil || v.Kind != core.VerifiedOrg {
		t.Fatalf("org: %+v %v", v, err)
	}
	if v, _, err = c.VerifyUser(ctx, "nobody", Conditional{}); err != nil || v.Exists {
		t.Fatalf("user miss: %+v %v", v, err)
	}
	if v, _, err = c.VerifyCommit(ctx, repo, "abcdef1", Conditional{}); err != nil || v.Kind != core.VerifiedCommit || !v.Exists {
		t.Fatalf("commit: %+v %v", v, err)
	}
	if v, _, err = c.VerifyIssue(ctx, repo, 4, Conditional{}); err != nil || !v.Exists || v.Kind != core.VerifiedIssue {
		t.Fatalf("issue with an issue type: %+v %v", v, err)
	}
	if v, _, err = c.VerifyCommit(ctx, repo, "deadbeef", Conditional{}); err != nil || v.Exists {
		t.Fatalf("unknown commit should be a miss: %+v %v", v, err)
	}
	if _, _, err = c.VerifyIssue(ctx, repo, 500, Conditional{}); err == nil {
		t.Fatal("a server error was taken for an answer")
	}
}
