package files

import (
	"context"
	"iter"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/revalidate"
	"github.com/eggzec/gh-tui/internal/service/recheck"
)

// SyncKey names changes to where the refs of repo point in sync events,
// such as a branch that a force-push moved.
func SyncKey(repo core.RepoRef) string {
	return "files:" + repoID(repo)
}

// catalog is a Store that can go through its objects, and read and rewrite
// them without counting as a use, such as the disk layer of the cache.
type catalog interface {
	Store
	List(kind string) iter.Seq2[string, time.Time]
	Peek(kind, key string) ([]byte, bool)
	Replace(kind, key string, data []byte) error
}

// Kept lists the refs whose trees the service keeps, for a revalidator to
// check in the background whether they moved. Trees and blobs named by a
// SHA never change, so they need no check. Each check is one conditional
// request, which costs no rate limit when the ref didn't move. A ref that
// moved has its new tree cached and kept, and reports SyncKey of its
// repository. It reads the store, so call it where I/O is fine.
func (s *Service) Kept() []revalidate.Entry {
	cat, ok := s.store.(catalog)
	if !ok {
		return nil
	}
	var out []revalidate.Entry
	for name, usedAt := range cat.List(kindRef) {
		data, ok := cat.Peek(kindRef, name)
		if !ok {
			continue
		}
		r, err := decodeRef(data)
		if err != nil {
			continue
		}
		repo, err := core.ParseRepoRef(r.Repo)
		l, known := listers[r.Kind]
		// Records of the first version don't say what they are, and are
		// replaced by the next read of their ref.
		if err != nil || !known || r.Ref == "" || refKey(l.kind, repo, r.Ref) != name {
			continue
		}
		q := TreeQuery{Repo: repo, Ref: r.Ref}
		out = append(out, revalidate.Entry{
			ID:        kindRef + ":" + name,
			Repo:      repo,
			UsedAt:    usedAt,
			CheckedAt: r.CheckedAt,
			Check: func(ctx context.Context) revalidate.Result {
				return s.recheckRef(ctx, q, l, r)
			},
		})
	}
	return out
}

var listers = map[string]lister{kindTree: level, kindListing: recursive}

// recheckRef asks GitHub whether the ref of q moved from where r, its
// record, says it pointed: in memory, if the tree is there, and otherwise
// in the store only.
func (s *Service) recheckRef(ctx context.Context, q TreeQuery, l lister, r refRecord) revalidate.Result {
	key := l.key(q.Repo, q.Ref)
	var (
		found atomic.Int32
		was   string
		t     core.Tree
		err   error
	)
	switch e, st := s.refs.Get(key); st {
	case cache.Fresh:
		return revalidate.Result{Status: revalidate.Skipped}
	case cache.Stale:
		was = e.Value.SHA
		var e cache.Entry[core.Tree]
		e, err = s.refs.Fetch(ctx, key, s.refFetch(q, l, false, &found))
		t = e.Value
	case cache.Miss:
		was = r.SHA
		var e cache.Entry[core.Tree]
		// The tree the record names need not be read: GitHub answers
		// whether the ref still points at it either way.
		prev := cache.Entry[core.Tree]{Value: core.Tree{SHA: r.SHA}, ETag: r.ETag, LastModified: r.LastModified}
		fetch := s.refFetch(q, l, false, &found)
		e, err = fetch(ctx, prev, true)
		t = e.Value
	}
	switch {
	case err != nil:
		return recheck.Failure(ctx, err)
	case found.Load() == refUnasked:
		return revalidate.Result{Status: revalidate.Skipped}
	case !strings.EqualFold(t.SHA, was):
		return revalidate.Result{Status: revalidate.Changed, Sync: SyncKey(q.Repo)}
	default:
		return revalidate.Result{Status: revalidate.NotModified}
	}
}
