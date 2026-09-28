package issues

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/obs"
)

// GitHub numbers the issues and pull requests of a repository in one
// sequence, and a number never changes what it names. So once the service
// knows a number's kind it keeps it for good, in memory and on the store,
// with no validators: there is nothing to revalidate, and Kept doesn't
// list it. Kept kinds are evicted with the rest of the store, least
// recently used first. The issue endpoint answers for pull requests too,
// so one request resolves a number of either kind, and when it is an
// issue the same response is the issue as Get would read it.
//
// A repository deleted and made again under its name, or one moved away
// and replaced, may number anew, so a kind is only a hint of which reader
// to open: if it is wrong, that read fails as for any missing number.
//
// A cached issue says nothing of its kind, since GetIssue returns the
// issue part of a pull request too, so only kinds resolved here and the
// cached pull requests are trusted.

// PullCache is the part of the pull request service that Kind asks before
// it makes a request.
type PullCache interface {
	CachedGet(repo core.RepoRef, number int) (core.PullRequestDetail, bool)
}

// span names the service's layer in its log records.
const span = "service.issues"

// CachedKind reports whether number of repo is an issue or a pull request,
// if that is known without a request: from a number resolved before, or
// from the pull request's detail cached in memory. It reports false
// otherwise.
func (s *Service) CachedKind(repo core.RepoRef, number int) (core.NumberKind, bool) {
	k, _ := s.cachedKind(repo, number)
	return k, k.Known()
}

// cachedKind is CachedKind with where the kind was found, for the log.
func (s *Service) cachedKind(repo core.RepoRef, number int) (k core.NumberKind, found string) {
	if k, ok := s.kinds.get(numberKey(repo, number)); ok {
		return k, "memory"
	}
	if s.pulls != nil {
		if _, ok := s.pulls.CachedGet(repo, number); ok {
			return core.KindPull, "pull"
		}
	}
	return "", "miss"
}

// Kind returns whether number of repo is an issue or a pull request. It
// asks what CachedKind does, then what an earlier session kept, and only
// then GitHub, with one request. An issue that request brings is cached as
// Get would cache it; a pull request's issue part is not. It may read the
// store, so call it where I/O is fine.
//
// A number that is neither fails with a *core.NoNumberError, and so do a
// number the account may not see, which GitHub answers with a 404, and a
// deleted issue or any number of a repository with its issues turned off,
// which it answers with a 410. None is remembered, so the number is asked
// about again next time. Other failures, such as a rate limit, aren't
// remembered either. Without GitHub, only a kind known before is returned;
// otherwise the error satisfies github.Unreachable.
func (s *Service) Kind(ctx context.Context, repo core.RepoRef, number int) (core.NumberKind, error) {
	key := numberKey(repo, number)
	if k, found := s.cachedKind(repo, number); k.Known() {
		logKind(ctx, key, found)
		obs.CountCache(kindNumber, obs.MemoryHit)
		if found != "memory" {
			s.remember(key, k)
		}
		return k, nil
	}
	if e, ok := s.keptKinds.Load(key); ok && e.Value.Known() {
		logKind(ctx, key, "disk")
		s.kinds.set(key, e.Value)
		return e.Value, nil
	}
	logKind(ctx, key, "miss")
	obs.CountCache(kindNumber, obs.MemoryMiss)
	k, it, res, err := s.api.GetIssueKind(ctx, repo, number, github.Conditional{})
	switch {
	case missing(err):
		return "", fmt.Errorf("kind of %s#%d: %w", repo, number, &core.NoNumberError{Repo: repo, Number: number, Err: err})
	case err != nil:
		return "", fmt.Errorf("kind of %s#%d: %w", repo, number, err)
	case !k.Known():
		return "", fmt.Errorf("kind of %s#%d: GitHub didn't say", repo, number)
	}
	slog.InfoContext(ctx, "number resolved", "span", span, "repo", repo.String(), "number", number, "kind", string(k))
	s.remember(key, k)
	if k == core.KindIssue {
		s.warmIssue(ctx, repo, number, it, res)
	}
	return k, nil
}

// missing reports whether err is GitHub saying that a number is neither an
// issue nor a pull request the account may see.
func missing(err error) bool {
	if errors.Is(err, core.ErrNotFound) {
		return true
	}
	e, ok := errors.AsType[*github.Error](err)
	return ok && e.StatusCode == http.StatusGone
}

// remember keeps k as the kind of the number under key, for good.
func (s *Service) remember(key string, k core.NumberKind) {
	s.kinds.set(key, k)
	// The store is only a shortcut, so a failure is ignored.
	_ = s.keptKinds.Save(key, cache.Entry[core.NumberKind]{Value: k})
}

// warmIssue caches it, which Kind read from the issue endpoint with res,
// as Get would have cached the same response: under the issue's key, with
// its validators and tags, in memory and on the store. An issue cached
// already is left as it is, unless it is stale, when this newer copy
// replaces it.
func (s *Service) warmIssue(ctx context.Context, repo core.RepoRef, number int, it core.Issue, res github.Response) {
	key := issueKey(repo, number)
	e := cache.Entry[core.Issue]{
		Value: it, ETag: res.ETag, LastModified: res.LastModified, Source: res.URL,
		FetchedAt: time.Now(), Tags: []string{repoTag(repo), key},
	}
	_, _ = s.issues.Fetch(ctx, key, func(context.Context, cache.Entry[core.Issue], bool) (cache.Entry[core.Issue], error) {
		_ = s.keptIssues.Save(key, e)
		return e, nil
	})
}

// logKind logs at debug level where Kind found the kind of the number
// under key.
func logKind(ctx context.Context, key, found string) {
	if obs.Enabled(ctx, slog.LevelDebug) {
		slog.DebugContext(ctx, "number kind", "span", span, "key", key, "found", found)
	}
}

// numberKey is the key of a number's kind, in memory and on the store.
func numberKey(repo core.RepoRef, number int) string {
	return "number:" + repoID(repo) + "#" + strconv.Itoa(number)
}

// kindMemo holds the kinds of numbers by key. Its zero value is empty and
// ready to use, and it is safe for concurrent use. It is never trimmed:
// an entry is a few bytes, and only numbers someone asked about are in it.
type kindMemo struct {
	mu sync.Mutex
	m  map[string]core.NumberKind
}

func (m *kindMemo) get(key string) (core.NumberKind, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.m[key]
	return k, ok
}

func (m *kindMemo) set(key string, k core.NumberKind) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = make(map[string]core.NumberKind)
	}
	m.m[key] = k
}
