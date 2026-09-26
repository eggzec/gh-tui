package obs

import (
	"math/rand/v2"
	"sync"
	"sync/atomic"
	"time"
)

// Stats counts what the summary reports: the requests sent and the quotas
// they left, how the caches served reads, how much of what was read ahead
// got used, and how much of its budget the revalidator spent. Counting is
// cheap: the counters that reads bump are atomic, and only the per-request
// ones take a lock. Stats is safe for concurrent use.
type Stats struct {
	start time.Time

	mu     sync.Mutex
	quotas map[quotaKey]*quotaStat
	routes map[routeKey]*routeStat

	// Counters by kind: *counters[CacheEvent] and *counters[PrefetchEvent].
	cache    sync.Map
	prefetch sync.Map

	passes      atomic.Int64
	revalSent   atomic.Int64
	revalBudget atomic.Int64

	// waits counts the requests that waited for a slot to be sent, and
	// waited their total and longest wait in nanoseconds.
	waits, waited, maxWait atomic.Int64
}

// NewStats returns empty stats that measure uptime from now.
func NewStats() *Stats {
	return &Stats{
		start:  time.Now(),
		quotas: make(map[quotaKey]*quotaStat),
		routes: make(map[routeKey]*routeStat),
	}
}

var std atomic.Pointer[Stats]

func init() { std.Store(NewStats()) }

// Default returns the stats that the package's counting functions add to.
func Default() *Stats { return std.Load() }

// SetDefault makes s the stats that the package's counting functions add
// to, such as for a test, and returns the previous ones.
func SetDefault(s *Stats) *Stats { return std.Swap(s) }

// APIs of an HTTP record.
const (
	REST    = "rest"
	GraphQL = "graphql"
)

// HTTP is one HTTP attempt as the stats count it.
type HTTP struct {
	// API is REST or GraphQL.
	API    string
	Method string
	// Route is the path with its variable parts replaced, such as
	// /repos/{owner}/{repo}/issues, or the operation of a GraphQL query.
	Route string
	// Status is 0 when no response came.
	Status      int
	NotModified bool
	Duration    time.Duration
	Bytes       int64
	Rate        Rate
	// Cost is what a GraphQL query cost in points, or 0 if unknown.
	Cost int
}

// Rate is the quota a response reported. A zero Resource means none.
type Rate struct {
	Resource  string
	Limit     int
	Remaining int
	Used      int
	Reset     time.Time
}

type quotaKey struct{ api, resource string }

type quotaStat struct {
	requests, notModified, failed, bytes, cost int64
	last                                       Rate
	lastAt                                     time.Time
}

type routeKey struct{ api, method, route string }

// maxSamples bounds the latencies kept per route. Past it, the samples are
// a uniform reservoir of all, so percentiles stay representative.
const maxSamples = 2048

type routeStat struct {
	requests, notModified, failed, bytes int64
	maxMS                                float64
	samples                              []float64
}

// HTTP counts one HTTP attempt.
func (s *Stats) HTTP(h HTTP) {
	ms := Millis(h.Duration)
	failed := h.Status == 0 || h.Status >= 400
	s.mu.Lock()
	defer s.mu.Unlock()

	qk := quotaKey{h.API, h.Rate.Resource}
	q := s.quotas[qk]
	if q == nil {
		q = new(quotaStat)
		s.quotas[qk] = q
	}
	q.requests++
	q.bytes += h.Bytes
	q.cost += int64(h.Cost)
	if h.NotModified {
		q.notModified++
	}
	if failed {
		q.failed++
	}
	if h.Rate.Resource != "" {
		q.last, q.lastAt = h.Rate, time.Now()
	}

	rk := routeKey{h.API, h.Method, h.Route}
	r := s.routes[rk]
	if r == nil {
		r = new(routeStat)
		s.routes[rk] = r
	}
	r.requests++
	r.bytes += h.Bytes
	if h.NotModified {
		r.notModified++
	}
	if failed {
		r.failed++
	}
	r.maxMS = max(r.maxMS, ms)
	switch {
	case len(r.samples) < maxSamples:
		r.samples = append(r.samples, ms)
	default:
		if i := rand.Int64N(r.requests); i < maxSamples {
			r.samples[i] = ms
		}
	}
}

// CacheEvent is what a cache did with a read, or with an entry.
type CacheEvent int

// Cache events. Memory ones are counted per kind of key, disk ones per
// kind of object.
const (
	// MemoryHit is a read served from memory without a request.
	MemoryHit CacheEvent = iota
	// MemoryMiss is a read that fetched, because the entry was missing or
	// stale.
	MemoryMiss
	// Shared is a read that joined a fetch already running for its key.
	Shared
	// Revalidated is a fetch that found the entry still current.
	Revalidated
	// Seeded is an entry put in memory from what an earlier session kept.
	Seeded
	// StaleServed is a kept entry served at once while it is revalidated,
	// counted once however many readers it is served to.
	StaleServed
	// Evicted is an entry that made room for others.
	Evicted
	// DiskHit and DiskMiss are reads of the disk layer.
	DiskHit
	DiskMiss
	numCache
)

// PrefetchEvent is what came of reading something ahead of its use.
type PrefetchEvent int

// Prefetch events.
const (
	// PrefetchSent is a read that was started.
	PrefetchSent PrefetchEvent = iota
	// PrefetchCached is a read skipped since the cache held it.
	PrefetchCached
	// PrefetchLimited is a read skipped since GitHub reported the rate
	// limit.
	PrefetchLimited
	// PrefetchCanceled is a read canceled, such as by leaving the list.
	PrefetchCanceled
	// PrefetchRateLimited is a read that GitHub refused with a rate limit.
	PrefetchRateLimited
	// PrefetchFailed is a read that failed otherwise.
	PrefetchFailed
	// PrefetchRead is a read that brought what it was after.
	PrefetchRead
	// PrefetchOpened is something read ahead that was then opened.
	PrefetchOpened
	numPrefetch
)

// counters holds a counter per event of a kind, for enums of up to 16 events.
type counters[N ~int] struct{ n [16]atomic.Int64 }

func (c *counters[N]) add(e N) { c.n[e].Add(1) }

func (c *counters[N]) get(e N) int64 { return c.n[e].Load() }

func count[N ~int](m *sync.Map, kind string, e N) {
	v, ok := m.Load(kind)
	if !ok {
		v, _ = m.LoadOrStore(kind, new(counters[N]))
	}
	v.(*counters[N]).add(e)
}

// Cache counts a cache event of kind.
func (s *Stats) Cache(kind string, e CacheEvent) { count(&s.cache, kind, e) }

// CacheCounters are the cache counters of one kind, for code that counts
// often enough to keep them rather than look them up each time.
type CacheCounters struct{ c *counters[CacheEvent] }

// CacheCounters returns the cache counters of kind.
func (s *Stats) CacheCounters(kind string) CacheCounters {
	v, ok := s.cache.Load(kind)
	if !ok {
		v, _ = s.cache.LoadOrStore(kind, new(counters[CacheEvent]))
	}
	return CacheCounters{v.(*counters[CacheEvent])}
}

// Add counts e.
func (c CacheCounters) Add(e CacheEvent) { c.c.add(e) }

// Prefetch counts a prefetch event of kind.
func (s *Stats) Prefetch(kind string, e PrefetchEvent) { count(&s.prefetch, kind, e) }

// Revalidate counts a pass of the revalidator that sent requests out of a
// budget of budget.
func (s *Stats) Revalidate(sent, budget int) {
	s.passes.Add(1)
	s.revalSent.Add(int64(sent))
	s.revalBudget.Add(int64(budget))
}

// HTTPWait counts a request that waited d for a slot among those the
// client lets be in flight at once.
func (s *Stats) HTTPWait(d time.Duration) {
	s.waits.Add(1)
	s.waited.Add(int64(d))
	for {
		m := s.maxWait.Load()
		if int64(d) <= m || s.maxWait.CompareAndSwap(m, int64(d)) {
			return
		}
	}
}

// CountHTTPWait counts a request that waited d to be sent in the default
// stats.
func CountHTTPWait(d time.Duration) { Default().HTTPWait(d) }

// CountHTTP counts an HTTP attempt in the default stats.
func CountHTTP(h HTTP) { Default().HTTP(h) }

// CountCache counts a cache event in the default stats.
func CountCache(kind string, e CacheEvent) { Default().Cache(kind, e) }

// CountPrefetch counts a prefetch event in the default stats.
func CountPrefetch(kind string, e PrefetchEvent) { Default().Prefetch(kind, e) }

// CountRevalidate counts a revalidation pass in the default stats.
func CountRevalidate(sent, budget int) { Default().Revalidate(sent, budget) }
