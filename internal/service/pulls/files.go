package pulls

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/fallback"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// The files a pull request changes are read a page at a time, each page
// cached under its head and cursor, with REST's validators.
//
// The diff is against the commit the branch forked from, which moves when
// the base branch is merged into it or the pull request is retargeted. So
// a head doesn't fix what it changes: a page is as stale as any other,
// confirmed with its ETag when the TTL runs out, and the head only keeps
// the pages of one push apart from the next.

// FilesQuery selects a page of the files that a pull request changes. A
// page holds as many files as GitHub gives, 100, so there is no page size
// to set.
type FilesQuery struct {
	Repo   core.RepoRef
	Number int
	// Head is the SHA of the pull request's head commit, as its detail
	// shows. A push is a new head, which reads its files anew. It must be
	// set: without it, the pages of one push would be taken for another's.
	Head string
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// Again reads past a kept page: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

// errNoHead is a read of files that names no head.
var errNoHead = errors.New("no head commit")

// headKey is the key of the pages of the files of the pull request at the
// head of q, apart from their cursor. It also tags them, so that the
// pages of one head are revalidated together.
func (q FilesQuery) headKey() string {
	return "pullfiles:" + repoID(q.Repo) + "#" + strconv.Itoa(q.Number) + "@" + strings.ToLower(q.Head)
}

// key is the key of the page of q.
func (q FilesQuery) key() string {
	return q.headKey() + "?" + url.Values{"cursor": {q.Cursor}}.Encode()
}

// CachedFiles returns the page for q if it is cached, fresh or stale,
// without fetching it.
func (s *Service) CachedFiles(q FilesQuery) (core.Page[core.CommitFile], bool) {
	if q.Head == "" {
		return core.Page[core.CommitFile]{}, false
	}
	return cached(s.files, q.key())
}

// Files returns the page of the files that pull request q.Number changes,
// with their patches, in the order GitHub lists them. A fresh cached page
// is returned without a request. Otherwise it is read with REST,
// conditionally if a stale copy is cached, so that a page that didn't
// change costs no rate limit. It fails if q has no Head.
//
// A page that only an earlier session kept is returned at once, with Stale
// set, to every read until one with q.Again set reads it. If GitHub can't
// be reached, a cached page is served with Offline set, and if it rate
// limits the read, with Limited set. GitHub lists core.MaxPullFiles files
// at most, and the first page is then Truncated.
//
// The pages are read at different times, and a change of the base moves
// all of them. When a page comes back changed, the others of its head are
// marked stale, so that none is shown beside a page that is newer.
func (s *Service) Files(ctx context.Context, q FilesQuery) (core.Page[core.CommitFile], error) {
	if q.Head == "" {
		return core.Page[core.CommitFile]{}, fmt.Errorf("list files of pull %s#%d: %w", q.Repo, q.Number, errNoHead)
	}
	key := q.key()
	if e, ok := s.keptFiles.Warm(s.files, key, q.Again); ok {
		p := e.Value
		p.Stale = true
		return p, nil
	}
	p, err := fetch(ctx, s.files, s.keptFiles, key, fallback.Page[core.CommitFile], s.loadFiles(q))
	if err != nil {
		return core.Page[core.CommitFile]{}, fmt.Errorf("list files of pull %s#%d: %w", q.Repo, q.Number, err)
	}
	return p, nil
}

// loadFiles reads the page for q with its validators, if it has a cached
// one. It marks the other pages of the head stale when the page changed.
func (s *Service) loadFiles(q FilesQuery) cache.FetchFunc[core.Page[core.CommitFile]] {
	load := recheck.Load(func(ctx context.Context, cond github.Conditional) (core.Page[core.CommitFile], github.Response, error) {
		return s.api.ListPullRequestFiles(ctx, q.Repo, q.Number, q.Cursor, cond)
	}, func(core.Page[core.CommitFile]) []string {
		return append(tags(q.Repo, q.Number), q.headKey())
	})
	return func(ctx context.Context, prev cache.Entry[core.Page[core.CommitFile]], ok bool) (cache.Entry[core.Page[core.CommitFile]], error) {
		e, err := load(ctx, prev, ok)
		if err == nil && ok && prev.ETag != "" && e.ETag != prev.ETag {
			s.files.InvalidateTag(q.headKey())
		}
		return e, err
	}
}

func (s *Service) filesTarget(key string) (recheck.Target, bool) {
	q, ok := parseFilesKey(key)
	if !ok {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: q.Repo, Check: func(ctx context.Context) revalidate.Result {
		_, res := recheck.Check(ctx, s.files, s.keptFiles, key, SyncKey(q.Repo), s.loadFiles(q))
		return res
	}}, true
}

// parseFilesKey returns the query that FilesQuery.key made key of. The
// repository comes back in lower case, which GitHub doesn't mind.
func parseFilesKey(key string) (FilesQuery, bool) {
	rest, ok := strings.CutPrefix(key, "pullfiles:")
	if !ok {
		return FilesQuery{}, false
	}
	pull, query, ok := strings.Cut(rest, "?")
	if !ok {
		return FilesQuery{}, false
	}
	pull, head, ok := strings.Cut(pull, "@")
	if !ok || head == "" {
		return FilesQuery{}, false
	}
	name, num, ok := strings.Cut(pull, "#")
	if !ok {
		return FilesQuery{}, false
	}
	repo, err := core.ParseRepoRef(name)
	number, nerr := strconv.Atoi(num)
	v, verr := url.ParseQuery(query)
	if err != nil || nerr != nil || verr != nil || number <= 0 {
		return FilesQuery{}, false
	}
	q := FilesQuery{Repo: repo, Number: number, Head: head, Cursor: v.Get("cursor")}
	// A key that doesn't make itself again isn't one of the service's.
	return q, q.key() == key
}

// filesSize is the memory a cached page of files takes, about.
func filesSize(p core.Page[core.CommitFile]) int64 {
	n := int64(len(p.Next)) + 64
	for i := range p.Items {
		f := &p.Items[i]
		n += int64(len(f.Patch)+len(f.Path)+len(f.PreviousPath)+len(f.SHA)) + 64
	}
	return n
}
