package details

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/service/pulls"
)

var cli = core.RepoRef{Owner: "cli", Name: "cli"}

func TestOf(t *testing.T) {
	tests := []struct {
		hit  core.SearchHit
		want Key
		ok   bool
	}{
		{core.SearchHit{Kind: core.SearchPulls, Issue: core.Issue{Repo: cli, Number: 7}}, Key{Pull: true, Repo: cli, Number: 7}, true},
		{core.SearchHit{Kind: core.SearchIssues, Issue: core.Issue{Repo: cli, Number: 8}}, Key{Repo: cli, Number: 8}, true},
		{core.SearchHit{Kind: core.SearchRepos, Repo: core.Repo{Ref: cli}}, Key{}, false},
	}
	for _, tt := range tests {
		if got, ok := Of(tt.hit); got != tt.want || ok != tt.ok {
			t.Errorf("Of(%s) = %v, %v, want %v, %v", tt.hit.Kind, got, ok, tt.want, tt.ok)
		}
	}
}

// calls records what the services were asked for.
type calls struct {
	mu   sync.Mutex
	what []string
	err  error
}

func (c *calls) add(what string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.what = append(c.what, what)
	return c.err
}

type pullsFake struct{ *calls }

func (f pullsFake) Get(context.Context, core.RepoRef, int) (core.PullRequestDetail, error) {
	return core.PullRequestDetail{}, f.add("pull")
}

func (f pullsFake) Comments(_ context.Context, q pulls.CommentsQuery) (core.Page[core.Comment], error) {
	if q != (pulls.CommentsQuery{Repo: cli, Number: 7}) {
		return core.Page[core.Comment]{}, errors.New("not the query the modal reads")
	}
	return core.Page[core.Comment]{}, f.add("pull comments")
}

func (f pullsFake) CurrentGet(core.RepoRef, int) bool { return false }

// CurrentComments says the comments are cached, so that a test can tell
// the two kinds apart.
func (f pullsFake) CurrentComments(pulls.CommentsQuery) bool { return true }

type issuesFake struct{ *calls }

func (f issuesFake) Get(context.Context, core.RepoRef, int) (core.Issue, error) {
	return core.Issue{}, f.add("issue")
}

func (f issuesFake) Comments(_ context.Context, q issuesvc.CommentsQuery) (core.Page[core.Comment], error) {
	if q != (issuesvc.CommentsQuery{Repo: cli, Number: 8}) {
		return core.Page[core.Comment]{}, errors.New("not the query the modal reads")
	}
	return core.Page[core.Comment]{}, f.add("issue comments")
}

func (f issuesFake) CurrentGet(core.RepoRef, int) bool { return true }

func (f issuesFake) CurrentComments(issuesvc.CommentsQuery) bool { return true }

func TestRead(t *testing.T) {
	c := &calls{}
	r := Reader{Pulls: pullsFake{c}, Issues: issuesFake{c}}
	if err := r.Read(t.Context(), Key{Pull: true, Repo: cli, Number: 7}); err != nil {
		t.Fatal(err)
	}
	if err := r.Read(t.Context(), Key{Repo: cli, Number: 8}); err != nil {
		t.Fatal(err)
	}
	if len(c.what) != 4 {
		t.Errorf("read %v, want the detail and first comments of each", c.what)
	}
	c.err = core.ErrRateLimited
	if err := r.Read(t.Context(), Key{Pull: true, Repo: cli, Number: 7}); !errors.Is(err, core.ErrRateLimited) {
		t.Errorf("read = %v, want the rate limit", err)
	}
	if r.Current(Key{Pull: true, Repo: cli, Number: 7}) || !r.Current(Key{Repo: cli, Number: 8}) {
		t.Error("current doesn't ask the service of the kind")
	}
	if none := (Reader{}); !none.Current(Key{Pull: true}) || !none.Current(Key{}) {
		t.Error("without a service there should be nothing to read")
	}
}

func TestKinds(t *testing.T) {
	c := &calls{}
	r := Reader{Pulls: pullsFake{c}, Issues: issuesFake{c}}
	kinds := r.Kinds("pull")
	if len(kinds) != 2 || kinds[0].Name != "details" || kinds[1].Name != "comments" || kinds[1].Log != "pull_comments" {
		t.Fatalf("kinds = %+v, want details and comments", kinds)
	}
	k := Key{Pull: true, Repo: cli, Number: 7}
	if kinds[0].Current(k) || !kinds[1].Current(k) {
		t.Error("each kind should ask the service of its own reads whether they are cached")
	}
	if err := kinds[0].Read(t.Context(), k); err != nil {
		t.Fatal(err)
	}
	if err := kinds[1].Read(t.Context(), k); err != nil {
		t.Fatal(err)
	}
	if want := []string{"pull", "pull comments"}; len(c.what) != 2 || c.what[0] != want[0] || c.what[1] != want[1] {
		t.Errorf("read %v, want the detail, then apart from it the comments, %v", c.what, want)
	}
}
