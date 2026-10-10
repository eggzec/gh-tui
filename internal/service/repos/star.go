package repos

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

// Star stars ref for the viewer. The returned Op already shows the star, and
// one more stargazer, in every cached list page and repository holding ref;
// its Do sends the change. Ref doesn't need to be cached.
func (s *Service) Star(ref core.RepoRef) *optimistic.Op {
	return s.setStarred(ref, true)
}

// Unstar removes the viewer's star from ref, the same way Star adds it.
func (s *Service) Unstar(ref core.RepoRef) *optimistic.Op {
	return s.setStarred(ref, false)
}

// setStarred stars ref or removes the star. A star the token may not
// change changes nothing, and the Op returns why.
func (s *Service) setStarred(ref core.RepoRef, starred bool) *optimistic.Op {
	if err := s.refused(ref); err != nil {
		verb := "star"
		if !starred {
			verb = "unstar"
		}
		return optimistic.Refused(fmt.Errorf("%s %s: %w", verb, ref, err))
	}
	tag := repoTag(ref)
	rollbacks := []func(){
		s.repos.MutateTag(tag, func(r core.Repo) (core.Repo, bool) {
			return withStarred(r, starred)
		}),
		s.lists.MutateTag(tag, func(p core.Page[core.Repo]) (core.Page[core.Repo], bool) {
			i := slices.IndexFunc(p.Items, func(r core.Repo) bool { return sameRepo(r.Ref, ref) })
			if i < 0 {
				return p, false
			}
			r, changed := withStarred(p.Items[i], starred)
			if !changed {
				return p, false
			}
			p.Items = slices.Clone(p.Items)
			p.Items[i] = r
			return p, true
		}),
	}
	send := func(ctx context.Context) error {
		call, verb := s.api.Star, "star"
		if !starred {
			call, verb = s.api.Unstar, "unstar"
		}
		if err := call(ctx, ref); err != nil {
			return fmt.Errorf("%s %s: %w", verb, ref, err)
		}
		// GitHub answers with no body, so the guessed count is all there
		// is. Marking the entries stale keeps them on screen until the next
		// read replaces them with the real count, and drops anything a
		// fetch stored while the request was in flight.
		s.repos.InvalidateTag(tag)
		s.lists.InvalidateTag(tag)
		// The viewer's own stars, on their owner page, are cached by the
		// owners service, which this one doesn't know.
		if s.stars != nil {
			s.stars.InvalidateStars()
		}
		return nil
	}
	return optimistic.New(send, rollbacks...)
}

// refused returns why the token may not star ref or remove its star, or
// nil when it may, or when that isn't known. A repository not read yet
// may be public, where public_repo is enough.
func (s *Service) refused(ref core.RepoRef) error {
	if s.access == nil {
		return nil
	}
	r, _ := s.CachedGet(ref)
	return s.access.Check(core.NeedWrite(r.Caps))
}

// withStarred returns r starred or not, and whether that changed it.
func withStarred(r core.Repo, starred bool) (core.Repo, bool) {
	if r.Starred == starred {
		return r, false
	}
	r.Starred = starred
	if starred {
		r.Stars++
	} else {
		r.Stars = max(r.Stars-1, 0)
	}
	return r, true
}

func sameRepo(a, b core.RepoRef) bool {
	return strings.EqualFold(a.Owner, b.Owner) && strings.EqualFold(a.Name, b.Name)
}
