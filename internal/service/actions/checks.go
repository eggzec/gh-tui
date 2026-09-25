package actions

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
)

// ChecksQuery selects the checks of a commit: of the head of pull request
// Number, or else of commit SHA.
type ChecksQuery struct {
	Repo   core.RepoRef
	Number int
	SHA    string
}

var errNoCommit = errors.New("checks query names neither a pull request nor a commit")

func checksKey(q ChecksQuery) string {
	if q.Number > 0 {
		return "checks:" + repoID(q.Repo) + "#" + strconv.Itoa(q.Number)
	}
	return "checks:" + repoID(q.Repo) + "@" + q.SHA
}

// normalize names the commit of q by its SHA in lower case, as GitHub
// writes it.
func (q ChecksQuery) normalize() ChecksQuery {
	q.SHA = strings.ToLower(q.SHA)
	return q
}

// CachedChecks returns the cached checks for q, fresh or stale, without a
// request. It reports false if they aren't in memory.
func (s *Service) CachedChecks(q ChecksQuery) (core.Checks, bool) {
	e, st := s.checks.Get(checksKey(q.normalize()))
	return e.Value, st != cache.Miss
}

// Checks returns the check runs and commit statuses of the commit q
// names, in one GraphQL query. GraphQL has no validators, so a stale entry
// is read again in full, and nothing is kept beyond memory: checks move
// while a pull request is open.
func (s *Service) Checks(ctx context.Context, q ChecksQuery) (core.Checks, error) {
	q = q.normalize()
	if q.Number <= 0 && q.SHA == "" {
		return core.Checks{}, errNoCommit
	}
	e, err := s.checks.Fetch(ctx, checksKey(q), func(ctx context.Context, _ cache.Entry[core.Checks], _ bool) (cache.Entry[core.Checks], error) {
		var (
			c   core.Checks
			err error
		)
		if q.Number > 0 {
			c, err = s.api.PullChecks(ctx, q.Repo, q.Number)
		} else {
			c, err = s.api.CommitChecks(ctx, q.Repo, q.SHA)
		}
		if err != nil {
			return cache.Entry[core.Checks]{}, err
		}
		return cache.Entry[core.Checks]{Value: c, Tags: []string{repoTag(q.Repo)}}, nil
	})
	if err != nil {
		return core.Checks{}, fmt.Errorf("checks of %s: %w", q.Repo, err)
	}
	return e.Value, nil
}

// DefaultAnnotationPageSize is the page size of an AnnotationsQuery that
// sets none.
const DefaultAnnotationPageSize = 50

// AnnotationsQuery selects a page of the annotations of a check run, whose
// ID is that of its job for GitHub Actions.
type AnnotationsQuery struct {
	Repo       core.RepoRef
	CheckRunID int64
	// Cursor is the Next of the previous page, or empty for the first.
	Cursor string
	// PageSize defaults to DefaultAnnotationPageSize and is at most 100.
	PageSize int
}

func (q AnnotationsQuery) normalize() AnnotationsQuery {
	q.PageSize = pageSize(q.PageSize, DefaultAnnotationPageSize)
	return q
}

func annotationsKey(q AnnotationsQuery) string {
	v := url.Values{"per_page": {strconv.Itoa(q.PageSize)}}
	if q.Cursor != "" {
		v.Set("cursor", q.Cursor)
	}
	return "annotations:" + repoID(q.Repo) + "/" + strconv.FormatInt(q.CheckRunID, 10) + "?" + v.Encode()
}

// CachedAnnotations returns the cached page for q, fresh or stale, without
// a request. It reports false if the page isn't in memory.
func (s *Service) CachedAnnotations(q AnnotationsQuery) (core.Page[core.Annotation], bool) {
	e, st := s.annotations.Get(annotationsKey(q.normalize()))
	return e.Value, st != cache.Miss
}

// Annotations returns a page of the annotations of check run q.CheckRunID,
// such as the errors a compiler reported, revalidated with its ETag.
func (s *Service) Annotations(ctx context.Context, q AnnotationsQuery) (core.Page[core.Annotation], error) {
	q = q.normalize()
	e, err := fetch(ctx, s.annotations, nil, annotationsKey(q), []string{repoTag(q.Repo)}, offlinePage[core.Annotation],
		func(ctx context.Context, cond github.Conditional) (core.Page[core.Annotation], github.Response, error) {
			return s.api.ListAnnotations(ctx, q.Repo, q.CheckRunID, q.Cursor, q.PageSize, cond)
		})
	if err != nil {
		return core.Page[core.Annotation]{}, fmt.Errorf("annotations of check run %d of %s: %w", q.CheckRunID, q.Repo, err)
	}
	return e.Value, nil
}
