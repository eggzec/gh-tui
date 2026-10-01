package issues

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// Kept lists the list pages, issues and comment pages that the service
// keeps with validators, for a revalidator to check in the background.
// Each check is one conditional request, which costs no rate limit when
// nothing changed. What changed is cached and kept, and reports SyncKey
// of its repository, so that the views showing it read it again. It reads
// the store, so call it where I/O is fine.
func (s *Service) Kept() []revalidate.Entry {
	return slices.Concat(
		recheck.Entries(s.keptLists, kindList, s.ttl, s.listTarget),
		recheck.Entries(s.keptIssues, kindIssue, s.ttl, s.issueTarget),
		recheck.Entries(s.keptComments, kindComments, s.ttl, s.commentsTarget),
	)
}

func (s *Service) listTarget(key string) (recheck.Target, bool) {
	q, ok := parseListKey(key)
	if !ok {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: q.Repo, Check: func(ctx context.Context) revalidate.Result {
		load := recheck.Load(func(ctx context.Context, cond github.Conditional) (core.Page[core.Issue], github.Response, error) {
			return s.api.ListIssues(ctx, q.Repo, q.State, q.Cursor, q.PageSize, cond)
		}, listTags(q.Repo))
		e, res := recheck.Check(ctx, s.lists, s.keptLists, key, SyncKey(q.Repo), load)
		if res.Status == revalidate.NotModified || res.Status == revalidate.Changed {
			s.vouch(q.Repo, e.Value.Items)
		}
		return res
	}}, true
}

func (s *Service) issueTarget(key string) (recheck.Target, bool) {
	repo, number, ok := parseIssueKey(strings.TrimPrefix(key, "issue:"))
	if !ok || !strings.HasPrefix(key, "issue:") {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: repo, Check: func(ctx context.Context) revalidate.Result {
		load := recheck.Load(func(ctx context.Context, cond github.Conditional) (core.Issue, github.Response, error) {
			return s.api.GetIssue(ctx, repo, number, cond)
		}, func(core.Issue) []string { return []string{repoTag(repo), key} })
		_, res := recheck.Check(ctx, s.issues, s.keptIssues, key, SyncKey(repo), load)
		return res
	}}, true
}

func (s *Service) commentsTarget(key string) (recheck.Target, bool) {
	q, ok := parseCommentsKey(key)
	if !ok {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: q.Repo, Check: func(ctx context.Context) revalidate.Result {
		ikey := issueKey(q.Repo, q.Number)
		// As for Comments, the page is at least as recent as what the
		// list showed before the request.
		version, _ := s.seen.Get(ikey)
		load := func(ctx context.Context, prev cache.Entry[stampedComments], ok bool) (cache.Entry[stampedComments], error) {
			var cond github.Conditional
			if ok {
				cond = github.Conditional{ETag: prev.ETag, LastModified: prev.LastModified}
			}
			p, res, err := s.api.ListIssueComments(ctx, q.Repo, q.Number, q.Cursor, q.PageSize, cond)
			switch {
			case err != nil:
				return cache.Entry[stampedComments]{}, err
			case res.NotModified:
				return cache.Entry[stampedComments]{}, cache.ErrNotModified
			}
			return cache.Entry[stampedComments]{
				Value: stampedComments{Value: p, Version: version},
				ETag:  res.ETag, LastModified: res.LastModified, Source: res.URL,
				Tags: []string{repoTag(q.Repo), ikey},
			}, nil
		}
		_, res := recheck.Check(ctx, s.comments, s.keptComments, key, SyncKey(q.Repo), load)
		return res
	}}, true
}

// parseListKey returns the query that listKey made key of.
func parseListKey(key string) (ListQuery, bool) {
	// The cursor is a URL, and may hold colons, so it comes last.
	parts := strings.SplitN(key, ":", 5)
	if len(parts) != 5 || parts[0] != "list" {
		return ListQuery{}, false
	}
	repo, err := core.ParseRepoRef(parts[1])
	size, serr := strconv.Atoi(parts[3])
	if err != nil || serr != nil {
		return ListQuery{}, false
	}
	return ListQuery{Repo: repo, State: core.StateFilter(parts[2]), PageSize: size, Cursor: parts[4]}, true
}

// parseCommentsKey returns the query that commentsKey made key of.
func parseCommentsKey(key string) (CommentsQuery, bool) {
	parts := strings.SplitN(key, ":", 4)
	if len(parts) != 4 || parts[0] != "comments" {
		return CommentsQuery{}, false
	}
	repo, number, ok := parseIssueKey(parts[1])
	size, err := strconv.Atoi(parts[2])
	if !ok || err != nil {
		return CommentsQuery{}, false
	}
	return CommentsQuery{Repo: repo, Number: number, PageSize: size, Cursor: parts[3]}, true
}

// parseIssueKey parses "owner/name#number".
func parseIssueKey(s string) (core.RepoRef, int, bool) {
	name, num, ok := strings.Cut(s, "#")
	if !ok {
		return core.RepoRef{}, 0, false
	}
	repo, err := core.ParseRepoRef(name)
	number, nerr := strconv.Atoi(num)
	if err != nil || nerr != nil || number <= 0 {
		return core.RepoRef{}, 0, false
	}
	return repo, number, true
}
