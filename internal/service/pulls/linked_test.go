package pulls

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestLinksFreshHitMakesNoCall(t *testing.T) {
	api := &fakeAPI{}
	s := New(api)
	q := LinksQuery{Repo: repo, Number: 7}

	if _, ok := s.CachedLinks(q); ok {
		t.Fatal("CachedLinks reported links before any read")
	}
	if n := api.fakeLinked.count("PullLinks"); n != 0 {
		t.Fatalf("CachedLinks made %d calls, want none", n)
	}
	for range 2 {
		got, err := s.Links(t.Context(), q)
		if err != nil || got.ClosesTotal != 7 {
			t.Fatalf("Links = %+v, %v; want the links of #7", got, err)
		}
	}
	if n := api.fakeLinked.count("PullLinks"); n != 1 {
		t.Errorf("API called %d times, want 1: the second read is within the TTL", n)
	}
	if _, ok := s.CachedLinks(q); !ok {
		t.Error("CachedLinks reported nothing after a read")
	}
}

// A refresh of the pull request reads its links again.
func TestInvalidateRefetchesLinks(t *testing.T) {
	api := &fakeAPI{}
	s := New(api)
	q := LinksQuery{Repo: repo, Number: 7}
	if _, err := s.Links(t.Context(), q); err != nil {
		t.Fatalf("Links: %v", err)
	}

	s.Invalidate(repo)
	if _, err := s.Links(t.Context(), q); err != nil {
		t.Fatalf("Links: %v", err)
	}
	if n := api.fakeLinked.count("PullLinks"); n != 2 {
		t.Errorf("API called %d times, want 2: a refresh drops the links", n)
	}
}

// A refresh of one pull request reads its links and previews again, and
// leaves those of another pull request alone.
func TestRevalidateRefetchesLinks(t *testing.T) {
	api := &fakeAPI{get: func(context.Context, core.RepoRef, int) (core.PullRequestDetail, error) {
		return core.PullRequestDetail{}, nil
	}}
	s := New(api)
	q, other := LinksQuery{Repo: repo, Number: 7}, LinksQuery{Repo: repo, Number: 8}
	for _, l := range []LinksQuery{q, other} {
		if _, err := s.Links(t.Context(), l); err != nil {
			t.Fatalf("Links: %v", err)
		}
	}
	pq := PreviewQuery{Repo: repo, Number: 7}
	if _, err := s.IssuePreview(t.Context(), pq); err != nil {
		t.Fatalf("IssuePreview: %v", err)
	}

	if _, err := s.Revalidate(t.Context(), repo, 7); err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	for _, l := range []LinksQuery{q, other} {
		if _, err := s.Links(t.Context(), l); err != nil {
			t.Fatalf("Links: %v", err)
		}
	}
	if _, err := s.IssuePreview(t.Context(), pq); err != nil {
		t.Fatalf("IssuePreview: %v", err)
	}
	if n := api.fakeLinked.count("PullLinks"); n != 3 {
		t.Errorf("PullLinks called %d times, want 3: only #7 is read again", n)
	}
	if n := api.fakeLinked.count("IssuePreview"); n != 2 {
		t.Errorf("IssuePreview called %d times, want 2: the refresh drops the preview", n)
	}
}

func TestPreviewsKeyedByItem(t *testing.T) {
	api := &fakeAPI{}
	s := New(api)
	other := core.RepoRef{Owner: "acme", Name: "lib"}
	qs := []PreviewQuery{{Repo: repo, Number: 1}, {Repo: repo, Number: 2}, {Repo: other, Number: 1}}

	for range 2 {
		for _, q := range qs {
			i, err := s.IssuePreview(t.Context(), q)
			if err != nil || i.Target.Number != q.Number || i.Target.Repo != q.Repo {
				t.Fatalf("IssuePreview(%+v) = %+v, %v", q, i.Reference, err)
			}
			p, err := s.PullPreview(t.Context(), q)
			if err != nil || p.Target.Number != q.Number || p.Target.Repo != q.Repo {
				t.Fatalf("PullPreview(%+v) = %+v, %v", q, p.Reference, err)
			}
		}
	}
	for _, m := range []string{"IssuePreview", "PullPreview"} {
		if n := api.fakeLinked.count(m); n != len(qs) {
			t.Errorf("%s called %d times, want %d: one per item", m, n, len(qs))
		}
	}
	for _, q := range qs {
		if _, ok := s.CachedIssuePreview(q); !ok {
			t.Errorf("CachedIssuePreview(%+v) found nothing", q)
		}
		if _, ok := s.CachedPullPreview(q); !ok {
			t.Errorf("CachedPullPreview(%+v) found nothing", q)
		}
	}
	if _, ok := s.CachedPullPreview(PreviewQuery{Repo: repo, Number: 3}); ok {
		t.Error("CachedPullPreview found a preview that was never read")
	}
}

func TestLinkedErrorsWrappedAndNotCached(t *testing.T) {
	api := &fakeAPI{}
	api.err = core.ErrNotFound
	s := New(api)
	pq := PreviewQuery{Repo: repo, Number: 4}
	lq := LinksQuery{Repo: repo, Number: 4}

	for range 2 {
		_, err := s.Links(t.Context(), lq)
		if !errors.Is(err, core.ErrNotFound) || err == nil || !strings.Contains(err.Error(), "read linked items of pull eggzec/gh-tui#4") {
			t.Errorf("Links error = %v, want ErrNotFound with context", err)
		}
		_, err = s.IssuePreview(t.Context(), pq)
		if !errors.Is(err, core.ErrNotFound) || err == nil || !strings.Contains(err.Error(), "read preview of eggzec/gh-tui#4") {
			t.Errorf("IssuePreview error = %v, want ErrNotFound with context", err)
		}
		_, err = s.PullPreview(t.Context(), pq)
		if !errors.Is(err, core.ErrNotFound) || err == nil || !strings.Contains(err.Error(), "read preview of eggzec/gh-tui#4") {
			t.Errorf("PullPreview error = %v, want ErrNotFound with context", err)
		}
	}
	for _, m := range []string{"PullLinks", "IssuePreview", "PullPreview"} {
		if n := api.fakeLinked.count(m); n != 2 {
			t.Errorf("%s called %d times, want 2: errors must not be cached", m, n)
		}
	}
	if _, ok := s.CachedLinks(lq); ok {
		t.Error("CachedLinks reported links after a failed read")
	}
}
