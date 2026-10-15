package pulls

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/fallback"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// The commits of a pull request are read in one go, under its head and with
// REST's validators, as its files are. What a commit changed is named by its
// SHA, so it never changes, and is kept for good.

// CommitsQuery selects the commits of a pull request.
type CommitsQuery struct {
	Repo   core.RepoRef
	Number int
	// Head is the SHA of the pull request's head commit, as its detail
	// shows. A push is a new head, which reads its commits anew. It must be
	// set: without it, the commits of one push would be taken for another's.
	Head string
	// Again reads past a kept list: set it on the read that follows one that
	// came back Stale. It doesn't key the cache.
	Again bool
}

// key is the key of the list of q.
func (q CommitsQuery) key() string {
	return "pullcommits:" + repoID(q.Repo) + "#" + strconv.Itoa(q.Number) + "@" + strings.ToLower(q.Head)
}

// CachedCommits returns the commits of q if they are cached, fresh or stale,
// without fetching them.
func (s *Service) CachedCommits(q CommitsQuery) (core.Page[core.Commit], bool) {
	if q.Head == "" {
		return core.Page[core.Commit]{}, false
	}
	return cached(s.commits, q.key())
}

// Commits returns the commits of pull request q.Number, oldest first, in one
// page that holds them all. A fresh cached list is returned without a
// request. Otherwise it is read with REST, conditionally if a stale copy is
// cached, so that a list that didn't change costs no rate limit. It fails if
// q has no Head.
//
// A list that only an earlier session kept is returned at once, with Stale
// set, to every read until one with q.Again set reads it. If GitHub can't be
// reached, a cached list is served with Offline set, and if it rate limits
// the read, with Limited set. GitHub lists core.MaxPullCommits commits at
// most, and the page is then Truncated.
func (s *Service) Commits(ctx context.Context, q CommitsQuery) (core.Page[core.Commit], error) {
	if q.Head == "" {
		return core.Page[core.Commit]{}, fmt.Errorf("list commits of pull %s#%d: %w", q.Repo, q.Number, errNoHead)
	}
	key := q.key()
	if e, ok := s.keptCommits.Warm(s.commits, key, q.Again); ok {
		p := e.Value
		p.Stale = true
		return p, nil
	}
	p, err := fetch(ctx, s.commits, s.keptCommits, key, fallback.Page[core.Commit], s.loadCommits(q))
	if err != nil {
		return core.Page[core.Commit]{}, fmt.Errorf("list commits of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return p, nil
}

// loadCommits reads the commits of q with their validators, if it has a
// cached list.
func (s *Service) loadCommits(q CommitsQuery) cache.FetchFunc[core.Page[core.Commit]] {
	return recheck.Load(func(ctx context.Context, cond github.Conditional) (core.Page[core.Commit], github.Response, error) {
		return s.api.ListPullRequestCommits(ctx, q.Repo, q.Number, cond)
	}, func(core.Page[core.Commit]) []string { return tags(q.Repo, q.Number) })
}

func (s *Service) commitsTarget(key string) (recheck.Target, bool) {
	q, ok := parseCommitsKey(key)
	if !ok {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: q.Repo, Check: func(ctx context.Context) revalidate.Result {
		_, res := recheck.Check(ctx, s.commits, s.keptCommits, key, SyncKey(q.Repo), s.loadCommits(q))
		return res
	}}, true
}

// parseCommitsKey returns the query that CommitsQuery.key made key of. The
// repository comes back in lower case, which GitHub doesn't mind.
func parseCommitsKey(key string) (CommitsQuery, bool) {
	rest, ok := strings.CutPrefix(key, "pullcommits:")
	if !ok {
		return CommitsQuery{}, false
	}
	pull, head, ok := strings.Cut(rest, "@")
	if !ok || head == "" {
		return CommitsQuery{}, false
	}
	name, num, ok := strings.Cut(pull, "#")
	if !ok {
		return CommitsQuery{}, false
	}
	repo, err := core.ParseRepoRef(name)
	number, nerr := strconv.Atoi(num)
	if err != nil || nerr != nil || number <= 0 {
		return CommitsQuery{}, false
	}
	q := CommitsQuery{Repo: repo, Number: number, Head: head}
	// A key that doesn't make itself again isn't one of the service's.
	return q, q.key() == key
}

// CommitFilesQuery selects the files that one commit changed.
type CommitFilesQuery struct {
	Repo core.RepoRef
	// SHA is the full SHA of the commit, which is what makes what it changed
	// immutable.
	SHA string
}

// key is the key of the files of q.
func (q CommitFilesQuery) key() string {
	return "pullcommitfiles:" + repoID(q.Repo) + "@" + strings.ToLower(q.SHA)
}

// CachedCommitFiles returns the files of q if they are in memory, without
// fetching them.
func (s *Service) CachedCommitFiles(q CommitFilesQuery) (core.Page[core.CommitFile], bool) {
	return cached(s.commitFiles, q.key())
}

// CommitFiles returns the files that commit q.SHA changed against its first
// parent, with their patches, in one page that holds them all. They are
// cached for good, in memory and in the store, since a SHA never changes. It
// reads the first page with GetCommit and the rest with ListCommitFiles. A
// commit lists core.MaxCommitFiles files at most, and the page is then
// Truncated. It fails if q.SHA isn't a full SHA.
func (s *Service) CommitFiles(ctx context.Context, q CommitFilesQuery) (core.Page[core.CommitFile], error) {
	if !isSHA(q.SHA) {
		return core.Page[core.CommitFile]{}, fmt.Errorf("list files of commit %q of %s: %w", q.SHA, q.Repo, errNotSHA)
	}
	key := q.key()
	e, err := s.commitFiles.Fetch(ctx, key, func(ctx context.Context, _ cache.Entry[core.Page[core.CommitFile]], _ bool) (cache.Entry[core.Page[core.CommitFile]], error) {
		if kept, ok := s.keptCommitFiles.Load(key); ok {
			return cache.Entry[core.Page[core.CommitFile]]{Value: kept.Value}, nil
		}
		p, err := s.readCommitFiles(ctx, q)
		if err != nil {
			return cache.Entry[core.Page[core.CommitFile]]{}, err
		}
		e := cache.Entry[core.Page[core.CommitFile]]{Value: p}
		// The store is only a shortcut, so a failure is ignored.
		_ = s.keptCommitFiles.Save(key, e)
		return e, nil
	})
	if err != nil {
		return core.Page[core.CommitFile]{}, fmt.Errorf("list files of commit %s of %s: %w", q.SHA, q.Repo, err)
	}
	return e.Value, nil
}

// readCommitFiles reads all the files of the commit of q.
func (s *Service) readCommitFiles(ctx context.Context, q CommitFilesQuery) (core.Page[core.CommitFile], error) {
	d, err := s.api.GetCommit(ctx, q.Repo, q.SHA)
	if err != nil {
		return core.Page[core.CommitFile]{}, err
	}
	files := d.Files
	for next := d.FilesNext; next != ""; {
		p, err := s.api.ListCommitFiles(ctx, q.Repo, q.SHA, next)
		if err != nil {
			return core.Page[core.CommitFile]{}, err
		}
		files = append(files, p.Items...)
		next = p.Next
	}
	return core.Page[core.CommitFile]{Items: files, Truncated: d.FilesTruncated}, nil
}
