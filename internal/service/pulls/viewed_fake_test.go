package pulls

import (
	"context"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
)

// viewedFake is the part of fakeAPI that answers the viewed marks. It counts
// its reads, and records what was sent as "path=true" or "path=false".
type viewedFake struct {
	viewedMu    sync.Mutex
	viewedRead  core.PullViewed
	viewedReads int
	viewedSent  []string
	// viewedErr fails SetFileViewed.
	viewedErr error
}

func (f *viewedFake) ListPullViewed(context.Context, core.RepoRef, int) (core.PullViewed, error) {
	f.viewedMu.Lock()
	defer f.viewedMu.Unlock()
	f.viewedReads++
	return f.viewedRead, nil
}

func (f *viewedFake) SetFileViewed(_ context.Context, _, path string, viewed bool) error {
	f.viewedMu.Lock()
	defer f.viewedMu.Unlock()
	if f.viewedErr != nil {
		return f.viewedErr
	}
	f.viewedSent = append(f.viewedSent, path+"="+map[bool]string{true: "true", false: "false"}[viewed])
	return nil
}

func (f *viewedFake) reads() int {
	f.viewedMu.Lock()
	defer f.viewedMu.Unlock()
	return f.viewedReads
}

func (f *viewedFake) viewedMarks() []string {
	f.viewedMu.Lock()
	defer f.viewedMu.Unlock()
	return append([]string(nil), f.viewedSent...)
}
