package obs

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	tests := []struct {
		p    float64
		want float64
	}{
		{0.5, 5}, {0.95, 10}, {0.1, 1}, {1, 10}, {0.01, 1},
	}
	for _, tt := range tests {
		if got := Percentile(sorted, tt.p); got != tt.want {
			t.Errorf("Percentile(1..10, %v) = %v, want %v", tt.p, got, tt.want)
		}
	}
	if got := Percentile(nil, 0.5); got != 0 {
		t.Errorf("Percentile(nil) = %v, want 0", got)
	}
}

func TestSummaryHTTP(t *testing.T) {
	s := NewStats()
	reset := time.Unix(1_900_000_000, 0)
	for i := range 20 {
		s.HTTP(HTTP{
			API: REST, Method: "GET", Route: "/repos/{owner}/{repo}/issues",
			Status: 200, NotModified: i%4 == 0, Duration: time.Duration(i+1) * time.Millisecond, Bytes: 100,
			Rate: Rate{Resource: "core", Limit: 5000, Remaining: 4999 - i, Used: i + 1, Reset: reset},
		})
	}
	s.HTTP(HTTP{API: GraphQL, Method: "POST", Route: "GetPull", Status: 200, Duration: 50 * time.Millisecond, Cost: 1,
		Rate: Rate{Resource: "graphql", Limit: 5000, Remaining: 4990, Used: 10, Reset: reset}})
	s.HTTP(HTTP{API: REST, Method: "GET", Route: "/notifications"})

	sum := s.Summary()
	if len(sum.Quotas) != 3 {
		t.Fatalf("quotas = %+v, want graphql/graphql, rest/core and rest/none", sum.Quotas)
	}
	g, core, none := sum.Quotas[0], sum.Quotas[1], sum.Quotas[2]
	if g.API != GraphQL || g.Cost != 1 || g.Remaining != 4990 {
		t.Errorf("graphql quota = %+v", g)
	}
	if core.Resource != "core" || core.Requests != 20 || core.NotModified != 5 || core.NotModifiedShare != 0.25 ||
		core.Remaining != 4980 || core.Used != 20 || !core.Reset.Equal(reset) || core.Bytes != 2000 {
		t.Errorf("core quota = %+v", core)
	}
	if none.Resource != "none" || none.Failed != 1 || none.Reset != nil {
		t.Errorf("quota without a response = %+v", none)
	}

	var issues RouteSummary
	for _, r := range sum.Routes {
		if r.Route == "/repos/{owner}/{repo}/issues" {
			issues = r
		}
	}
	if issues.Requests != 20 || issues.P50MS != 10 || issues.P95MS != 19 || issues.MaxMS != 20 {
		t.Errorf("issues route = %+v, want p50 10, p95 19, max 20", issues)
	}
}

func TestSummaryReservoir(t *testing.T) {
	s := NewStats()
	for i := range 3 * maxSamples {
		s.HTTP(HTTP{API: REST, Route: "/x", Status: 200, Duration: time.Duration(i%100+1) * time.Millisecond})
	}
	r := s.Summary().Routes[0]
	if r.Requests != 3*maxSamples || r.MaxMS != 100 {
		t.Errorf("route = %+v", r)
	}
	// Uniform over 1..100 ms, so the median is near 50.
	if r.P50MS < 40 || r.P50MS > 60 {
		t.Errorf("p50 = %v, want about 50", r.P50MS)
	}
}

func TestSummaryCounters(t *testing.T) {
	s := NewStats()
	for range 3 {
		s.Cache("pulls", MemoryHit)
	}
	s.Cache("pulls", MemoryMiss)
	s.Cache("pulls", Evicted)
	s.Cache("blob", DiskHit)
	s.Cache("blob", DiskMiss)
	s.Cache("blob", DiskMiss)
	s.Cache("blob", DiskMiss)
	for range 4 {
		s.Prefetch("pull", PrefetchSent)
		s.Prefetch("pull", PrefetchRead)
	}
	s.Prefetch("pull", PrefetchOpened)
	s.Prefetch("pull", PrefetchCached)
	s.Revalidate(10, 40)
	s.Revalidate(0, 40)

	sum := s.Summary()
	if len(sum.Cache) != 1 || sum.Cache[0] != (CacheSummary{Kind: "pulls", Hit: 3, Miss: 1, HitRatio: 0.75, Evicted: 1}) {
		t.Errorf("cache = %+v", sum.Cache)
	}
	if len(sum.Disk) != 1 || sum.Disk[0] != (DiskSummary{Kind: "blob", Hit: 1, Miss: 3, HitRatio: 0.25}) {
		t.Errorf("disk = %+v", sum.Disk)
	}
	want := PrefetchStats{Kind: "pull", Sent: 4, Cached: 1, Read: 4, Opened: 1, Useful: 0.25}
	if len(sum.Prefetch) != 1 || sum.Prefetch[0] != want {
		t.Errorf("prefetch = %+v, want %+v", sum.Prefetch, want)
	}
	if sum.Revalidate != (RevalSummary{Passes: 2, Sent: 10, Budget: 80, BudgetUsed: 0.125}) {
		t.Errorf("revalidate = %+v", sum.Revalidate)
	}
}

func TestSummaryConcurrent(t *testing.T) {
	s := NewStats()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 500 {
				s.Cache("pulls", MemoryHit)
				s.HTTP(HTTP{API: REST, Route: "/x", Status: 200})
				s.Summary()
			}
		})
	}
	wg.Wait()
	sum := s.Summary()
	if sum.Cache[0].Hit != 4000 || sum.Routes[0].Requests != 4000 {
		t.Errorf("counted %d hits and %d requests, want 4000 each", sum.Cache[0].Hit, sum.Routes[0].Requests)
	}
}

func TestLogSummary(t *testing.T) {
	buf := capture(t, slog.LevelInfo)
	s := NewStats()
	s.HTTP(HTTP{API: REST, Method: "GET", Route: "/x", Status: 304, NotModified: true, Rate: Rate{Resource: "core", Limit: 5000, Remaining: 4000}})
	s.Log(context.Background())
	recs := records(t, buf)
	if len(recs) != 1 || recs[0]["msg"] != "summary" {
		t.Fatalf("records = %v", recs)
	}
	quotas, _ := recs[0]["quotas"].([]any)
	if len(quotas) != 1 || quotas[0].(map[string]any)["remaining"] != 4000.0 {
		t.Errorf("quotas = %v", recs[0]["quotas"])
	}
	if !strings.Contains(buf.String(), `"not_modified_share":1`) {
		t.Errorf("summary has no 304 share: %s", buf)
	}
}

func TestPrefetched(t *testing.T) {
	s := NewStats()
	prev := SetDefault(s)
	t.Cleanup(func() { SetDefault(prev) })

	p := NewPrefetched[int]("pull")
	p.Read(1)
	p.Read(2)
	p.Opened(1)
	p.Opened(1) // counts once
	p.Opened(3) // never read ahead
	var none *Prefetched[int]
	none.Read(1)
	none.Opened(1)

	got := s.Summary().Prefetch
	if len(got) != 1 || got[0].Read != 2 || got[0].Opened != 1 || got[0].Useful != 0.5 {
		t.Errorf("prefetch = %+v, want 2 read and 1 opened", got)
	}
}

func TestPrefetchedInFlight(t *testing.T) {
	tests := []struct {
		name       string
		steps      func(p *Prefetched[int])
		wantOpened int64
	}{
		{"opened while read", func(p *Prefetched[int]) { p.Started(1); p.Opened(1); p.Read(1) }, 1},
		{"opened twice while read", func(p *Prefetched[int]) { p.Started(1); p.Opened(1); p.Opened(1); p.Read(1); p.Opened(1) }, 1},
		{"opened while read, which failed", func(p *Prefetched[int]) { p.Started(1); p.Opened(1); p.Dropped(1); p.Opened(1) }, 0},
		{"opened after a failed read", func(p *Prefetched[int]) { p.Started(1); p.Dropped(1); p.Opened(1) }, 0},
		{"opened once read", func(p *Prefetched[int]) { p.Started(1); p.Read(1); p.Opened(1) }, 1},
		{"read again after a use", func(p *Prefetched[int]) {
			p.Started(1)
			p.Read(1)
			p.Opened(1)
			p.Started(1)
			p.Read(1)
			p.Opened(1)
		}, 2},
		{"never read", func(p *Prefetched[int]) { p.Opened(1) }, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStats()
			prev := SetDefault(s)
			t.Cleanup(func() { SetDefault(prev) })
			tt.steps(NewPrefetched[int]("pull"))
			var opened int64
			for _, p := range s.Summary().Prefetch {
				opened += p.Opened
			}
			if opened != tt.wantOpened {
				t.Errorf("opened = %d, want %d", opened, tt.wantOpened)
			}
		})
	}
}

func BenchmarkCountCache(b *testing.B) {
	s := NewStats()
	b.ReportAllocs()
	for b.Loop() {
		s.Cache("pulls", MemoryHit)
	}
}

func BenchmarkCountHTTP(b *testing.B) {
	s := NewStats()
	h := HTTP{API: REST, Method: "GET", Route: "/repos/{owner}/{repo}/issues", Status: 200, Duration: time.Millisecond}
	b.ReportAllocs()
	for b.Loop() {
		s.HTTP(h)
	}
}
