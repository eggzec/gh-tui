package owner

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/service/owners"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// landingFake records the repositories read ahead.
type landingFake struct {
	mu   sync.Mutex
	read []string
}

func (f *landingFake) Read(_ context.Context, repo core.RepoRef) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.read = append(f.read, repo.String())
	return nil
}

func (f *landingFake) Cached(repo core.RepoRef) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.read, repo.String())
}

// names returns the repositories read, sorted.
func (f *landingFake) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(slices.Values(f.read))
}

// chargingService charges each header it reads ahead cost GraphQL points
// against the budget of the reads ahead.
type chargingService struct {
	*fakeService
	cost int
}

func (c chargingService) Header(ctx context.Context, q owners.HeaderQuery) (core.Owner, error) {
	obs.ChargeGraphQL(ctx, c.cost)
	return c.fakeService.Header(ctx, q)
}

// withStats counts in stats of their own the reads ahead of a test, and
// what they spent of their budget, which is the default 10% of each
// quota.
func withStats(t *testing.T) *obs.Stats {
	t.Helper()
	stats := obs.NewStats()
	prev := obs.SetDefault(stats)
	obs.SetPrefetchBudget(config.Default().Prefetch.Budget)
	t.Cleanup(func() {
		obs.SetDefault(prev)
		obs.SetPrefetchBudget(0)
	})
	return stats
}

// prefetchOn is the default config with the kinds of the owner page
// named turned on.
func prefetchOn(t *testing.T, kinds ...string) config.Config {
	t.Helper()
	cfg := config.Default()
	for _, k := range kinds {
		var err error
		if cfg, err = cfg.Set("prefetch.owner."+k+".enabled", "true"); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

// withFollowers gives every follower of octocat a page of their own.
func withFollowers(f *fakeService) *fakeService {
	for _, p := range f.people["octocat followers"] {
		o := user()
		o.Profile.Login = p.Login
		f.owners[p.Login] = o
	}
	return f
}

// aheadSection returns the page of login, which reads ahead into svc and
// f as cfg says.
func aheadSection(t *testing.T, svc Service, f *landingFake, cfg config.Config, login string) *Section {
	t.Helper()
	s := newSection(t, svc, login, 120, 40, WithLanding(f), WithPrefetch(cfg.Prefetch))
	// The app's next message finds the page on view.
	run(t, s, s.Update(nil))
	return s
}

// headers returns the logins whose headers f read, in order.
func headers(f *fakeService) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, c := range f.calls {
		if login, ok := strings.CutPrefix(c, "header "); ok {
			out = append(out, login)
		}
	}
	return out
}

// calls returns what f was asked for.
func calls(f *fakeService) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// The headers of the people around the cursor are read, the row under it
// too, two below it and none above it.
func TestPeopleReadAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		withStats(t)
		svc := withFollowers(newFake())
		s := aheadSection(t, svc, &landingFake{}, prefetchOn(t, "people"), "octocat")
		press(t, s, "]", "]")
		// The list loading counts as a rest. The reads run at once, in any
		// order.
		if got, want := slices.Sorted(slices.Values(headers(svc))), []string{"octocat", "person-00", "person-01", "person-02"}; !slices.Equal(got, want) {
			t.Fatalf("read the headers of %v, want %v", got, want)
		}
		press(t, s, "down", "down")
		if got := headers(svc); !slices.Contains(got, "person-03") || !slices.Contains(got, "person-04") {
			t.Errorf("read the headers of %v on the third row, want person-03 and person-04 too", got)
		}
		seen := map[string]int{}
		for _, l := range headers(svc) {
			seen[l]++
		}
		for l, n := range seen {
			if n > 1 {
				t.Errorf("read the header of %s %d times, want once", l, n)
			}
		}
	})
}

// Opening a person read ahead counts as its use, and the page opens on
// what was read.
func TestPeopleOpenCounts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := withStats(t)
		svc := withFollowers(newFake())
		s := aheadSection(t, svc, &landingFake{}, prefetchOn(t, "people"), "octocat")
		press(t, s, "]", "]", "down")
		app := press(t, s, "enter")
		if len(app) != 1 || app[0] != (ui.OwnerMsg{Login: "person-01"}) {
			t.Fatalf("enter sent %v, want person-01's page", app)
		}
		for _, p := range stats.Summary().Prefetch {
			if p.Kind == "owner" && p.Opened == 1 {
				return
			}
		}
		t.Errorf("summary = %+v, want the person opened", stats.Summary().Prefetch)
	})
}

// The repositories around the cursor are read as it rests, one above and
// one below, and the one under it is left to opening it; the pinned
// cards are read while their pane has the focus.
func TestOwnerReposReadAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		withStats(t)
		f := &landingFake{}
		s := aheadSection(t, newFake(), f, prefetchOn(t, "repositories"), "octocat")
		if got, want := f.names(), []string{"octocat/repo-001"}; !slices.Equal(got, want) {
			t.Fatalf("read %v ahead once listed, want %v", got, want)
		}
		press(t, s, "down")
		if got, want := f.names(), []string{"octocat/repo-000", "octocat/repo-001", "octocat/repo-002"}; !slices.Equal(got, want) {
			t.Errorf("read %v ahead on the second row, want %v", got, want)
		}
		press(t, s, "]")
		if got := f.names(); !slices.Contains(got, "cli/cli") {
			t.Errorf("read %v ahead on the stars, want the second star", got)
		}
		press(t, s, "1")
		if got := f.names(); !slices.Contains(got, "octocat/spoon-knife") {
			t.Errorf("read %v ahead on the first pinned card, want the second card", got)
		}
	})
}

// The first page of each tab not on view is read once the page rests,
// and only the tabs the viewer may see.
func TestOtherTabsReadAhead(t *testing.T) {
	for _, tt := range []struct {
		name, login string
		setup       func(f *fakeService)
		want, not   []string
	}{
		{"user", "octocat", nil,
			[]string{"stars octocat ", "people octocat followers ", "people octocat following ", "people octocat orgs "}, nil},
		{"member", "github", nil, []string{"people github members ", "teams github "}, nil},
		{"outsider", "github", func(f *fakeService) { f.owners["github"] = outsider() },
			[]string{"people github members "}, []string{"teams github "}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				stats := withStats(t)
				svc := newFake()
				if tt.setup != nil {
					tt.setup(svc)
				}
				s := aheadSection(t, svc, &landingFake{}, prefetchOn(t, "other_tabs"), tt.login)
				got := calls(svc)
				for _, want := range tt.want {
					if !slices.Contains(got, want) {
						t.Errorf("calls %v, want %q", got, want)
					}
				}
				for _, not := range tt.not {
					if slices.Contains(got, not) {
						t.Errorf("calls %v, want no %q", got, not)
					}
				}
				// Showing a tab read ahead counts as its use.
				press(t, s, "]")
				for _, p := range stats.Summary().Prefetch {
					if p.Kind == "owner_tab" && p.Opened == 1 {
						return
					}
				}
				t.Errorf("summary = %+v, want the next tab opened", stats.Summary().Prefetch)
			})
		})
	}
}

// By default nothing is read ahead on the page.
func TestOwnerPrefetchOffByDefault(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		withStats(t)
		svc := withFollowers(newFake())
		f := &landingFake{}
		s := aheadSection(t, svc, f, config.Default(), "octocat")
		press(t, s, "down", "down", "1", "right", "2", "]", "]", "down")
		if got := headers(svc); !slices.Equal(got, []string{"octocat"}) {
			t.Errorf("read the headers of %v by default, want octocat's only", got)
		}
		if got := f.names(); len(got) != 0 {
			t.Errorf("read %v ahead by default, want nothing", got)
		}
		for _, c := range calls(svc) {
			if strings.HasPrefix(c, "people octocat following") || strings.HasPrefix(c, "people octocat orgs") {
				t.Errorf("read %q ahead by default", c)
			}
		}
	})
}

// Turning a kind on with the set command reads ahead from the next move.
func TestOwnerPrefetchSettings(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		withStats(t)
		svc := withFollowers(newFake())
		s := aheadSection(t, svc, &landingFake{}, config.Default(), "octocat")
		press(t, s, "]", "]")
		run(t, s, s.Update(ui.SettingsMsg{Config: prefetchOn(t, "people")}))
		press(t, s, "down")
		if got := headers(svc); !slices.Contains(got, "person-01") {
			t.Errorf("read the headers of %v once turned on, want person-01", got)
		}
	})
}

// The reads ahead stop once they spent their budget, for every kind.
func TestOwnerPrefetchStopsOnBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := withStats(t)
		f := withFollowers(newFake())
		// Each header spends a fifth of the budget, 500 of 5000 points.
		svc := chargingService{fakeService: f, cost: 100}
		s := aheadSection(t, svc, &landingFake{}, prefetchOn(t, "people"), "octocat")
		press(t, s, "]", "]")
		for range 10 {
			press(t, s, "down")
		}
		// octocat's own header isn't read ahead, so it costs nothing.
		if n := len(headers(f)) - 1; n < 5 || n > 5+config.Default().Prefetch.Parallel-1 {
			t.Errorf("read %d headers ahead, want 5 and those already in flight", n)
		}
		if !stats.Summary().Budget.Spent {
			t.Error("the budget isn't spent")
		}
		var over int64
		for _, p := range stats.Summary().Prefetch {
			if p.Kind == "owner" {
				over = p.OverBudget
			}
		}
		if over == 0 {
			t.Errorf("summary = %+v, want reads skipped for the budget", stats.Summary().Prefetch)
		}
	})
}

// A read ahead that GitHub refuses with the rate limit, such as once less
// than a tenth of the quota is left, stops the reads until the limit
// lifts, and they go on from there.
func TestOwnerPrefetchResumesAfterLimit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		withStats(t)
		svc := withFollowers(newFake())
		s := aheadSection(t, svc, &landingFake{}, prefetchOn(t, "people"), "octocat")
		svc.mu.Lock()
		svc.fail["header"] = fmt.Errorf("refused: %w", core.ErrRateLimited)
		svc.mu.Unlock()
		press(t, s, "]", "]")
		limited := len(headers(svc))
		press(t, s, "down", "down", "down")
		if got := len(headers(svc)); got != limited {
			t.Fatalf("read %d more headers while rate limited", got-limited)
		}
		// Another limit still holds.
		run(t, s, s.Update(ui.OnlineMsg{Limited: true}))
		press(t, s, "down")
		if got := len(headers(svc)); got != limited {
			t.Fatalf("read %d more headers while a limit holds", got-limited)
		}
		svc.mu.Lock()
		delete(svc.fail, "header")
		svc.mu.Unlock()
		run(t, s, s.Update(ui.OnlineMsg{}))
		press(t, s, "down")
		if got := headers(svc); !slices.Contains(got, "person-05") || !slices.Contains(got, "person-06") {
			t.Errorf("read the headers of %v once the limit lifted, want those around the cursor", got)
		}
	})
}

// The viewer's own row opens the dashboard, so their header isn't read
// ahead, while the rows around it are.
func TestPeopleSkipViewer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		withStats(t)
		svc := withFollowers(newFake())
		s := newSection(t, svc, "octocat", 120, 40, WithLanding(&landingFake{}),
			WithPrefetch(prefetchOn(t, "people").Prefetch), WithViewer("Person-01"))
		press(t, s, "]", "]")
		if got, want := slices.Sorted(slices.Values(headers(svc))), []string{"octocat", "person-00", "person-02"}; !slices.Equal(got, want) {
			t.Errorf("read the headers of %v, want %v", got, want)
		}
	})
}

// Reading the page again reads again only the tabs read ahead before;
// a tab never read waits until the page shows another tab.
func TestOtherTabsRefresh(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		withStats(t)
		svc := newFake()
		svc.fail["stars"] = errors.New("github: decode: unexpected EOF")
		s := aheadSection(t, svc, &landingFake{}, prefetchOn(t, "other_tabs"), "octocat")
		svc.mu.Lock()
		delete(svc.fail, "stars")
		svc.mu.Unlock()
		n := len(calls(svc))
		press(t, s, "r")
		again := calls(svc)[n:]
		for _, want := range []string{"people octocat followers ", "people octocat following ", "people octocat orgs "} {
			if !slices.Contains(again, want) {
				t.Errorf("read %v again, want %q", again, want)
			}
		}
		if slices.Contains(again, "stars octocat ") {
			t.Errorf("read %v again, want no stars, which were never read", again)
		}
		// The organizations tab is another list, whose window reads the
		// stars.
		n = len(calls(svc))
		press(t, s, "[")
		if more := calls(svc)[n:]; !slices.Contains(more, "stars octocat ") {
			t.Errorf("read %v on another tab, want the stars", more)
		}
	})
}

// holdingService holds every header read ahead until hold is closed or
// the read is canceled, and keeps the context of each.
type holdingService struct {
	*fakeService
	hold chan struct{}
	mu   *sync.Mutex
	ctxs *[]context.Context
}

func newHolding(f *fakeService) holdingService {
	return holdingService{fakeService: f, hold: make(chan struct{}), mu: new(sync.Mutex), ctxs: new([]context.Context)}
}

func (h holdingService) Header(ctx context.Context, q owners.HeaderQuery) (core.Owner, error) {
	if obs.IsPrefetch(ctx) {
		h.mu.Lock()
		*h.ctxs = append(*h.ctxs, ctx)
		h.mu.Unlock()
		select {
		case <-h.hold:
		case <-ctx.Done():
			return core.Owner{}, ctx.Err()
		}
	}
	return h.fakeService.Header(ctx, q)
}

func (h holdingService) inFlight() []context.Context {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(*h.ctxs)
}

// Leaving the page, or opening another account, stops the reads ahead
// in flight of the page that was on view.
func TestOwnerPrefetchStops(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(t *testing.T, s *Section)
	}{
		{"blur", func(_ *testing.T, s *Section) { s.Blur() }},
		{"another account", func(_ *testing.T, s *Section) { s.Update(ui.OwnerMsg{Login: "github"}) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				withStats(t)
				svc := newHolding(withFollowers(newFake()))
				s := aheadSection(t, svc, &landingFake{}, prefetchOn(t, "people"), "octocat")
				// The Followers tab shows; its first window is read once
				// the cursor rests on it.
				reads := s.Update(keyPress("]"))
				reads = tea.Batch(reads, s.Update(keyPress("]")))
				done := make(chan struct{})
				go func() {
					run(t, s, reads)
					close(done)
				}()
				time.Sleep(time.Second)
				synctest.Wait()
				ctxs := svc.inFlight()
				if len(ctxs) == 0 {
					t.Fatal("no reads in flight")
				}
				tt.change(t, s)
				<-done
				for i, ctx := range ctxs {
					if ctx.Err() == nil {
						t.Errorf("read %d goes on", i)
					}
				}
				close(svc.hold)
			})
		})
	}
}
