package pulls

import (
	"context"
	"net/url"
	"strconv"
	"strings"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// Kept lists the comment pages that the service keeps with validators, for
// a revalidator to check in the background. Each check is one conditional
// request, which costs no rate limit when nothing changed. A page that
// changed is cached and kept, and reports SyncKey of its repository, so
// that the views showing it read it again. The list pages and details are
// GraphQL reads, which have no validators: Poll and the list's update
// times watch them instead. It reads the store, so call it where I/O is
// fine.
func (s *Service) Kept() []revalidate.Entry {
	return recheck.Entries(s.keptComments, kindComments, s.commentsTarget)
}

func (s *Service) commentsTarget(key string) (recheck.Target, bool) {
	q, ok := parseCommentsKey(key)
	if !ok {
		return recheck.Target{}, false
	}
	return recheck.Target{Repo: q.Repo, Check: func(ctx context.Context) revalidate.Result {
		// As for Comments, the page is at least as recent as what the
		// list showed before the request.
		m, _ := s.seen.Get(detailKey(q.Repo, q.Number))
		_, res := recheck.Check(ctx, s.comments, s.keptComments, key, SyncKey(q.Repo), s.loadComments(q, m.updated))
		return res
	}}, true
}

// parseCommentsKey returns the query that CommentsQuery.key made key of.
// The repository comes back in lower case, which GitHub doesn't mind.
func parseCommentsKey(key string) (CommentsQuery, bool) {
	pull, query, ok := strings.Cut(strings.TrimPrefix(key, "pull:"), "/comments?")
	if !ok || !strings.HasPrefix(key, "pull:") {
		return CommentsQuery{}, false
	}
	name, num, ok := strings.Cut(pull, "#")
	if !ok {
		return CommentsQuery{}, false
	}
	repo, err := core.ParseRepoRef(name)
	number, nerr := strconv.Atoi(num)
	v, verr := url.ParseQuery(query)
	if err != nil || nerr != nil || verr != nil || number <= 0 {
		return CommentsQuery{}, false
	}
	size, err := strconv.Atoi(v.Get("first"))
	if err != nil || size <= 0 {
		return CommentsQuery{}, false
	}
	q := CommentsQuery{Repo: repo, Number: number, Cursor: v.Get("cursor"), PageSize: size}
	// A key that doesn't make itself again isn't one of the service's.
	return q, q.key(size) == key
}
