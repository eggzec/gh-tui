package obs

import (
	"cmp"
	"context"
	"iter"
	"log/slog"
	"math"
	"slices"
	"sync"
	"time"
)

// Summary is what the stats counted since they started. Its fields are
// logged as they are, so their JSON names are the record's keys.
type Summary struct {
	UptimeS    float64         `json:"uptime_s"`
	Quotas     []QuotaSummary  `json:"quotas"`
	Routes     []RouteSummary  `json:"routes"`
	Waits      WaitSummary     `json:"http_waits"`
	Cache      []CacheSummary  `json:"cache"`
	Disk       []DiskSummary   `json:"disk"`
	Prefetch   []PrefetchStats `json:"prefetch"`
	Budget     BudgetSummary   `json:"prefetch_budget"`
	Revalidate RevalSummary    `json:"revalidate"`
}

// QuotaSummary covers the requests of one API against one rate-limit
// resource, such as rest and core, and what the last of them left.
type QuotaSummary struct {
	API      string `json:"api"`
	Resource string `json:"resource"`
	Requests int64  `json:"requests"`
	// NotModified counts the 304s, which cost nothing against the quota,
	// and NotModifiedShare is their share of the requests.
	NotModified      int64   `json:"not_modified"`
	NotModifiedShare float64 `json:"not_modified_share"`
	Failed           int64   `json:"failed"`
	Bytes            int64   `json:"bytes"`
	// Cost is the GraphQL points the queries cost.
	Cost      int64      `json:"cost,omitempty"`
	Limit     int        `json:"limit,omitempty"`
	Remaining int        `json:"remaining,omitempty"`
	Used      int        `json:"used,omitempty"`
	Reset     *time.Time `json:"reset,omitempty"`
	// SeenAt is when a response last reported the quota.
	SeenAt *time.Time `json:"seen_at,omitempty"`
}

// RouteSummary covers the requests of one route, with their latencies.
type RouteSummary struct {
	API         string  `json:"api"`
	Method      string  `json:"method"`
	Route       string  `json:"route"`
	Requests    int64   `json:"requests"`
	NotModified int64   `json:"not_modified"`
	Failed      int64   `json:"failed"`
	Bytes       int64   `json:"bytes"`
	P50MS       float64 `json:"p50_ms"`
	P95MS       float64 `json:"p95_ms"`
	MaxMS       float64 `json:"max_ms"`
}

// WaitSummary covers the requests that waited to be sent, since as many
// as the client lets be in flight at once were.
type WaitSummary struct {
	Requests int64   `json:"requests"`
	TotalMS  float64 `json:"total_ms"`
	MaxMS    float64 `json:"max_ms"`
}

// CacheSummary covers the reads of the in-memory cache of one kind of key,
// such as pulls or issue.
type CacheSummary struct {
	Kind string `json:"kind"`
	Hit  int64  `json:"hit"`
	Miss int64  `json:"miss"`
	// Shared counts the reads that joined a fetch in flight.
	Shared int64 `json:"shared"`
	// HitRatio is Hit over the reads.
	HitRatio    float64 `json:"hit_ratio"`
	Revalidated int64   `json:"revalidated"`
	Seeded      int64   `json:"seeded"`
	StaleServed int64   `json:"stale_served"`
	Evicted     int64   `json:"evicted"`
}

// DiskSummary covers the reads of the disk layer of one kind of object.
type DiskSummary struct {
	Kind     string  `json:"kind"`
	Hit      int64   `json:"hit"`
	Miss     int64   `json:"miss"`
	HitRatio float64 `json:"hit_ratio"`
}

// PrefetchStats covers the reads ahead of one kind, such as pull or
// file, and how many of those that brought something were then opened.
type PrefetchStats struct {
	Kind        string `json:"kind"`
	Sent        int64  `json:"sent"`
	Cached      int64  `json:"skipped_cached"`
	Limited     int64  `json:"skipped_limit"`
	Canceled    int64  `json:"canceled"`
	RateLimited int64  `json:"rate_limited"`
	Failed      int64  `json:"failed"`
	OverBudget  int64  `json:"skipped_budget"`
	Read        int64  `json:"read"`
	Opened      int64  `json:"opened"`
	// Useful is Opened over Read.
	Useful float64 `json:"useful_share"`
}

// BudgetSummary covers the GraphQL points the reads ahead spent out of
// their budget, since it last started over, and whether they stopped for
// it, until when.
type BudgetSummary struct {
	Points int64      `json:"points"`
	Budget int64      `json:"budget"`
	Spent  bool       `json:"spent"`
	Until  *time.Time `json:"until,omitempty"`
}

// RevalSummary covers the revalidator's passes and how much of their
// budget they spent.
type RevalSummary struct {
	Passes int64 `json:"passes"`
	Sent   int64 `json:"sent"`
	Budget int64 `json:"budget"`
	// BudgetUsed is Sent over Budget.
	BudgetUsed float64 `json:"budget_used_share"`
}

// Summary returns what s counted so far.
func (s *Stats) Summary() Summary {
	out := Summary{UptimeS: math.Round(time.Since(s.start).Seconds())}
	s.mu.Lock()
	for k, q := range s.quotas {
		qs := QuotaSummary{
			API: k.api, Resource: cmp.Or(k.resource, "none"),
			Requests: q.requests, NotModified: q.notModified, NotModifiedShare: ratio(q.notModified, q.requests),
			Failed: q.failed, Bytes: q.bytes, Cost: q.cost,
		}
		if k.resource != "" {
			reset, seen := q.last.Reset, q.lastAt
			qs.Limit, qs.Remaining, qs.Used, qs.Reset, qs.SeenAt = q.last.Limit, q.last.Remaining, q.last.Used, &reset, &seen
		}
		out.Quotas = append(out.Quotas, qs)
	}
	for k, r := range s.routes {
		samples := slices.Clone(r.samples)
		slices.Sort(samples)
		out.Routes = append(out.Routes, RouteSummary{
			API: k.api, Method: k.method, Route: k.route,
			Requests: r.requests, NotModified: r.notModified, Failed: r.failed, Bytes: r.bytes,
			P50MS: Percentile(samples, 0.50), P95MS: Percentile(samples, 0.95), MaxMS: r.maxMS,
		})
	}
	s.mu.Unlock()
	slices.SortFunc(out.Quotas, func(a, b QuotaSummary) int {
		return cmp.Or(cmp.Compare(a.API, b.API), cmp.Compare(a.Resource, b.Resource))
	})
	slices.SortFunc(out.Routes, func(a, b RouteSummary) int {
		return cmp.Or(cmp.Compare(a.API, b.API), cmp.Compare(a.Route, b.Route), cmp.Compare(a.Method, b.Method))
	})

	out.Waits = WaitSummary{
		Requests: s.waits.Load(),
		TotalMS:  Millis(time.Duration(s.waited.Load())),
		MaxMS:    Millis(time.Duration(s.maxWait.Load())),
	}

	for kind, c := range each[CacheEvent](&s.cache) {
		hit, miss, shared := c.get(MemoryHit), c.get(MemoryMiss), c.get(Shared)
		if hit+miss+shared+c.get(Seeded)+c.get(Evicted) > 0 {
			out.Cache = append(out.Cache, CacheSummary{
				Kind: kind, Hit: hit, Miss: miss, Shared: shared, HitRatio: ratio(hit, hit+miss+shared),
				Revalidated: c.get(Revalidated), Seeded: c.get(Seeded), StaleServed: c.get(StaleServed), Evicted: c.get(Evicted),
			})
		}
		if hit, miss := c.get(DiskHit), c.get(DiskMiss); hit+miss > 0 {
			out.Disk = append(out.Disk, DiskSummary{Kind: kind, Hit: hit, Miss: miss, HitRatio: ratio(hit, hit+miss)})
		}
	}
	for kind, c := range each[PrefetchEvent](&s.prefetch) {
		read, opened := c.get(PrefetchRead), c.get(PrefetchOpened)
		out.Prefetch = append(out.Prefetch, PrefetchStats{
			Kind: kind, Sent: c.get(PrefetchSent), Cached: c.get(PrefetchCached), Limited: c.get(PrefetchLimited),
			Canceled: c.get(PrefetchCanceled), RateLimited: c.get(PrefetchRateLimited), Failed: c.get(PrefetchFailed),
			OverBudget: c.get(PrefetchOverBudget), Read: read, Opened: opened, Useful: ratio(opened, read),
		})
	}
	slices.SortFunc(out.Cache, func(a, b CacheSummary) int { return cmp.Compare(a.Kind, b.Kind) })
	slices.SortFunc(out.Disk, func(a, b DiskSummary) int { return cmp.Compare(a.Kind, b.Kind) })
	slices.SortFunc(out.Prefetch, func(a, b PrefetchStats) int { return cmp.Compare(a.Kind, b.Kind) })

	limit, _ := s.prefetchLimit()
	s.budgetMu.Lock()
	s.renewPrefetch(time.Now())
	out.Budget = BudgetSummary{Points: s.budget.points, Budget: limit, Spent: s.budget.spent}
	if s.budget.spent {
		renew := s.budget.renew
		out.Budget.Until = &renew
	}
	s.budgetMu.Unlock()

	sent, budget := s.revalSent.Load(), s.revalBudget.Load()
	out.Revalidate = RevalSummary{Passes: s.passes.Load(), Sent: sent, Budget: budget, BudgetUsed: ratio(sent, budget)}
	return out
}

// Log logs the summary of s at info level, as a record with the message
// "summary".
func (s *Stats) Log(ctx context.Context) {
	sum := s.Summary()
	slog.InfoContext(ctx, "summary",
		slog.Float64("uptime_s", sum.UptimeS),
		slog.Any("quotas", sum.Quotas),
		slog.Any("routes", sum.Routes),
		slog.Any("http_waits", sum.Waits),
		slog.Any("cache", sum.Cache),
		slog.Any("disk", sum.Disk),
		slog.Any("prefetch", sum.Prefetch),
		slog.Any("prefetch_budget", sum.Budget),
		slog.Any("revalidate", sum.Revalidate),
	)
}

// Summarize logs the summary of the default stats every interval until
// ctx is done. It returns at once if interval is not positive.
func Summarize(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			Default().Log(ctx)
		}
	}
}

// Percentile returns the p-th quantile of sorted, 0 < p <= 1, by the
// nearest rank, or 0 when sorted is empty.
func Percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	i := int(math.Ceil(p*float64(len(sorted)))) - 1
	return sorted[min(max(i, 0), len(sorted)-1)]
}

// ratio returns n over total to three decimals, or 0 without a total.
func ratio(n, total int64) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(n)/float64(total)*1000) / 1000
}

// each yields the counters of m by kind.
func each[N ~int](m *sync.Map) iter.Seq2[string, *counters[N]] {
	return func(yield func(string, *counters[N]) bool) {
		m.Range(func(k, v any) bool { return yield(k.(string), v.(*counters[N])) })
	}
}
