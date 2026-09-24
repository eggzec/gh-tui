package history

import (
	"context"
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// CachedCommit returns the cached detail of the commit sha without a
// request. It reports false if the detail isn't in memory.
func (s *Service) CachedCommit(repo core.RepoRef, sha string) (core.CommitDetail, bool) {
	e, st := s.details.Get(detailKey(repo, sha))
	return e.Value, st != cache.Miss
}

// Commit returns the commit sha with its stats and the first page of the
// files it changed, with their patches; CommitFiles reads the rest. Sha
// must be a full SHA, which is what makes the detail immutable: it is
// cached for good, in memory and in the object store.
func (s *Service) Commit(ctx context.Context, repo core.RepoRef, sha string) (core.CommitDetail, error) {
	if !isSHA(sha) {
		return core.CommitDetail{}, fmt.Errorf("get commit %q of %s: %w", sha, repo, errNotSHA)
	}
	d, err := object(ctx, s.details, s.keptDetails, detailKey(repo, sha), func(ctx context.Context) (core.CommitDetail, error) {
		return s.api.GetCommit(ctx, repo, sha)
	})
	if err != nil {
		return core.CommitDetail{}, fmt.Errorf("get commit %s of %s: %w", sha, repo, err)
	}
	return d, nil
}

// CommitFilesQuery selects a page of the files that a commit changed.
// GitHub lists them 300 to a page, so there is no page size.
type CommitFilesQuery struct {
	Repo core.RepoRef
	// SHA is the full SHA of the commit.
	SHA string
	// Cursor is the FilesNext of the commit's detail, or the Next of the
	// previous page. Empty means the first page, which the detail holds.
	Cursor string
}

// CachedCommitFiles returns the cached page for q without a request. It
// reports false if the page isn't in memory.
func (s *Service) CachedCommitFiles(q CommitFilesQuery) (core.Page[core.CommitFile], bool) {
	if q.Cursor == "" {
		d, ok := s.CachedCommit(q.Repo, q.SHA)
		return firstFiles(d), ok
	}
	e, st := s.files.Get(filesKey(q))
	return e.Value, st != cache.Miss
}

// CommitFiles returns a page of the files that a commit changed, with
// their patches, cached for good like Commit. A commit lists at most
// core.MaxCommitFiles files, which the detail's FilesTruncated reports.
func (s *Service) CommitFiles(ctx context.Context, q CommitFilesQuery) (core.Page[core.CommitFile], error) {
	if q.Cursor == "" {
		d, err := s.Commit(ctx, q.Repo, q.SHA)
		return firstFiles(d), err
	}
	if !isSHA(q.SHA) {
		return core.Page[core.CommitFile]{}, fmt.Errorf("list files of commit %q of %s: %w", q.SHA, q.Repo, errNotSHA)
	}
	p, err := object(ctx, s.files, s.keptFiles, filesKey(q), func(ctx context.Context) (core.Page[core.CommitFile], error) {
		return s.api.ListCommitFiles(ctx, q.Repo, q.SHA, q.Cursor)
	})
	if err != nil {
		return core.Page[core.CommitFile]{}, fmt.Errorf("list files of commit %s of %s: %w", q.SHA, q.Repo, err)
	}
	return p, nil
}

func firstFiles(d core.CommitDetail) core.Page[core.CommitFile] {
	return core.Page[core.CommitFile]{Items: d.Files, Next: d.FilesNext}
}

func detailKey(repo core.RepoRef, sha string) string {
	return "commit:" + repoKey(repo) + ":" + strings.ToLower(sha)
}

func filesKey(q CommitFilesQuery) string {
	return "commitfiles:" + repoKey(q.Repo) + ":" + strings.ToLower(q.SHA) + ":" + q.Cursor
}

// detailSize is what a cached detail costs in memory, roughly: its
// patches, mostly.
func detailSize(d core.CommitDetail) int64 {
	return int64(len(d.Message)) + filesBytes(d.Files) + 512
}

func filesSize(p core.Page[core.CommitFile]) int64 {
	return filesBytes(p.Items) + 64
}

func filesBytes(files []core.CommitFile) int64 {
	var n int64
	for i := range files {
		f := &files[i]
		n += int64(len(f.Patch)+len(f.Path)+len(f.PreviousPath)+len(f.SHA)) + 64
	}
	return n
}
