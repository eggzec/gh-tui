package repos

import (
	"context"
	"slices"
	"strconv"
	"sync"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	reposvc "github.com/eggzec/gh-tui/internal/service/repos"
)

var _ Service = (*reposvc.Service)(nil)

// fakeService serves repos in pages of pageSize, with cursors that are the
// index of the page's first repository, and stars them the way the real
// service does: at once, with a rollback if sending fails.
type fakeService struct {
	mu       sync.Mutex
	repos    []core.Repo
	pageSize int
	listErr  error
	sendErr  error

	lists []string
	sent  []string
}

func newFake(repos ...core.Repo) *fakeService {
	return &fakeService{repos: slices.Clone(repos), pageSize: 30}
}

func (f *fakeService) List(_ context.Context, q reposvc.ListQuery) (core.Page[core.Repo], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists = append(f.lists, q.Cursor)
	if f.listErr != nil {
		return core.Page[core.Repo]{}, f.listErr
	}
	start, _ := strconv.Atoi(q.Cursor)
	end := min(start+f.pageSize, len(f.repos))
	p := core.Page[core.Repo]{Items: slices.Clone(f.repos[start:end])}
	if end < len(f.repos) {
		p.Next = strconv.Itoa(end)
	}
	return p, nil
}

func (f *fakeService) Star(ref core.RepoRef) *optimistic.Op {
	return f.setStarred(ref, true)
}

func (f *fakeService) Unstar(ref core.RepoRef) *optimistic.Op {
	return f.setStarred(ref, false)
}

func (f *fakeService) setStarred(ref core.RepoRef, starred bool) *optimistic.Op {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.repos, func(r core.Repo) bool { return r.Ref == ref })
	if i < 0 {
		return optimistic.New(func(context.Context) error { return nil })
	}
	prev := f.repos[i]
	f.repos[i].Starred = starred
	verb := "star "
	if !starred {
		verb = "unstar "
	}
	send := func(context.Context) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.sent = append(f.sent, verb+ref.String())
		return f.sendErr
	}
	rollback := func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.repos[i] = prev
	}
	return optimistic.New(send, rollback)
}

func (f *fakeService) starred(ref core.RepoRef) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.repos, func(r core.Repo) bool { return r.Ref == ref })
	return i >= 0 && f.repos[i].Starred
}

func (f *fakeService) listCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.lists)
}

func (f *fakeService) sentOps() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.sent)
}
