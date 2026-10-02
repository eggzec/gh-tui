package dashboard

import (
	"context"
	"slices"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// landingFake records the repositories read ahead, and counts one read as
// cached.
type landingFake struct {
	mu   sync.Mutex
	read []string
	ctxs []context.Context
	// hold, if set, holds every read until it is closed or the read is
	// canceled.
	hold chan struct{}
}

func (f *landingFake) Read(ctx context.Context, repo core.RepoRef) error {
	f.mu.Lock()
	f.ctxs = append(f.ctxs, ctx)
	hold := f.hold
	f.mu.Unlock()
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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

// holdReads holds the reads from now on, and forgets those started, so
// that the reads the dashboard sends as it starts are done.
func (f *landingFake) holdReads() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.hold, f.ctxs = make(chan struct{}), nil
}

// names returns the repositories read, sorted.
func (f *landingFake) names() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Sorted(slices.Values(f.read))
}

// inFlight returns the contexts of the reads started.
func (f *landingFake) inFlight() []context.Context {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.ctxs)
}

// landingOn is the default config with the reads ahead of the
// repositories and pinned panes on, which are off by default.
func landingOn(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Default()
	for _, k := range []string{"prefetch.dashboard.repositories.enabled", "prefetch.dashboard.pinned.enabled"} {
		var err error
		if cfg, err = cfg.Set(k, "true"); err != nil {
			t.Fatal(err)
		}
	}
	return cfg
}

// landingSection returns a dashboard that reads the repositories ahead
// into f, as cfg's prefetch settings say.
func landingSection(t *testing.T, f *landingFake, cfg config.Config) *Section {
	t.Helper()
	return newSection(t, newFake(), nil, 140, 38, WithLanding(f), WithPrefetch(cfg.Prefetch))
}

// The repositories around the cursor are read as it rests, one above and
// one below, and the one under it is left to opening it.
func TestReposReadAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &landingFake{}
		s := landingSection(t, f, landingOn(t))
		// The list loading counts as a rest.
		if got, want := f.names(), []string{"octocat/repo-001"}; !slices.Equal(got, want) {
			t.Fatalf("read %v ahead once listed, want %v", got, want)
		}
		press(t, s, "down")
		if got, want := f.names(), []string{"octocat/repo-000", "octocat/repo-001", "octocat/repo-002"}; !slices.Equal(got, want) {
			t.Errorf("read %v ahead on the second row, want %v", got, want)
		}
		press(t, s, "down", "down")
		if got := f.names(); !slices.Contains(got, "octocat/repo-003") || !slices.Contains(got, "octocat/repo-004") {
			t.Errorf("read %v ahead on the fourth row, want the third and fifth too", got)
		}
		if got := f.names(); len(got) != 5 {
			t.Errorf("read %v ahead, want each repository once", got)
		}
	})
}

// The pinned cards around the cursor are read while the pane has the
// focus, in the order the pane lists them, and not before.
func TestPinnedReadAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &landingFake{}
		s := landingSection(t, f, landingOn(t))
		if got := f.names(); slices.Contains(got, "octocat/spoon-knife") {
			t.Fatalf("read %v ahead before the pinned pane had the focus", got)
		}
		before := len(f.names())
		press(t, s, "1")
		if got := f.names(); len(got) != before+1 || !slices.Contains(got, "octocat/spoon-knife") {
			t.Fatalf("read %v ahead on the first card, want the second card only", got)
		}
		press(t, s, "right")
		got := f.names()
		for _, want := range []string{"octocat/hello-world", "charmbracelet/bubbletea"} {
			if !slices.Contains(got, want) {
				t.Errorf("read %v ahead on the second card, want %s", got, want)
			}
		}
	})
}

// Turning a kind off reads nothing ahead for its pane, while the other
// pane reads on.
func TestLandingOff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		cfg, err := landingOn(t).Set("prefetch.dashboard.repositories.enabled", "false")
		if err != nil {
			t.Fatal(err)
		}
		f := &landingFake{}
		s := landingSection(t, f, cfg)
		press(t, s, "down", "down")
		if got := f.names(); len(got) != 0 {
			t.Fatalf("read %v ahead with the repositories off", got)
		}
		press(t, s, "1")
		if got := f.names(); !slices.Equal(got, []string{"octocat/spoon-knife"}) {
			t.Errorf("read %v ahead, want the pinned pane to read on", got)
		}
	})
}

// Another tab of repositories, or a filter, stops the reads of the list
// that was shown.
func TestReposStopReadingAheadOnNewList(t *testing.T) {
	for _, tt := range []struct {
		name   string
		change func(s *Section)
	}{
		{"tab", func(s *Section) { s.Update(keyPress("]")) }},
		{"filter", func(s *Section) { s.repos.setFilter("repo-1") }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := &landingFake{}
				s := landingSection(t, f, landingOn(t))
				f.holdReads()
				reads := s.Update(keyPress("down"))
				done := make(chan struct{})
				go func() {
					run(t, s, reads)
					close(done)
				}()
				// The cursor rests.
				time.Sleep(time.Second)
				synctest.Wait()
				ctxs := f.inFlight()
				if len(ctxs) == 0 {
					t.Fatal("no reads in flight")
				}
				tt.change(s)
				<-done
				for i, ctx := range ctxs {
					if ctx.Err() == nil {
						t.Errorf("read %d goes on for the list that left", i)
					}
				}
				close(f.hold)
			})
		})
	}
}

// Pinned repositories that change stop the reads of the cards that were
// listed; the same ones read again leave them be.
func TestPinnedStopReadingAheadOnNewList(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &landingFake{}
		s := landingSection(t, f, landingOn(t))
		f.holdReads()
		reads := s.Update(keyPress("1"))
		done := make(chan struct{})
		go func() {
			run(t, s, reads)
			close(done)
		}()
		synctest.Wait()
		ctxs := f.inFlight()
		if len(ctxs) != 1 {
			t.Fatalf("%d reads in flight, want the second card's", len(ctxs))
		}
		s.setPinned(header().Pinned)
		if ctxs[0].Err() != nil {
			t.Fatal("the same pinned repositories stopped the read")
		}
		s.setPinned(header().Pinned[2:])
		<-done
		if ctxs[0].Err() == nil {
			t.Error("the read goes on for pinned repositories that changed")
		}
		close(f.hold)
	})
}

// Opening a repository read ahead counts as its use.
func TestLandingOpenCounts(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stats := obs.NewStats()
		prev := obs.SetDefault(stats)
		t.Cleanup(func() { obs.SetDefault(prev) })
		f := &landingFake{}
		s := landingSection(t, f, landingOn(t))
		press(t, s, "down")
		app := press(t, s, "enter")
		if len(app) != 1 || app[0] != (ui.RepoMsg{Repo: core.RepoRef{Owner: "octocat", Name: "repo-001"}}) {
			t.Fatalf("enter sent %v, want the second repository opened", app)
		}
		var opened int64
		for _, p := range stats.Summary().Prefetch {
			if p.Kind == "repo" {
				opened = p.Opened
			}
		}
		if opened != 1 {
			t.Errorf("summary = %+v, want the repository opened", stats.Summary().Prefetch)
		}
	})
}

// Leaving the dashboard stops the reads ahead of the repositories.
func TestLandingStopsOnBlur(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &landingFake{}
		s := landingSection(t, f, landingOn(t))
		f.holdReads()
		reads := s.Update(keyPress("down"))
		done := make(chan struct{})
		go func() {
			run(t, s, reads)
			close(done)
		}()
		time.Sleep(time.Second)
		synctest.Wait()
		ctxs := f.inFlight()
		if len(ctxs) == 0 {
			t.Fatal("no reads in flight")
		}
		s.Blur()
		<-done
		for i, ctx := range ctxs {
			if ctx.Err() == nil {
				t.Errorf("read %d goes on off screen", i)
			}
		}
		close(f.hold)
	})
}

// By default neither pane reads ahead, since a repository costs a few
// requests and its listing may be large.
func TestLandingOffByDefault(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := &landingFake{}
		s := landingSection(t, f, config.Default())
		press(t, s, "down", "down", "1", "right")
		if got := f.names(); len(got) != 0 {
			t.Errorf("read %v ahead by default, want nothing", got)
		}
	})
}
