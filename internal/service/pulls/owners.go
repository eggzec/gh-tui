package pulls

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/codeowners"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/fallback"
)

// GitHub has no read that maps the files of a pull request to their code
// owners, so the CODEOWNERS file of the base is read and matched here. A
// commit's file never changes, so it is kept for good, while the members of
// a team do change, and stay fresh for the TTL.

// OwnersQuery selects the CODEOWNERS file of a repository at a commit.
type OwnersQuery struct {
	Repo core.RepoRef
	// Base is the full SHA of the commit the pull request merges into, as its
	// detail shows (core.PullRequestDetail.BaseSHA). It must be set: a
	// branch's name would read a file that moves.
	Base string
}

// OwnersFile is a parsed CODEOWNERS file.
type OwnersFile struct {
	codeowners.File
	// Path is where the file was found, and empty if the commit has none, in
	// which case nothing is owned.
	Path string
	// Offline and Limited say that the file was served in place of an answer,
	// as Page's do.
	Offline, Limited bool
}

// errNoBase is a read of an owners file that names no commit.
var errNoBase = errors.New("no base commit")

func (q OwnersQuery) key() string {
	return "pullowners:" + repoID(q.Repo) + "@" + strings.ToLower(q.Base)
}

func ownersMarks(f *OwnersFile) (offline, limited *bool) { return &f.Offline, &f.Limited }

// CachedOwners returns the owners file for q if it is cached, without
// reading it.
func (s *Service) CachedOwners(q OwnersQuery) (OwnersFile, bool) {
	if q.Base == "" {
		return OwnersFile{}, false
	}
	return cached(s.owners, q.key())
}

// Owners returns the CODEOWNERS file of q.Repo as it was at q.Base, parsed:
// the one in .github, else in the root, else in docs, as GitHub chooses. A
// commit without one gives a file with no rules and no Path. It fails if q
// has no Base.
//
// A commit's file never changes, so it is read once and kept for good, in
// memory parsed, and in the store as GitHub sent it. A session that finds it
// in the store makes no request, so a pull request seen before shows its
// owners offline too.
func (s *Service) Owners(ctx context.Context, q OwnersQuery) (OwnersFile, error) {
	if q.Base == "" {
		return OwnersFile{}, fmt.Errorf("read owners of %s: %w", q.Repo, errNoBase)
	}
	key := q.key()
	load := func(ctx context.Context, _ cache.Entry[OwnersFile], _ bool) (cache.Entry[OwnersFile], error) {
		kept, ok := s.keptOwners.Load(key)
		if !ok {
			f, err := s.api.GetCodeowners(ctx, q.Repo, q.Base)
			if err != nil {
				return cache.Entry[OwnersFile]{}, err
			}
			kept = cache.Entry[core.CodeownersFile]{Value: f}
			// The store is only a shortcut, so a failure is ignored.
			_ = s.keptOwners.Save(key, kept)
		}
		f := kept.Value
		return cache.Entry[OwnersFile]{Value: OwnersFile{File: codeowners.Parse(f.Text), Path: f.Path}}, nil
	}
	e, err := fallback.Fetch(ctx, s.owners, nil, key, ownersMarks, load)
	if err != nil {
		return OwnersFile{}, fmt.Errorf("read owners of %s at %s: %w", q.Repo, q.Base, err)
	}
	return e.Value, nil
}

// TeamQuery selects a team of an organization.
type TeamQuery struct {
	Org, Slug string
}

// TeamMembers are the members of a team.
type TeamMembers struct {
	Members []core.Person
	// Truncated says that the team has more members than were read.
	Truncated bool
	// Unavailable says that the members can't be shown: the token lacks the
	// read:org scope, or GitHub hides the team from it (404) or refuses the
	// read (403). The view leaves the members out, which is no error.
	Unavailable bool
	// Offline and Limited say that the members were served in place of an
	// answer, as Page's are.
	Offline, Limited bool
}

func (q TeamQuery) key() string {
	return "pullteam:" + strings.ToLower(q.Org) + "/" + strings.ToLower(q.Slug)
}

func membersMarks(m *TeamMembers) (offline, limited *bool) { return &m.Offline, &m.Limited }

// CachedTeamMembers returns the members of q if they are cached, fresh or
// stale, without reading them.
func (s *Service) CachedTeamMembers(q TeamQuery) (TeamMembers, bool) {
	return cached(s.members, q.key())
}

// TeamMembers returns the members of the team q. A fresh cached list is
// returned without a request.
//
// A token that lacks the read:org scope, as far as the access service
// knows, is not sent to GitHub, which would only answer 404: the result is
// Unavailable. So is a 404, which is how GitHub hides a team the token
// can't see, and a 403, which a token without the organization's members
// permission, an app, or an organization that requires SSO the token isn't
// authorized for gets. Both are cached for the TTL like members are. A rate
// limit is an error. If GitHub can't be reached, a cached list is served
// with Offline set.
func (s *Service) TeamMembers(ctx context.Context, q TeamQuery) (TeamMembers, error) {
	if s.access != nil && s.access.Check(core.NeedReadOrg) != nil {
		return TeamMembers{Unavailable: true}, nil
	}
	load := func(ctx context.Context, _ cache.Entry[TeamMembers], _ bool) (cache.Entry[TeamMembers], error) {
		p, err := s.api.ListTeamMembers(ctx, q.Org, q.Slug)
		switch {
		case errors.Is(err, core.ErrNotFound), errors.Is(err, core.ErrForbidden):
			return cache.Entry[TeamMembers]{Value: TeamMembers{Unavailable: true}}, nil
		case err != nil:
			return cache.Entry[TeamMembers]{}, err
		}
		return cache.Entry[TeamMembers]{Value: TeamMembers{Members: p.Items, Truncated: p.Truncated}}, nil
	}
	e, err := fallback.Fetch(ctx, s.members, nil, q.key(), membersMarks, load)
	if err != nil {
		return TeamMembers{}, fmt.Errorf("list members of team %s/%s: %w", q.Org, q.Slug, err)
	}
	return e.Value, nil
}
