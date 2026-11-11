package releases

import (
	"context"
	"net/http"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

func defaultMain(context.Context, core.RepoRef) (string, error) { return "main", nil }

func extraOf(t *testing.T, s *Service) core.ReleaseExtra {
	t.Helper()
	x, err := s.Extra(t.Context(), repo, v3.ID)
	if err != nil {
		t.Fatalf("Extra: %v", err)
	}
	return x
}

func TestExtra(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{}
		s := New(api, WithDefaultBranch(defaultMain))
		if _, ok := s.CachedExtra(repo, v3.ID); ok {
			t.Fatal("an extra is cached before it was read")
		}
		want := core.ReleaseExtra{TagSHA: "abc123", Verified: true, AheadBy: 5, Default: "main"}
		if x := extraOf(t, s); !equal(x, want) {
			t.Errorf("Extra = %+v, want %+v", x, want)
		}
		if x, ok := s.CachedExtra(repo, v3.ID); !ok || x.AheadBy != 5 {
			t.Errorf("CachedExtra = %+v, %v", x, ok)
		}
		extraOf(t, s)
		if len(api.refs) != 1 || api.refs[0] != "v3.0.0" || api.heads[0] != "main" {
			t.Errorf("asked %v, %v; want the tag once against main", api.refs, api.heads)
		}

		// Past the TTL both are revalidated, and a 304 keeps the value.
		time.Sleep(config.Default().Cache.TTL.Releases)
		if x := extraOf(t, s); !equal(x, want) {
			t.Errorf("Extra after 304s = %+v, want %+v", x, want)
		}
		if len(api.refs) != 2 || len(api.heads) != 2 {
			t.Errorf("requests = %d commits, %d comparisons; want 2 each", len(api.refs), len(api.heads))
		}
	})
}

func equal(a, b core.ReleaseExtra) bool {
	return a.TagSHA == b.TagSHA && a.Verified == b.Verified && a.AheadBy == b.AheadBy &&
		a.Default == b.Default && slices.Equal(a.Mentions, b.Mentions)
}

func TestExtraCompareNotFound(t *testing.T) {
	api := &fakeAPI{compareErr: &github.Error{StatusCode: http.StatusNotFound}}
	s := New(api, WithDefaultBranch(defaultMain))
	x := extraOf(t, s)
	if x.AheadBy != -1 || x.TagSHA != "abc123" {
		t.Errorf("Extra = %+v, want the commit and AheadBy -1", x)
	}
}

func TestExtraNoDefault(t *testing.T) {
	api := &fakeAPI{}
	s := New(api)
	x := extraOf(t, s)
	if x.Default != "" || x.AheadBy != 5 {
		t.Errorf("Extra = %+v, want no branch name and AheadBy 5", x)
	}
	if len(api.heads) != 1 || api.heads[0] != "HEAD" {
		t.Errorf("compared with %v, want HEAD", api.heads)
	}
}

func TestExtraMentions(t *testing.T) {
	api := &fakeAPI{body: "Thanks @a and @b, and @A again."}
	s := New(api)
	want := []string{"a", "b"}
	if x := extraOf(t, s); !slices.Equal(x.Mentions, want) {
		t.Errorf("Extra mentions = %v, want %v", x.Mentions, want)
	}
	if x, ok := s.CachedExtra(repo, v3.ID); !ok || !slices.Equal(x.Mentions, want) {
		t.Errorf("CachedExtra mentions = %v, %v; want %v", x.Mentions, ok, want)
	}
}

// A tag that does not exist, such as that of a draft, is no error: the
// header shows the release without a commit.
func TestExtraTagNotFound(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusUnprocessableEntity} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				api := &fakeAPI{commitErr: &github.Error{StatusCode: status}}
				s := New(api, WithDefaultBranch(defaultMain))
				want := core.ReleaseExtra{AheadBy: -1, Default: "main"}
				if x := extraOf(t, s); !equal(x, want) {
					t.Errorf("Extra = %+v, want %+v", x, want)
				}
				if len(api.heads) != 0 {
					t.Errorf("compared %v, want no comparison without a tag", api.heads)
				}
				// Past the TTL the same answer keeps the entry.
				time.Sleep(config.Default().Cache.TTL.Releases)
				if x := extraOf(t, s); !equal(x, want) {
					t.Errorf("Extra again = %+v, want %+v", x, want)
				}
				if len(api.refs) != 2 {
					t.Errorf("asked %v, want the tag twice", api.refs)
				}
			})
		})
	}
}

func TestExtraCompareError(t *testing.T) {
	api := &fakeAPI{compareErr: &github.Error{StatusCode: http.StatusInternalServerError}}
	s := New(api, WithDefaultBranch(defaultMain))
	if _, err := s.Extra(t.Context(), repo, v3.ID); err == nil {
		t.Error("Extra succeeded, want the comparison's error")
	}
}

func TestExtraDefaultBranchError(t *testing.T) {
	api := &fakeAPI{}
	s := New(api, WithDefaultBranch(func(context.Context, core.RepoRef) (string, error) {
		return "", http.ErrHandlerTimeout
	}))
	x := extraOf(t, s)
	if x.Default != "" || len(api.heads) != 1 || api.heads[0] != "HEAD" {
		t.Errorf("Extra = %+v, compared %v; want no branch name and HEAD", x, api.heads)
	}
}
