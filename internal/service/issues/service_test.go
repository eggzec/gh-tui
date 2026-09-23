package issues

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var _ API = (*github.Client)(nil)

var repo = core.RepoRef{Owner: "octo-org", Name: "hello"}

// fakeAPI answers with its func fields and records the calls. A nil field
// fails the test when it is called.
type fakeAPI struct {
	t *testing.T

	listIssues   func(state core.StateFilter, cursor string, perPage int, cond github.Conditional) (core.Page[core.Issue], github.Response, error)
	getIssue     func(number int, cond github.Conditional) (core.Issue, github.Response, error)
	listComments func(number int, cursor string, perPage int, cond github.Conditional) (core.Page[core.Comment], github.Response, error)
	setState     func(number int, state core.State) (core.Issue, error)
	addLabels    func(number int, names []string) ([]core.Label, error)
	removeLabel  func(number int, name string) ([]core.Label, error)
	comment      func(number int, body string) (core.Comment, error)

	mu    sync.Mutex
	calls []string
}

// record logs a call to name and reports whether the test set up an answer.
func (f *fakeAPI) record(name string, set bool) bool {
	f.t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, name)
	if !set {
		f.t.Errorf("unexpected call to %s", name)
	}
	return set
}

// called returns the names of the calls so far and forgets them.
func (f *fakeAPI) called() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	calls := f.calls
	f.calls = nil
	slices.Sort(calls)
	return calls
}

func (f *fakeAPI) checkCalls(t *testing.T, want ...string) {
	t.Helper()
	slices.Sort(want)
	if got := f.called(); !slices.Equal(got, want) {
		t.Errorf("calls = %q, want %q", got, want)
	}
}

func (f *fakeAPI) ListIssues(_ context.Context, r core.RepoRef, state core.StateFilter, cursor string, perPage int, cond github.Conditional) (core.Page[core.Issue], github.Response, error) {
	f.checkRepo(r)
	if !f.record("ListIssues", f.listIssues != nil) {
		return core.Page[core.Issue]{}, github.Response{}, errUnexpected
	}
	return f.listIssues(state, cursor, perPage, cond)
}

func (f *fakeAPI) GetIssue(_ context.Context, r core.RepoRef, number int, cond github.Conditional) (core.Issue, github.Response, error) {
	f.checkRepo(r)
	if !f.record("GetIssue", f.getIssue != nil) {
		return core.Issue{}, github.Response{}, errUnexpected
	}
	return f.getIssue(number, cond)
}

func (f *fakeAPI) ListIssueComments(_ context.Context, r core.RepoRef, number int, cursor string, perPage int, cond github.Conditional) (core.Page[core.Comment], github.Response, error) {
	f.checkRepo(r)
	if !f.record("ListIssueComments", f.listComments != nil) {
		return core.Page[core.Comment]{}, github.Response{}, errUnexpected
	}
	return f.listComments(number, cursor, perPage, cond)
}

func (f *fakeAPI) SetIssueState(_ context.Context, r core.RepoRef, number int, state core.State) (core.Issue, error) {
	f.checkRepo(r)
	if !f.record("SetIssueState", f.setState != nil) {
		return core.Issue{}, errUnexpected
	}
	return f.setState(number, state)
}

func (f *fakeAPI) AddIssueLabels(_ context.Context, r core.RepoRef, number int, names []string) ([]core.Label, error) {
	f.checkRepo(r)
	if !f.record("AddIssueLabels", f.addLabels != nil) {
		return nil, errUnexpected
	}
	return f.addLabels(number, names)
}

func (f *fakeAPI) RemoveIssueLabel(_ context.Context, r core.RepoRef, number int, name string) ([]core.Label, error) {
	f.checkRepo(r)
	if !f.record("RemoveIssueLabel", f.removeLabel != nil) {
		return nil, errUnexpected
	}
	return f.removeLabel(number, name)
}

func (f *fakeAPI) CreateIssueComment(_ context.Context, r core.RepoRef, number int, body string) (core.Comment, error) {
	f.checkRepo(r)
	if !f.record("CreateIssueComment", f.comment != nil) {
		return core.Comment{}, errUnexpected
	}
	return f.comment(number, body)
}

func (f *fakeAPI) checkRepo(r core.RepoRef) {
	f.t.Helper()
	if r != repo {
		f.t.Errorf("repo = %v, want %v", r, repo)
	}
}

type fakeError string

func (e fakeError) Error() string { return string(e) }

const errUnexpected = fakeError("unexpected call")

// Fixtures. Each test gets fresh values, so no test can change another's.

var epoch = time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

func issue(number int) core.Issue {
	return core.Issue{
		ID:        fmt.Sprintf("I_%d", number),
		Repo:      repo,
		Number:    number,
		Title:     "Issue",
		State:     core.StateOpen,
		Author:    core.User{Login: "octocat"},
		Labels:    []core.Label{{Name: "bug", Color: "d73a4a"}},
		Comments:  1,
		CreatedAt: epoch,
		UpdatedAt: epoch,
	}
}

func page(next string, numbers ...int) core.Page[core.Issue] {
	p := core.Page[core.Issue]{Next: next}
	for _, n := range numbers {
		p.Items = append(p.Items, issue(n))
	}
	return p
}

func comment(n int) core.Comment {
	return core.Comment{
		ID:        fmt.Sprintf("IC_%d", n),
		Author:    core.User{Login: "hubot"},
		Body:      fmt.Sprintf("Comment %d", n),
		CreatedAt: epoch,
		UpdatedAt: epoch,
	}
}

// thread returns comments 1 to n, oldest first.
func thread(n int) []core.Comment {
	out := make([]core.Comment, n)
	for i := range n {
		out[i] = comment(i + 1)
	}
	return out
}

// commentPage pages through all as GitHub does, with cursors of the form
// "offset=N". It may run outside the test's goroutine, so it doesn't stop
// the test.
func commentPage(t *testing.T, all []core.Comment, cursor string, perPage int) core.Page[core.Comment] {
	t.Helper()
	off := 0
	if cursor != "" {
		if _, err := fmt.Sscanf(cursor, "offset=%d", &off); err != nil {
			t.Errorf("cursor %q: %v", cursor, err)
			return core.Page[core.Comment]{}
		}
	}
	end := min(off+perPage, len(all))
	p := core.Page[core.Comment]{Items: slices.Clone(all[off:end])}
	if end < len(all) {
		p.Next = fmt.Sprintf("offset=%d", end)
	}
	return p
}

// ids returns the comment IDs of p.
func ids(p core.Page[core.Comment]) []string {
	out := make([]string, len(p.Items))
	for i := range p.Items {
		out[i] = p.Items[i].ID
	}
	return out
}

func ok(etag string) github.Response {
	return github.Response{StatusCode: 200, ETag: etag}
}

var notModified = github.Response{StatusCode: 304, NotModified: true}

// numbers returns the issue numbers of p.
func numbers(p core.Page[core.Issue]) []int {
	out := make([]int, len(p.Items))
	for i := range p.Items {
		out[i] = p.Items[i].Number
	}
	return out
}
