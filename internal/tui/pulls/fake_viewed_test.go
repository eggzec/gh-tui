package pulls

import (
	"context"
	"maps"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

// viewedFake is the part of fakeService that keeps the viewed marks of files,
// as GitHub does, and serves them cached once they were read, as the service
// does. It records what was asked to be marked as "path=true" or "path=false".
type viewedFake struct {
	viewedMu sync.Mutex
	// held are the marks GitHub keeps, and viewedOff says that the server keeps
	// none.
	held      map[string]core.ViewedState
	viewedOff bool
	// viewedReadErr fails the reads, and viewedSendErr the sends.
	viewedReadErr error
	viewedSendErr error
	cache         *core.PullViewed
	viewedSets    []string
	viewedReads   int
}

// holding makes GitHub keep the marks of paths as state.
func (f *viewedFake) holding(state core.ViewedState, paths ...string) {
	f.viewedMu.Lock()
	defer f.viewedMu.Unlock()
	if f.held == nil {
		f.held = map[string]core.ViewedState{}
	}
	for _, p := range paths {
		f.held[p] = state
	}
}

func (f *viewedFake) Viewed(context.Context, pulls.ViewedQuery) (core.PullViewed, error) {
	f.viewedMu.Lock()
	defer f.viewedMu.Unlock()
	f.viewedReads++
	if f.viewedReadErr != nil {
		return core.PullViewed{}, f.viewedReadErr
	}
	pv := core.PullViewed{States: maps.Clone(f.held), Supported: !f.viewedOff}
	f.cache = &pv
	return pv, nil
}

func (f *viewedFake) CachedViewed(pulls.ViewedQuery) (core.PullViewed, bool) {
	f.viewedMu.Lock()
	defer f.viewedMu.Unlock()
	if f.cache == nil {
		return core.PullViewed{}, false
	}
	return *f.cache, true
}

// SetViewed shows the mark in the cached marks at once, and sends it when the
// Op runs: a refusal brings the old marks back.
func (f *viewedFake) SetViewed(_ pulls.ViewedQuery, path string, viewed bool) *optimistic.Op {
	f.viewedMu.Lock()
	defer f.viewedMu.Unlock()
	f.viewedSets = append(f.viewedSets, path+"="+map[bool]string{true: "true", false: "false"}[viewed])
	var before *core.PullViewed
	if f.cache != nil {
		old := *f.cache
		before = &old
		states := maps.Clone(old.States)
		if states == nil {
			states = map[string]core.ViewedState{}
		}
		if viewed {
			states[path] = core.Viewed
		} else {
			delete(states, path)
		}
		now := old
		now.States = states
		f.cache = &now
	}
	return optimistic.New(func(context.Context) error {
		f.viewedMu.Lock()
		defer f.viewedMu.Unlock()
		if f.viewedSendErr != nil {
			return f.viewedSendErr
		}
		if f.held == nil {
			f.held = map[string]core.ViewedState{}
		}
		if viewed {
			f.held[path] = core.Viewed
		} else {
			delete(f.held, path)
		}
		return nil
	}, func() {
		f.viewedMu.Lock()
		defer f.viewedMu.Unlock()
		f.cache = before
	})
}

// sets returns what was asked to be marked.
func (f *viewedFake) sets() []string {
	f.viewedMu.Lock()
	defer f.viewedMu.Unlock()
	return append([]string(nil), f.viewedSets...)
}
