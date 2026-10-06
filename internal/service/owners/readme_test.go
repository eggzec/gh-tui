package owners

import (
	"errors"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
)

// readmeAPI answers the README of an organization as GitHub would: from
// .github-private for a member, else from .github, with the ETag of its
// content, which is the same in both, and a 304 when cond has it.
func readmeAPI(t *testing.T, content *string) *fakeAPI {
	t.Helper()
	return &fakeAPI{t: t, readme: func(login string, _ core.OwnerKind, member bool, cond github.Conditional) (core.Readme, github.Response, error) {
		etag := `"` + *content + `"`
		if cond.ETag == etag {
			return core.Readme{}, github.Response{NotModified: true, ETag: etag}, nil
		}
		r := core.Readme{Markdown: *content, Source: core.RepoRef{Owner: login, Name: ".github"}}
		if member {
			r.Source.Name, r.MembersOnly = ".github-private", true
		}
		return r, github.Response{ETag: etag}, nil
	}}
}

// A README read again past its TTL is revalidated with its ETag, and a
// 304 keeps it with its Source, which GitHub doesn't send again.
func TestReadmeNotModified(t *testing.T) {
	content := "v1"
	store := openStore(t)
	q := ReadmeQuery{Login: "Charm", Kind: core.OwnerOrg, Member: true}
	if _, err := New(readmeAPI(t, &content), WithStore(store)).Readme(t.Context(), q); err != nil {
		t.Fatal(err)
	}

	api := readmeAPI(t, &content)
	s := New(api, WithStore(cachetest.Aged(store, 7*time.Hour)))
	r, err := s.Readme(t.Context(), q)
	if err != nil || !r.Stale {
		t.Fatalf("first read = %+v, %v; want the kept README, stale", r, err)
	}
	q.Again = true
	r, err = s.Readme(t.Context(), q)
	want := core.Readme{Markdown: "v1", Source: core.RepoRef{Owner: "Charm", Name: ".github-private"}, MembersOnly: true}
	if err != nil || r != (Readme{Readme: want}) {
		t.Fatalf("read again = %+v, %v; want %+v", r, err, want)
	}
	if !s.FreshReadme(ReadmeQuery{Login: "charm", Kind: core.OwnerOrg, Member: true}) {
		t.Error("the confirmed README isn't fresh")
	}
	api.wantCalls(t, `readme Charm true "v1"`)
}

// A change of membership reads the README again without validators, so
// that a 304 can't confirm the README of the other repository.
func TestReadmeMembershipChange(t *testing.T) {
	content := "same"
	api := readmeAPI(t, &content)
	s := New(api)
	ctx := t.Context()
	public, err := s.Readme(ctx, ReadmeQuery{Login: "charm", Kind: core.OwnerOrg})
	if err != nil || public.Source.Name != ".github" || public.MembersOnly {
		t.Fatalf("README of an outsider = %+v, %v", public, err)
	}
	member, err := s.Readme(ctx, ReadmeQuery{Login: "Charm", Kind: core.OwnerOrg, Member: true})
	if err != nil || member.Source.Name != ".github-private" || !member.MembersOnly {
		t.Fatalf("README of a member = %+v, %v; want the members' README", member, err)
	}
	// Each membership's README stays cached on its own.
	if _, err := s.Readme(ctx, ReadmeQuery{Login: "charm", Kind: core.OwnerOrg}); err != nil {
		t.Fatal(err)
	}
	api.wantCalls(t, "readme charm false ", "readme Charm true ")
	// A user's README has no members, whatever the query says.
	user := ReadmeQuery{Login: "octocat"}
	if user.key() != (ReadmeQuery{Login: "OctoCat", Member: true}).key() {
		t.Error("a user's README is keyed by membership")
	}
}

// Kept lists the kept READMEs, and checking one revalidates it with its
// ETag: a 304 changes nothing, and a new README is stored and named by the
// account's sync key.
func TestReadmeKept(t *testing.T) {
	content := "v1"
	store := openStore(t)
	api := readmeAPI(t, &content)
	s := New(api, WithStore(store))
	q := ReadmeQuery{Login: "Charm", Kind: core.OwnerOrg}
	if _, err := s.Readme(t.Context(), q); err != nil {
		t.Fatal(err)
	}
	// A later session finds it due.
	kept := New(api, WithStore(cachetest.Aged(store, 7*time.Hour))).Kept()
	if len(kept) != 1 || kept[0].FreshFor != time.Hour {
		t.Fatalf("Kept = %+v, want the README, fresh for the TTL", kept)
	}
	if res := kept[0].Check(t.Context()); res.Status != revalidate.NotModified {
		t.Errorf("check of the same README = %+v, want not modified", res)
	}
	content = "v2"
	if res := kept[0].Check(t.Context()); res.Status != revalidate.Changed || res.Sync != SyncKey("charm") {
		t.Errorf("check of a new README = %+v, want changed, with %q", res, SyncKey("charm"))
	}
	r, err := New(api, WithStore(store)).Readme(t.Context(), q)
	if err != nil || r.Stale || r.Markdown != "v2" || r.Source.Name != ".github" {
		t.Errorf("Readme after the check = %+v, %v; want the new README, fresh", r, err)
	}
	api.wantCalls(t, "readme Charm false ", `readme charm false "v1"`, `readme charm false "v1"`)
}

func TestParseReadmeKey(t *testing.T) {
	for _, q := range []ReadmeQuery{
		{Login: "octocat"},
		{Login: "charm", Kind: core.OwnerOrg},
		{Login: "charm", Kind: core.OwnerOrg, Member: true},
	} {
		got, ok := parseReadmeKey(q.key())
		if !ok || got != q {
			t.Errorf("parseReadmeKey(%q) = %+v, %v; want %+v", q.key(), got, ok, q)
		}
	}
	for _, key := range []string{"owner:octocat", "ownerreadme:octocat", "ownerreadme:?kind=user", "ownerreadme:x?kind=team", "ownerreadme:x?kind=user&member=1"} {
		if _, ok := parseReadmeKey(key); ok {
			t.Errorf("parseReadmeKey(%q) succeeded", key)
		}
	}
}

// A README too large to read fails with the error that says so, and
// leaves nothing in the cache, so that the next read asks again.
func TestReadmeTooLarge(t *testing.T) {
	api := &fakeAPI{t: t, readme: func(string, core.OwnerKind, bool, github.Conditional) (core.Readme, github.Response, error) {
		return core.Readme{}, github.Response{}, &core.TooLargeError{Size: 3 << 20, Limit: 1 << 20}
	}}
	s := New(api, WithStore(openStore(t)))
	q := ReadmeQuery{Login: "octocat"}
	_, err := s.Readme(t.Context(), q)
	if e, ok := errors.AsType[*core.TooLargeError](err); !ok || e.Size != 3<<20 || e.Limit != 1<<20 || !errors.Is(err, core.ErrTooLarge) {
		t.Fatalf("Readme error = %v, want a TooLargeError of %d over %d", err, 3<<20, 1<<20)
	}
	if _, ok := s.CachedReadme(q); ok {
		t.Error("a README too large to read was cached")
	}
	if _, err := s.Readme(t.Context(), q); err == nil {
		t.Error("second read succeeded, want the error again")
	}
	if got := len(api.Calls()); got != 2 {
		t.Errorf("calls = %d, want 2: a failure isn't kept", got)
	}
}
