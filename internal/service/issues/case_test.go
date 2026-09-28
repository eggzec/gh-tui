package issues

import (
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// shouted is repo as a user may type it: GitHub ignores the case of names.
var shouted = core.RepoRef{Owner: "OCTO-org", Name: "Hello"}

// What was read under one spelling of a repository is found under any.
func TestReadsIgnoreCase(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	if _, ok := s.CachedList(ListQuery{Repo: shouted}); !ok {
		t.Error("CachedList under another case missed")
	}
	if _, ok := s.CachedGet(shouted, 7); !ok {
		t.Error("CachedGet under another case missed")
	}
	for _, q := range primed {
		q.Repo = shouted
		if _, ok := s.CachedComments(q); !ok {
			t.Errorf("CachedComments(%+v) under another case missed", q)
		}
	}
	if _, err := s.Get(t.Context(), shouted, 7); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := s.List(t.Context(), ListQuery{Repo: shouted}); err != nil {
		t.Fatalf("List: %v", err)
	}
	api.checkCalls(t)
}

func TestKindIgnoresCase(t *testing.T) {
	api := kindAPI(t)
	s := New(api)
	kind(t, s, 7)
	api.checkCalls(t, "GetIssueKind")
	if k, ok := s.CachedKind(shouted, 7); !ok || k != core.KindIssue {
		t.Errorf("CachedKind under another case = %q, %v; want issue", k, ok)
	}
	if k, err := s.Kind(t.Context(), shouted, 7); err != nil || k != core.KindIssue {
		t.Errorf("Kind under another case = %q, %v; want issue", k, err)
	}
	api.checkCalls(t)
}

// A change under one spelling reaches what was read under another.
func TestChangesIgnoreCase(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	s.Invalidate(shouted)
	if _, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil {
		t.Fatalf("List: %v", err)
	}
	api.checkCalls(t, "ListIssues")

	api.setState = func(number int, state core.State) (core.Issue, error) {
		it := issue(number)
		it.State = state
		return it, nil
	}
	op := s.Close(shouted, 7)
	checkIssue7(t, s, "closed before Do", func(it core.Issue) bool { return it.State == core.StateClosed })
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if _, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil {
		t.Fatalf("List: %v", err)
	}
	api.checkCalls(t, "SetIssueState", "ListIssues")
}

// Labels added under another spelling take the colors the repository's
// cached issues show.
func TestLabelsIgnoreCase(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	api.addLabels = func(int, []string) ([]core.Label, error) { return []core.Label{enhancement}, nil }
	_ = s.AddLabels(shouted, 7, []string{"enhancement"})
	checkIssue7(t, s, "the known label's color", func(it core.Issue) bool { return slices.Contains(it.Labels, enhancement) })
}
