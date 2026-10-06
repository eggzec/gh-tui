package owners

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// served is what a read returned, as far as these tests care.
type served struct {
	ok, stale, offline, limited bool
}

// ownerRead is one of the service's reads, run the same way for each.
type ownerRead struct {
	name string
	// fake makes api answer this read with err, or with a value if err is
	// nil.
	fake func(api *fakeAPI, err error)
	// read reads it, with again set on the read that follows a stale one.
	read func(ctx context.Context, s *Service, again bool) (served, error)
	// fresh reports whether reading it costs no request.
	fresh func(s *Service) bool
}

var ownerReads = []ownerRead{
	{
		name: "header",
		fake: func(api *fakeAPI, err error) {
			api.header = func(string) (core.Owner, error) {
				if errors.Is(err, core.ErrNotFound) {
					return core.Owner{}, err
				}
				return octocat, err
			}
		},
		read: func(ctx context.Context, s *Service, again bool) (served, error) {
			o, err := s.Header(ctx, HeaderQuery{Login: "Octocat", Again: again})
			return served{o.Profile.Login == "octocat", o.Stale, o.Offline, o.Limited}, err
		},
		fresh: func(s *Service) bool { return s.FreshHeader("octocat") },
	},
	{
		name: "user repos",
		fake: func(api *fakeAPI, err error) {
			api.userRepos = func(login string, _ core.RepoOrder, _ int, _ string) (core.Page[core.Repo], error) {
				return core.Page[core.Repo]{Items: repos(login, 2)}, err
			}
		},
		read: func(ctx context.Context, s *Service, again bool) (served, error) {
			p, err := s.Repos(ctx, ReposQuery{Owner: "octocat", Again: again})
			return served{len(p.Items) == 2, p.Stale, p.Offline, p.Limited}, err
		},
		fresh: func(s *Service) bool { return s.FreshRepos(ReposQuery{Owner: "OCTOCAT"}) },
	},
	{
		name: "org repos",
		fake: func(api *fakeAPI, err error) {
			api.orgRepos = func(login string, _ int, _ string) (core.Page[core.Repo], error) {
				return core.Page[core.Repo]{Items: repos(login, 2)}, err
			}
		},
		read: func(ctx context.Context, s *Service, again bool) (served, error) {
			p, err := s.Repos(ctx, ReposQuery{Owner: "Charm", Kind: core.OwnerOrg, Again: again})
			return served{len(p.Items) == 2, p.Stale, p.Offline, p.Limited}, err
		},
		fresh: func(s *Service) bool { return s.FreshRepos(ReposQuery{Owner: "charm", Kind: core.OwnerOrg}) },
	},
	{
		name: "contributions",
		fake: func(api *fakeAPI, err error) {
			api.contributions = func(string) (core.Contributions, error) { return core.Contributions{Total: 42}, err }
		},
		read: func(ctx context.Context, s *Service, again bool) (served, error) {
			c, err := s.Contributions(ctx, ContributionsQuery{Login: "OctoCat", Again: again})
			return served{c.Total == 42, c.Stale, c.Offline, c.Limited}, err
		},
		fresh: func(s *Service) bool { return s.FreshContributions("octocat") },
	},
	{
		name: "stars",
		fake: func(api *fakeAPI, err error) {
			api.stars = func(string, int, string) (core.Page[core.Repo], error) {
				return core.Page[core.Repo]{Items: repos("charm", 2)}, err
			}
		},
		read: func(ctx context.Context, s *Service, again bool) (served, error) {
			p, err := s.Stars(ctx, StarsQuery{Login: "OctoCat", Again: again})
			return served{len(p.Items) == 2, p.Stale, p.Offline, p.Limited}, err
		},
		fresh: func(s *Service) bool { return s.FreshStars(StarsQuery{Login: "octocat"}) },
	},
	peopleRead("followers", Followers, "OctoCat"),
	peopleRead("following", Following, "octocat"),
	peopleRead("orgs", Orgs, "Octocat"),
	peopleRead("members", Members, "Charm"),
	peopleRead("sponsors", Sponsors, "CHARM"),
	peopleRead("sponsoring", Sponsoring, "Octocat"),
	{
		name: "teams",
		fake: func(api *fakeAPI, err error) {
			api.teams = func(string, int, string) (core.Page[core.Team], error) {
				return core.Page[core.Team]{Items: []core.Team{{Slug: "a"}, {Slug: "b"}}}, err
			}
		},
		read: func(ctx context.Context, s *Service, again bool) (served, error) {
			p, err := s.Teams(ctx, TeamsQuery{Login: "Charm", Again: again})
			return served{len(p.Items) == 2, p.Stale, p.Offline, p.Limited}, err
		},
		fresh: func(s *Service) bool { return s.FreshTeams(TeamsQuery{Login: "charm"}) },
	},
	{
		name: "readme",
		fake: func(api *fakeAPI, err error) {
			api.readme = func(login string, _ core.OwnerKind, _ bool, _ github.Conditional) (core.Readme, github.Response, error) {
				if err != nil {
					return core.Readme{}, github.Response{}, err
				}
				return core.Readme{Markdown: "# hi", Source: core.RepoRef{Owner: login, Name: login}}, github.Response{ETag: `"r1"`}, nil
			}
		},
		read: func(ctx context.Context, s *Service, again bool) (served, error) {
			r, err := s.Readme(ctx, ReadmeQuery{Login: "OctoCat", Again: again})
			return served{r.Markdown == "# hi", r.Stale, r.Offline, r.Limited}, err
		},
		fresh: func(s *Service) bool { return s.FreshReadme(ReadmeQuery{Login: "octocat"}) },
	},
	{
		name: "org followers",
		fake: func(api *fakeAPI, err error) {
			api.orgFollowers = func(string) (int, error) { return 7, err }
		},
		read: func(ctx context.Context, s *Service, again bool) (served, error) {
			f, err := s.OrgFollowers(ctx, OrgFollowersQuery{Login: "Charm", Again: again})
			return served{f.Count == 7, f.Stale, f.Offline, f.Limited}, err
		},
		fresh: func(s *Service) bool { return s.FreshOrgFollowers("CHARM") },
	},
}

// peopleRead is the ownerRead of list, read for login.
func peopleRead(name string, list PeopleList, login string) ownerRead {
	return ownerRead{
		name: name,
		fake: func(api *fakeAPI, err error) {
			api.people = func(got, _ string, _ int, _ string) (core.Page[core.Person], error) {
				if got != name {
					return core.Page[core.Person]{}, fmt.Errorf("asked for %s: %w", got, errUnexpected)
				}
				return core.Page[core.Person]{Items: people(name, 2)}, err
			}
		},
		read: func(ctx context.Context, s *Service, again bool) (served, error) {
			p, err := s.People(ctx, PeopleQuery{Login: login, List: list, Again: again})
			return served{len(p.Items) == 2, p.Stale, p.Offline, p.Limited}, err
		},
		fresh: func(s *Service) bool { return s.FreshPeople(PeopleQuery{Login: strings.ToLower(login), List: list}) },
	}
}

func openStore(t *testing.T) *disk.Store {
	t.Helper()
	store, err := disk.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return store
}

// keep runs r once with store, so that a later session finds it kept.
func keep(t *testing.T, r ownerRead, store cache.Store) {
	t.Helper()
	api := &fakeAPI{t: t}
	r.fake(api, nil)
	if _, err := r.read(t.Context(), New(api, WithStore(store)), false); err != nil {
		t.Fatal(err)
	}
}

// A second read within the TTL is served from memory, whatever the case
// of the login.
func TestCacheHit(t *testing.T) {
	for _, r := range ownerReads {
		t.Run(r.name, func(t *testing.T) {
			api := &fakeAPI{t: t}
			r.fake(api, nil)
			s := New(api)
			for range 2 {
				if got, err := r.read(t.Context(), s, false); err != nil || got != (served{ok: true}) {
					t.Fatalf("read = %+v, %v; want the fetched value", got, err)
				}
			}
			if !r.fresh(s) {
				t.Error("the value isn't fresh")
			}
			if n := len(api.Calls()); n != 1 {
				t.Errorf("%d calls, want one fetch", n)
			}
		})
	}
}

// A session long after the one that kept an entry paints it at once,
// stale, until a read with Again set fetches it.
func TestKeptIsServedStaleThenFetched(t *testing.T) {
	for _, r := range ownerReads {
		t.Run(r.name, func(t *testing.T) {
			store := openStore(t)
			keep(t, r, store)

			api := &fakeAPI{t: t}
			r.fake(api, nil)
			s := New(api, WithStore(cachetest.Aged(store, 7*time.Hour)))
			got, err := r.read(t.Context(), s, false)
			if err != nil || got != (served{ok: true, stale: true}) {
				t.Fatalf("first read = %+v, %v; want the kept value, stale", got, err)
			}
			api.wantCalls(t)
			if r.fresh(s) {
				t.Error("the kept value is fresh, want it read again")
			}
			if got, err = r.read(t.Context(), s, true); err != nil || got != (served{ok: true}) {
				t.Fatalf("second read = %+v, %v; want the fetched value", got, err)
			}
			if n := len(api.Calls()); n != 1 {
				t.Errorf("%d calls, want one fetch", n)
			}
		})
	}
}

// An outage serves the kept entry marked offline, and a rate limit marked
// limited, until GitHub answers.
func TestFallback(t *testing.T) {
	offline, limited := served{ok: true, offline: true}, served{ok: true, limited: true}
	tests := []struct {
		name string
		err  error
		want served
	}{
		{"unreachable", fmt.Errorf("%w: %w", core.ErrOffline, &url.Error{Op: "Post", URL: "https://api.github.com/graphql", Err: errors.New("refused")}), offline},
		{"server error", &github.Error{StatusCode: 502}, offline},
		{"rate limited", fmt.Errorf("graphql: %w", &core.RateLimitError{Reset: time.Now().Add(time.Hour)}), limited},
	}
	for _, r := range ownerReads {
		for _, tt := range tests {
			t.Run(r.name+"/"+tt.name, func(t *testing.T) {
				store := openStore(t)
				keep(t, r, store)

				api := &fakeAPI{t: t}
				r.fake(api, tt.err)
				s := New(api, WithStore(cachetest.Aged(store, 7*time.Hour)))
				_, _ = r.read(t.Context(), s, false)
				if got, err := r.read(t.Context(), s, true); err != nil || got != tt.want {
					t.Errorf("read = %+v, %v; want %+v", got, err, tt.want)
				}
				r.fake(api, nil)
				if got, err := r.read(t.Context(), s, false); err != nil || got != (served{ok: true}) {
					t.Errorf("read once GitHub answers = %+v, %v; want it unmarked", got, err)
				}
			})
		}
	}
}

// An account that isn't there fails, isn't served offline, drops what was
// kept of it, and isn't asked about again for a moment.
func TestNotFound(t *testing.T) {
	for _, r := range ownerReads {
		t.Run(r.name, func(t *testing.T) {
			store := openStore(t)
			keep(t, r, store)

			api := &fakeAPI{t: t}
			r.fake(api, core.ErrNotFound)
			s := New(api, WithStore(cachetest.Aged(store, 7*time.Hour)))
			_, _ = r.read(t.Context(), s, false)
			got, err := r.read(t.Context(), s, true)
			if !errors.Is(err, core.ErrNotFound) || got.offline || got.limited {
				t.Fatalf("read = %+v, %v; want core.ErrNotFound", got, err)
			}
			if _, err := r.read(t.Context(), s, true); !errors.Is(err, core.ErrNotFound) {
				t.Errorf("read again = %v, want core.ErrNotFound", err)
			}
			if n := len(api.Calls()); n != 1 {
				t.Errorf("%d calls, want the missing account asked about once", n)
			}
			s.Invalidate()
			_, _ = r.read(t.Context(), s, true)
			if n := len(api.Calls()); n != 2 {
				t.Errorf("%d calls after Invalidate, want the account asked about again", n)
			}

			later := &fakeAPI{t: t}
			r.fake(later, core.ErrNotFound)
			if got, _ := r.read(t.Context(), New(later, WithStore(cachetest.Aged(store, 7*time.Hour))), false); got.stale {
				t.Error("read in a later session = stale, want the kept entry gone")
			}
		})
	}
}

// A header whose pinned repository GitHub refused is the header GitHub
// answered, cached as any other, with its mark of hidden pins.
func TestHeaderPartlyRefused(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{"unknown error", &github.GraphQLError{Errors: []github.GraphQLErrorItem{{
			Type: "SOMETHING", Message: "no", Path: []any{"repositoryOwner", "pinnedItems"},
		}}}},
		{"pinned repository forbidden", fmt.Errorf("%w: %w", core.ErrForbidden, &github.GraphQLError{Errors: []github.GraphQLErrorItem{{
			Type: "FORBIDDEN", Message: "hidden", Path: []any{"repositoryOwner", "pinnedItems", "nodes", 0.0},
		}}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			partial := octocat
			partial.Pinned, partial.HiddenPins = nil, true
			api := &fakeAPI{t: t, header: func(string) (core.Owner, error) {
				return partial, fmt.Errorf("owner octocat: graphql: %w", tt.err)
			}}
			s := New(api)
			o, err := s.Header(t.Context(), HeaderQuery{Login: "octocat"})
			if err != nil || o.Profile.Login != "octocat" || o.Stale || o.Offline {
				t.Fatalf("Header = %+v, %v; want the partial header", o, err)
			}
			if c, ok := s.CachedHeader("octocat"); !ok || !s.FreshHeader("octocat") || !c.HiddenPins {
				t.Errorf("cached header = %+v, %v; want the partial header, fresh and marked", c, ok)
			}
		})
	}
}

// An error about the account itself, or one with no header, fails the
// read.
func TestHeaderRefused(t *testing.T) {
	forbidden := fmt.Errorf("%w: %w", core.ErrForbidden, &github.GraphQLError{Errors: []github.GraphQLErrorItem{{
		Type: "FORBIDDEN", Message: "SSO", Path: []any{"repositoryOwner"},
	}}})
	tests := []struct {
		name  string
		owner core.Owner
		err   error
	}{
		{"account forbidden", octocat, fmt.Errorf("owner octocat: graphql: %w", forbidden)},
		{"rate limited", octocat, fmt.Errorf("owner octocat: graphql: %w: %w", &core.RateLimitError{Reset: time.Now().Add(time.Hour)}, &github.GraphQLError{Errors: []github.GraphQLErrorItem{{Type: "RATE_LIMITED", Message: "slow down"}}})},
		{"unknown error about the account", octocat, fmt.Errorf("owner octocat: graphql: %w", &github.GraphQLError{Errors: []github.GraphQLErrorItem{{
			Type: "SOMETHING", Message: "no", Path: []any{"repositoryOwner"},
		}}})},
		{"unknown error without a path", octocat, fmt.Errorf("owner octocat: graphql: %w", &github.GraphQLError{Errors: []github.GraphQLErrorItem{{Message: "no"}}})},
		{"scope missing", octocat, fmt.Errorf("owner octocat: graphql: %w", &core.ScopeError{Scopes: []string{"read:org"}, Err: &github.GraphQLError{Errors: []github.GraphQLErrorItem{{
			Type: "INSUFFICIENT_SCOPES", Message: "needs read:org", Path: []any{"repositoryOwner", "membersWithRole"},
		}}}})},
		{"no header", core.Owner{}, fmt.Errorf("owner octocat: graphql: %w", &github.GraphQLError{Errors: []github.GraphQLErrorItem{{Message: "no"}}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeAPI{t: t, header: func(string) (core.Owner, error) { return tt.owner, tt.err }}
			if _, err := New(api).Header(t.Context(), HeaderQuery{Login: "octocat"}); err == nil {
				t.Error("Header succeeded, want the error")
			}
		})
	}
}

// A user's pages are keyed by their order; an organization's ignore it.
func TestReposOrderKeys(t *testing.T) {
	stars := core.RepoOrder{Field: core.RepoOrderStars}
	user := ReposQuery{Owner: "octocat"}.normalize(30)
	byStars := ReposQuery{Owner: "octocat", Order: stars}.normalize(30)
	if user.key() == byStars.key() {
		t.Errorf("a user's orders share the key %q", user.key())
	}
	org := ReposQuery{Owner: "charm", Kind: core.OwnerOrg}.normalize(30)
	orgStars := ReposQuery{Owner: "Charm", Kind: core.OwnerOrg, Order: stars}.normalize(30)
	if org.key() != orgStars.key() {
		t.Errorf("an organization's keys %q and %q differ", org.key(), orgStars.key())
	}
}

// A read that only a user has misses for an organization without failing
// the organization's other reads, and the header's miss fails them all.
func TestNotFoundScopes(t *testing.T) {
	charm := core.Owner{Kind: core.OwnerOrg, ID: "O_1", Profile: core.Profile{Login: "charm"}}
	api := &fakeAPI{t: t,
		contributions: func(string) (core.Contributions, error) { return core.Contributions{}, core.ErrNotFound },
		userRepos: func(string, core.RepoOrder, int, string) (core.Page[core.Repo], error) {
			return core.Page[core.Repo]{}, core.ErrNotFound
		},
		header: func(string) (core.Owner, error) { return charm, nil },
		orgRepos: func(login string, _ int, _ string) (core.Page[core.Repo], error) {
			return core.Page[core.Repo]{Items: repos(login, 2)}, nil
		},
	}
	s := New(api)
	ctx := t.Context()
	if _, err := s.Contributions(ctx, ContributionsQuery{Login: "charm"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Contributions of an organization = %v, want core.ErrNotFound", err)
	}
	if _, err := s.Repos(ctx, ReposQuery{Owner: "charm"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Repos of an organization read as a user's = %v, want core.ErrNotFound", err)
	}
	if o, err := s.Header(ctx, HeaderQuery{Login: "Charm"}); err != nil || o.Profile.Login != "charm" {
		t.Errorf("Header after a user's read missed = %+v, %v; want the organization", o, err)
	}
	if p, err := s.Repos(ctx, ReposQuery{Owner: "charm", Kind: core.OwnerOrg}); err != nil || len(p.Items) != 2 {
		t.Errorf("Repos as an organization's = %+v, %v; want its repositories", p, err)
	}
	if _, err := s.Contributions(ctx, ContributionsQuery{Login: "CHARM"}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Contributions again = %v, want core.ErrNotFound", err)
	}
	api.wantCalls(t, "contributions charm", "user charm 30 ", "header Charm", "org charm 30 ")

	gone := &fakeAPI{t: t, header: func(string) (core.Owner, error) { return core.Owner{}, core.ErrNotFound }}
	s = New(gone)
	if _, err := s.Header(ctx, HeaderQuery{Login: "Octocat"}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("Header = %v, want core.ErrNotFound", err)
	}
	if _, err := s.Header(ctx, HeaderQuery{Login: "octocat"}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Header in another case = %v, want core.ErrNotFound", err)
	}
	if _, err := s.Contributions(ctx, ContributionsQuery{Login: "OCTOCAT"}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Contributions of a missing account = %v, want core.ErrNotFound", err)
	}
	if _, err := s.Repos(ctx, ReposQuery{Owner: "octocat"}); !errors.Is(err, core.ErrNotFound) {
		t.Errorf("Repos of a missing account = %v, want core.ErrNotFound", err)
	}
	gone.wantCalls(t, "header Octocat")
}

// AllRepos reads every page once, and the pages it read are cached for
// CachedAllRepos and the reads after it.
func TestAllRepos(t *testing.T) {
	pages := map[string]core.Page[core.Repo]{
		"":   {Items: repos("octocat", 2), Next: "p2"},
		"p2": {Items: repos("octocat", 1)},
	}
	api := &fakeAPI{t: t, userRepos: func(_ string, _ core.RepoOrder, _ int, after string) (core.Page[core.Repo], error) {
		return pages[after], nil
	}}
	s := New(api)
	q := ReposQuery{Owner: "octocat", PageSize: 2}
	if _, ok := s.CachedAllRepos(q, 0); ok {
		t.Fatal("CachedAllRepos found pages before any read")
	}
	p, err := s.AllRepos(t.Context(), q, 0)
	if err != nil || len(p.Items) != 3 || p.Next != "" {
		t.Fatalf("AllRepos = %d repositories, next %q, %v; want 3 and the end", len(p.Items), p.Next, err)
	}
	if c, ok := s.CachedAllRepos(q, 0); !ok || len(c.Items) != 3 {
		t.Errorf("CachedAllRepos = %d repositories, %v; want the 3 read", len(c.Items), ok)
	}
	if p, err := s.AllRepos(t.Context(), q, 1); err != nil || len(p.Items) != 2 || p.Next != "p2" {
		t.Errorf("AllRepos up to 1 = %d repositories, next %q, %v; want the first page", len(p.Items), p.Next, err)
	}
	api.wantCalls(t, "user octocat 2 ", "user octocat 2 p2")
}

// A page that fails fails the whole read.
func TestAllReposFails(t *testing.T) {
	api := &fakeAPI{t: t, orgRepos: func(_ string, _ int, after string) (core.Page[core.Repo], error) {
		if after == "" {
			return core.Page[core.Repo]{Items: repos("charm", 1), Next: "p2"}, nil
		}
		return core.Page[core.Repo]{}, core.ErrRateLimited
	}}
	_, err := New(api).AllRepos(t.Context(), ReposQuery{Owner: "charm", Kind: core.OwnerOrg}, 0)
	if !errors.Is(err, core.ErrRateLimited) {
		t.Errorf("AllRepos = %v, want the rate limit", err)
	}
}

// InvalidateLogin marks the reads of one account stale, and those of
// others stay fresh.
func TestInvalidateLogin(t *testing.T) {
	charm := core.Owner{Kind: core.OwnerOrg, ID: "O_1", Profile: core.Profile{Login: "charm"}}
	api := &fakeAPI{t: t,
		header: func(login string) (core.Owner, error) {
			if login == "charm" {
				return charm, nil
			}
			return octocat, nil
		},
		userRepos: func(login string, _ core.RepoOrder, _ int, _ string) (core.Page[core.Repo], error) {
			return core.Page[core.Repo]{Items: repos(login, 1)}, nil
		},
	}
	s := New(api)
	ctx := t.Context()
	for _, login := range []string{"octocat", "charm"} {
		if _, err := s.Header(ctx, HeaderQuery{Login: login}); err != nil {
			t.Fatal(err)
		}
	}
	q := ReposQuery{Owner: "octocat"}
	if _, err := s.Repos(ctx, q); err != nil {
		t.Fatal(err)
	}
	s.InvalidateLogin("OctoCat")
	if s.FreshHeader("octocat") || s.FreshRepos(q) {
		t.Error("octocat's reads are still fresh")
	}
	if !s.FreshHeader("charm") {
		t.Error("charm's header went stale too")
	}
}
