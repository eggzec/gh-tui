// Package refs reads the issues and pull requests linked to one: those it
// closes or is closed by, those that its body, comments or reviews write
// the number or link of, and those that mention it. It reads them through
// a cache, as a whole for the first two and a page at a time for the
// mentions, which a busy item has by the hundred.
package refs

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/fallback"
)

// API is the part of the GitHub client the service uses.
type API interface {
	References(ctx context.Context, repo core.RepoRef, number int, pull bool) (core.RefSources, error)
	ResolveReferences(ctx context.Context, targets []core.Target) ([]core.Reference, error)
	Mentions(ctx context.Context, repo core.RepoRef, number int, pull bool, before string, last int) (core.Page[core.Reference], error)
}

// Query selects the links of a pull request or issue.
type Query struct {
	Repo   core.RepoRef
	Number int
	// Pull says that the item is a pull request, which links what it
	// closes, and not an issue, which links what closes it.
	Pull bool
	// Updated is when GitHub says the item last changed, as its list row or
	// detail has it, or zero if the caller doesn't know. A read made for an
	// earlier time is read again, so a new comment or an edit shows without
	// a call of Invalidate, which stays for a refresh the user asks for.
	Updated time.Time
	// Again reads past a kept value: set it on the read that follows one
	// that came back Stale. It doesn't key the cache.
	Again bool
}

// MentionsQuery selects a page of the items that mention a pull request
// or issue, the newest first.
type MentionsQuery struct {
	Repo   core.RepoRef
	Number int
	Pull   bool
	// Cursor is the Next of the previous page, or empty for the first page.
	Cursor string
	// PageSize is how many items a page holds. Zero means the service's
	// size, and sizes above GitHub's maximum of 100 are clamped.
	PageSize int
	// Again works as in Query.
	Again bool
}

// Service reads the links of pull requests and issues. It is safe for
// concurrent use.
type Service struct {
	api      API
	host     string
	pageSize int
	refs     reads[core.References]
	mentions reads[core.Page[core.Reference]]
	// chain holds, by the query of a page of mentions, the query of the
	// page before it, as pages are read in order. It lets a page leave out
	// the items the pages before it list. Pages read in an earlier session
	// aren't in it, so such a page may repeat an item of an earlier one.
	chain sync.Map
}

// The kinds of entries the service keeps in its store, and the version of
// their values. Bump schema when a core type they hold changes shape.
const (
	kindRefs     = "references"
	kindMentions = "refmentions"
	schema       = 1
)

// maxPageSize is the most items GitHub serves in a page.
const maxPageSize = 100

// New returns a Service that reads through api.
func New(api API, opts ...Option) *Service {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	d := config.Default()
	o.ttl = cmp.Or(o.ttl, d.Cache.TTL.References)
	o.capacity = cmp.Or(o.capacity, d.Cache.Memory.Entries)
	return &Service{
		api:      api,
		host:     cmp.Or(o.host, core.DefaultHost),
		pageSize: cmp.Or(o.pageSize, d.PageSize.References),
		refs: newReads(o, kindRefs, func(r *core.References) (*bool, *bool, *bool) {
			return &r.Stale, &r.Offline, &r.Limited
		}),
		mentions: newReads(o, kindMentions, func(p *core.Page[core.Reference]) (*bool, *bool, *bool) {
			return &p.Stale, &p.Offline, &p.Limited
		}),
	}
}

func (q Query) key() string {
	return "refs:" + kindName(q.Pull) + ":" + core.RefKey(core.Target{Repo: q.Repo, Number: q.Number})
}

func (q MentionsQuery) key() string {
	v := url.Values{"before": {q.Cursor}, "last": {strconv.Itoa(q.PageSize)}}
	// Its own kind of entry, which the cache counts apart from the links.
	return "refmentions:" + kindName(q.Pull) + ":" + core.RefKey(core.Target{Repo: q.Repo, Number: q.Number}) + "?" + v.Encode()
}

func (q MentionsQuery) normalize(size int) MentionsQuery {
	q.PageSize = min(cmp.Or(max(q.PageSize, 0), size), maxPageSize)
	return q
}

func kindName(pull bool) string {
	if pull {
		return "pull"
	}
	return "issue"
}

// itemTag marks the entries of the item, so that Invalidate finds them.
func itemTag(repo core.RepoRef, number int) string {
	return "refs:" + core.RefKey(core.Target{Repo: repo, Number: number})
}

// CachedReferences returns the links of q if they are cached, fresh or
// stale, without I/O.
func (s *Service) CachedReferences(q Query) (core.References, bool) {
	return s.refs.cached(q.key())
}

// References returns what is linked to the item of q: what it closes or
// is closed by, and what its texts link. They are fresh for the TTL. What
// an earlier session kept comes back at once with Stale set once the TTL
// has passed, until a read with q.Again set reads it again, in full, for
// GraphQL has no validators. If GitHub can't be reached, the last read
// is served with Offline set, and with Limited set if it rate limited the
// read. An item that isn't there fails with an error matching
// core.ErrNotFound.
//
// The items that mention it are not read: Mentioned counts them, and
// Mentions reads them a page at a time.
func (s *Service) References(ctx context.Context, q Query) (core.References, error) {
	whole := whole(func(ctx context.Context) (core.References, error) { return s.read(ctx, q) })
	load := func(ctx context.Context, prev cache.Entry[core.References], ok bool) (cache.Entry[core.References], error) {
		e, err := whole(ctx, prev, ok)
		if err == nil && e.Value.Failed > 0 {
			// Some links weren't read: the value is served, but stale at once,
			// so the next read tries again instead of waiting out the TTL.
			e.FetchedAt = time.Unix(1, 0)
		}
		return e, err
	}
	get := func(again bool) (core.References, error) {
		// The client already names the request in its error.
		return s.refs.get(ctx, q.key(), again, tagged(itemTag(q.Repo, q.Number), load))
	}
	r, err := get(q.Again)
	if err != nil || q.Updated.IsZero() || q.Again || !r.ItemUpdated.Before(q.Updated) {
		return r, err
	}
	// What was read, here or by an earlier session, is of an older item.
	s.Invalidate(q.Repo, q.Number)
	return get(true)
}

// read reads the links of q from GitHub: the item's own, then the items
// its texts name, which only their numbers say what they are.
func (s *Service) read(ctx context.Context, q Query) (core.References, error) {
	src, err := s.api.References(ctx, q.Repo, q.Number, q.Pull)
	if err != nil {
		return core.References{}, err
	}
	self := core.Target{Repo: q.Repo, Number: q.Number}
	p := merge(src, self, q.Pull, s.host)
	out := core.References{
		Closing: p.closing, Mentioned: src.Mentioned,
		CommentsRead: src.CommentsRead, CommentsTotal: src.CommentsTotal,
		ReviewsRead: src.ReviewsRead, ReviewsTotal: src.ReviewsTotal,
		ReadAt: time.Now(),
	}
	out.ItemUpdated = q.Updated
	pending := p.pending
	if len(pending) > maxResolve {
		out.Unresolved = len(pending) - maxResolve
		pending = pending[:maxResolve]
	}
	if len(pending) > 0 {
		resolved, failed, why, first, err := s.resolve(ctx, pending)
		if err != nil {
			return core.References{}, err
		}
		out.Failed, out.FailedWhy = failed, why
		out.Limited = errors.Is(first, core.ErrRateLimited)
		out.Written = s.written(&out, self, pending, resolved)
	}
	unreadable := 0
	for i := range out.Written {
		if out.Written[i].Problem != "" {
			unreadable++
		}
	}
	slog.InfoContext(ctx, "references", "span", "service.refs", "repo", q.Repo.String(), "number", q.Number,
		"closing", len(out.Closing), "written", len(out.Written), "mentioned", out.Mentioned,
		"resolved", len(out.Written)-unreadable, "unreadable", unreadable)
	return out, nil
}

// resolveWorkers is how many batches of links are read at once.
const resolveWorkers = 3

// resolve reads the items pending names, in batches that go at the same
// time, and returns what it read aligned with pending: an item whose batch
// failed has no Target. failed counts them, and why says why the first
// failed. It fails only when no batch was read, or ctx is done, so what
// earlier batches read isn't lost to a later one that was refused.
func (s *Service) resolve(ctx context.Context, pending []core.Reference) (resolved []core.Reference, failed int, why string, first, err error) {
	resolved = make([]core.Reference, len(pending))
	nbatch := (len(pending) + github.RefSlots - 1) / github.RefSlots
	errs := make([]error, nbatch)
	var wg sync.WaitGroup
	sem := make(chan struct{}, resolveWorkers)
	for b := range nbatch {
		lo, hi := b*github.RefSlots, min((b+1)*github.RefSlots, len(pending))
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			targets := make([]core.Target, hi-lo)
			for i := range targets {
				targets[i] = pending[lo+i].Target
			}
			refs, err := s.api.ResolveReferences(ctx, targets)
			if err == nil && len(refs) != len(targets) {
				err = fmt.Errorf("resolve references: %d answers for %d items", len(refs), len(targets))
			}
			if err != nil {
				errs[b] = err
				return
			}
			copy(resolved[lo:hi], refs)
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, 0, "", nil, err
	}
	for b, e := range errs {
		if e == nil {
			continue
		}
		if first == nil {
			first = e
		}
		failed += min((b+1)*github.RefSlots, len(pending)) - b*github.RefSlots
	}
	if failed == len(pending) {
		return nil, 0, "", nil, first
	}
	if first != nil {
		why = core.KindOf(first).String()
	}
	return resolved, failed, why, first, nil
}

// written joins what was read of the items the texts name with where each
// was written. One that turns out to be linked by GitHub joins that, and
// two that turn out to be one item are one.
func (s *Service) written(out *core.References, self core.Target, pending, resolved []core.Reference) []core.Reference {
	closing := map[string]int{}
	for i := range out.Closing {
		closing[core.RefKey(out.Closing[i].Target)] = i
	}
	at := map[string]int{}
	var written []core.Reference
	for i := range resolved {
		if resolved[i].Target == (core.Target{}) {
			// Its batch failed.
			continue
		}
		ref := resolved[i]
		ref.Origins = refOrigins(pending[i].Origins)
		k := core.RefKey(ref.Target)
		switch j, inClosing := closing[k]; {
		case ref.Target.Same(self):
		case inClosing:
			out.Closing[j].Origins = refOrigins(append(out.Closing[j].Origins, ref.Origins...))
		default:
			if w, ok := at[k]; ok {
				written[w].Origins = refOrigins(append(written[w].Origins, ref.Origins...))
				continue
			}
			at[k] = len(written)
			written = append(written, ref)
		}
	}
	return written
}

// CachedMentions returns the page of q if it is cached, fresh or stale,
// without I/O. The items that are in the item's other groups are left
// out.
func (s *Service) CachedMentions(q MentionsQuery) (core.Page[core.Reference], bool) {
	q = q.normalize(s.pageSize)
	p, ok := s.mentions.cached(q.key())
	if !ok {
		return p, false
	}
	s.promote(q, p.Items)
	s.link(q, p)
	return s.leaveOut(q, p), true
}

// Mentions returns the page of the items that mention the item of q, the
// newest first. A page is read as References is, apart from the others,
// and an item whose mention closes this one moves to the Closing of its
// References, if they are cached. Items that References lists are left
// out of the page; the pages are then shorter than their size, and
// GitHub's count is the one References gives.
func (s *Service) Mentions(ctx context.Context, q MentionsQuery) (core.Page[core.Reference], error) {
	q = q.normalize(s.pageSize)
	load := whole(func(ctx context.Context) (core.Page[core.Reference], error) {
		return s.api.Mentions(ctx, q.Repo, q.Number, q.Pull, q.Cursor, q.PageSize)
	})
	p, err := s.mentions.get(ctx, q.key(), q.Again, tagged(itemTag(q.Repo, q.Number), load))
	if err != nil {
		return core.Page[core.Reference]{}, err
	}
	s.link(q, p)
	if s.promote(q, p.Items) {
		s.keepRefs(Query{Repo: q.Repo, Number: q.Number, Pull: q.Pull})
	}
	return s.leaveOut(q, p), nil
}

// link records that the page after p, if any, follows the page of q.
func (s *Service) link(q MentionsQuery, p core.Page[core.Reference]) {
	if p.Next == "" {
		return
	}
	next := q
	next.Cursor = p.Next
	s.chain.Store(next.key(), q)
}

// leaveOut returns p without the items that References of the same item
// lists, if they are cached, and without those that the pages before it
// list. An item that p lists twice is one, with both mentions.
func (s *Service) leaveOut(q MentionsQuery, p core.Page[core.Reference]) core.Page[core.Reference] {
	listed := map[string]bool{}
	if r, ok := s.refs.cached(Query{Repo: q.Repo, Number: q.Number, Pull: q.Pull}.key()); ok {
		for _, group := range [][]core.Reference{r.Closing, r.Written} {
			for i := range group {
				listed[core.RefKey(group[i].Target)] = true
			}
		}
	}
	// Pages are read in order, so the chain ends at the first; the bound
	// only guards against a cursor that GitHub repeats.
	for cur, n := q, 0; n < maxChain; n++ {
		prev, ok := s.chain.Load(cur.key())
		if !ok {
			break
		}
		cur = prev.(MentionsQuery)
		if before, ok := s.mentions.cached(cur.key()); ok {
			for i := range before.Items {
				listed[core.RefKey(before.Items[i].Target)] = true
			}
		}
	}
	items := make([]core.Reference, 0, len(p.Items))
	at := map[string]int{}
	for i := range p.Items {
		k := core.RefKey(p.Items[i].Target)
		if listed[k] {
			continue
		}
		if j, ok := at[k]; ok {
			items[j].Origins = refOrigins(append(slices.Clone(items[j].Origins), p.Items[i].Origins...))
			continue
		}
		at[k] = len(items)
		items = append(items, p.Items[i])
	}
	p.Items = items
	return p
}

// maxChain bounds how many pages leaveOut looks back over.
const maxChain = 10000

// promote moves the items whose mention closes the item of q into the
// Closing of its cached References, and reports whether it changed them.
// An item there already, in Closing or in Written, is joined or moved:
// the strongest group has it.
func (s *Service) promote(q MentionsQuery, items []core.Reference) bool {
	key := Query{Repo: q.Repo, Number: q.Number, Pull: q.Pull}.key()
	e, state := s.refs.mem.Get(key)
	if state == cache.Miss {
		return false
	}
	r := e.Value
	changed := false
	for i := range items {
		item := &items[i]
		closes := slices.IndexFunc(item.Origins, func(o core.RefOrigin) bool { return o.Group == core.RefClosing })
		if closes < 0 {
			continue
		}
		k := core.RefKey(item.Target)
		has := func(ref core.Reference) bool { return core.RefKey(ref.Target) == k }
		if i := slices.IndexFunc(r.Closing, has); i >= 0 {
			if slices.Contains(r.Closing[i].Origins, item.Origins[closes]) {
				continue
			}
			r.Closing = slices.Clone(r.Closing)
			r.Closing[i].Origins = refOrigins(append(slices.Clip(r.Closing[i].Origins), item.Origins[closes]))
			changed = true
			continue
		}
		moved := *item
		if i := slices.IndexFunc(r.Written, has); i >= 0 {
			moved.Origins = append(slices.Clone(item.Origins), r.Written[i].Origins...)
			r.Written = slices.Delete(slices.Clone(r.Written), i, i+1)
		}
		moved.Origins = refOrigins(moved.Origins)
		r.Closing = append(slices.Clip(r.Closing), moved)
		slices.SortStableFunc(r.Closing, refClosingOrder)
		changed = true
	}
	if !changed {
		return false
	}
	// A read of the links that finishes between the Get above and this Set
	// is overwritten, and that read had the closing link too, or it is
	// found again by the next page that is read: the loss is benign.
	e.Value = r
	s.refs.mem.Set(key, e)
	if state == cache.Stale {
		s.refs.mem.Invalidate(key)
	}
	return true
}

// keepRefs keeps the cached References of q in the store again, after
// promote changed them.
func (s *Service) keepRefs(q Query) {
	if e, state := s.refs.mem.Get(q.key()); state != cache.Miss {
		_ = s.refs.kept.Save(q.key(), e)
	}
}

// Invalidate marks everything cached of the item number of repo stale:
// its links and every page of its mentions. They are still served by the
// Cached reads, and the next read of each goes to GitHub.
func (s *Service) Invalidate(repo core.RepoRef, number int) {
	tag := itemTag(repo, number)
	s.refs.mem.InvalidateTag(tag)
	s.mentions.mem.InvalidateTag(tag)
	// The pages are read again, in order, and link themselves anew.
	mine := ":" + core.RefKey(core.Target{Repo: repo, Number: number}) + "?"
	s.chain.Range(func(k, _ any) bool {
		if strings.Contains(k.(string), mine) {
			s.chain.Delete(k)
		}
		return true
	})
}

// reads is one kind of read: its cache in memory, its shelf in the store,
// and how to mark a value served stale, offline or limited.
type reads[V any] struct {
	mem   *cache.Cache[V]
	kept  *cache.Shelf[V]
	flags func(*V) (stale, offline, limited *bool)
}

func newReads[V any](o options, kind string, flags func(*V) (stale, offline, limited *bool)) reads[V] {
	return reads[V]{
		mem:   cache.New[V](cache.WithTTL(o.ttl), cache.WithCapacity(o.capacity)),
		kept:  cache.NewShelf[V](o.store, kind, schema),
		flags: flags,
	}
}

// cached returns the value under key in memory, fresh or stale, without
// I/O.
func (r *reads[V]) cached(key string) (V, bool) {
	e, state := r.mem.Get(key)
	return e.Value, state != cache.Miss
}

// get returns the value under key. A fresh value in memory is returned
// without a request, and so is one kept by an earlier session within the
// TTL. An older kept one is returned at once, marked stale, until a read
// with again set loads it. Otherwise get loads the value, stores it and
// keeps it, falling back on the last one as fallback.Fetch does.
func (r *reads[V]) get(ctx context.Context, key string, again bool, load cache.FetchFunc[V]) (V, error) {
	if e, ok := r.kept.Warm(r.mem, key, again); ok {
		v := e.Value
		stale, _, _ := r.flags(&v)
		*stale = true
		return v, nil
	}
	e, err := fallback.Fetch(ctx, r.mem, r.kept, key, r.marks, fallback.Keep(r.kept, key, load))
	return e.Value, err
}

// marks are the fallback.Marks of the values.
func (r *reads[V]) marks(v *V) (offline, limited *bool) {
	_, offline, limited = r.flags(v)
	return offline, limited
}

// whole adapts fetch, a read without validators such as a GraphQL query,
// to a cache.FetchFunc: a stale value is read again in full.
func whole[V any](fetch func(context.Context) (V, error)) cache.FetchFunc[V] {
	return func(ctx context.Context, _ cache.Entry[V], _ bool) (cache.Entry[V], error) {
		v, err := fetch(ctx)
		if err != nil {
			return cache.Entry[V]{}, err
		}
		return cache.Entry[V]{Value: v}, nil
	}
}

// tagged returns load, whose entries it tags as the item's.
func tagged[V any](tag string, load cache.FetchFunc[V]) cache.FetchFunc[V] {
	return func(ctx context.Context, prev cache.Entry[V], ok bool) (cache.Entry[V], error) {
		e, err := load(ctx, prev, ok)
		if err == nil && !slices.Contains(e.Tags, tag) {
			e.Tags = append(slices.Clip(e.Tags), tag)
		}
		return e, err
	}
}
