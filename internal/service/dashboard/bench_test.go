package dashboard

import (
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/cache/disk"
	"github.com/eggzec/gh-tui/internal/core"
)

// BenchmarkCachedAllRepos measures gathering the 1000 cached repositories
// of an owner, as filtering them by name may on every key.
func BenchmarkCachedAllRepos(b *testing.B) {
	s := New(&fakeAPI{t: b, orgRepos: pagedOrg(MaxOwnerRepos)})
	q := ReposQuery{Owner: "big"}
	if _, err := s.AllRepos(b.Context(), q, 0); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if p, ok := s.CachedAllRepos(q, 0); !ok || len(p.Items) != MaxOwnerRepos {
			b.Fatalf("CachedAllRepos = %d repos, %v", len(p.Items), ok)
		}
	}
}

// BenchmarkContributionsFromStore measures reading a year's calendar that
// an earlier session kept, as a warm start does.
func BenchmarkContributionsFromStore(b *testing.B) {
	store, err := disk.Open(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	s := New(&fakeAPI{t: b}, WithStore(store))
	if err := s.contributions.kept.Save(contributionsKey, cache.Entry[core.Contributions]{Value: year()}); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if e, ok := s.contributions.kept.Load(contributionsKey); !ok || len(e.Value.Weeks) != 53 {
			b.Fatal("missed the kept calendar")
		}
	}
}

// year is a calendar of 53 weeks with a contribution or a few most days.
func year() core.Contributions {
	c := core.Contributions{Weeks: make([][]core.ContributionDay, 53)}
	day := time.Date(2025, 9, 21, 0, 0, 0, 0, time.UTC)
	for w := range c.Weeks {
		c.Weeks[w] = make([]core.ContributionDay, 7)
		for d := range 7 {
			n := (w*7 + d) % 9
			c.Weeks[w][d] = core.ContributionDay{Date: day, Count: n, Level: min(n, 4)}
			c.Total += n
			day = day.AddDate(0, 0, 1)
		}
	}
	return c
}
