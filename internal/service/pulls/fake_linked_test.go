package pulls

import (
	"context"
	"strconv"
	"strings"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
)

// fakeLinked answers the reads of linked items and counts them. It is
// embedded in fakeAPI, and a test reads its counts through api.fakeLinked.
type fakeLinked struct {
	mu    sync.Mutex
	calls map[string]int
	// err, when set, fails every read.
	err error
}

// count returns how many times the read named method, with key, was made.
func (f *fakeLinked) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for k, v := range f.calls {
		if strings.HasPrefix(k, method+" ") {
			n += v
		}
	}
	return n
}

// called counts a read and returns the error to fail it with.
func (f *fakeLinked) called(method string, repo core.RepoRef, number int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[method+" "+repo.String()+"#"+strconv.Itoa(number)]++
	return f.err
}

func (f *fakeLinked) PullLinks(_ context.Context, repo core.RepoRef, number int) (core.PullLinks, error) {
	if err := f.called("PullLinks", repo, number); err != nil {
		return core.PullLinks{}, err
	}
	return core.PullLinks{ClosesTotal: number, BlockersSupported: true}, nil
}

func (f *fakeLinked) IssuePreview(_ context.Context, repo core.RepoRef, number int) (core.IssuePreview, error) {
	if err := f.called("IssuePreview", repo, number); err != nil {
		return core.IssuePreview{}, err
	}
	return core.IssuePreview{Target: core.Target{Repo: repo, Number: number}}, nil
}

func (f *fakeLinked) PullPreview(_ context.Context, repo core.RepoRef, number int) (core.PullPreview, error) {
	if err := f.called("PullPreview", repo, number); err != nil {
		return core.PullPreview{}, err
	}
	return core.PullPreview{Target: core.Target{Repo: repo, Number: number}}, nil
}
