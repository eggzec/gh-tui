package pulls

import (
	"errors"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// labeled gives #1 the labels and assignees that the edits start from. The
// names differ in case from what the tests remove.
func labeled(pr *core.PullRequest) {
	pr.Labels = []core.Label{{Name: "bug", Color: "d73a4a"}, {Name: "Docs", Color: "0075ca"}}
	pr.Assignees = []core.User{{Login: "alice"}, {Login: "Bob"}}
}

func labelNames(ls []core.Label) []string {
	out := make([]string, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.Name)
	}
	return out
}

func logins(us []core.User) []string {
	out := make([]string, 0, len(us))
	for _, u := range us {
		out = append(out, u.Login)
	}
	return out
}

var feature = core.Label{Name: "feature", Color: "a2eeef", Description: "New"}

func TestEditLabelsShownBeforeDo(t *testing.T) {
	api := &fakeAPI{}
	s := seeded(t, api, labeled)
	before := take(t, s)
	old := before.first.Items[0].Labels
	oldCopy := slices.Clone(old)

	s.EditLabels(repo, 1, []core.Label{feature, {Name: "DOCS"}}, []string{"BUG"})

	after := take(t, s)
	want := []core.Label{{Name: "Docs", Color: "0075ca"}, feature}
	if got := after.first.Items[0].Labels; !reflect.DeepEqual(got, want) {
		t.Errorf("list labels = %+v, want %+v", got, want)
	}
	if got := after.detail.Labels; !reflect.DeepEqual(got, want) {
		t.Errorf("detail labels = %+v, want %+v", got, want)
	}
	if after.detail.Body != "The body." {
		t.Errorf("detail body = %q, want it kept", after.detail.Body)
	}
	if !reflect.DeepEqual(after.first.Items[1], before.first.Items[1]) || !reflect.DeepEqual(after.second, before.second) {
		t.Error("the edit touched other pull requests")
	}
	if !slices.Equal(old, oldCopy) {
		t.Errorf("the old labels are now %+v, want %+v: the edit changed them in place", old, oldCopy)
	}
	if got := api.labelCalls(); len(got) != 0 {
		t.Errorf("calls = %v, want none before Do", got)
	}
}

func TestEditLabelsRemoveOnly(t *testing.T) {
	api := &fakeAPI{}
	api.removeLabel = func(string) ([]core.Label, error) { return []core.Label{{Name: "Docs"}}, nil }
	s := seeded(t, api, labeled)

	op := s.EditLabels(repo, 1, nil, []string{"BUG"})
	if got := labelNames(take(t, s).detail.Labels); !slices.Equal(got, []string{"Docs"}) {
		t.Errorf("labels = %v, want [Docs]", got)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got, want := api.labelCalls(), []string{"RemoveIssueLabel BUG"}; !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestEditLabelsSuccessReplacesWithGitHubs(t *testing.T) {
	api := &fakeAPI{}
	ghost := core.Label{Name: "ghost", Color: "ffffff"}
	// GitHub keeps docs and the new feature, drops what was asked to add
	// as ghost, and has a label that was added on the web meanwhile.
	server := []core.Label{{Name: "Docs", Color: "0075ca"}, {Name: "feature", Color: "111111"}, {Name: "web", Color: "222222"}}
	api.addLabels = func([]string) ([]core.Label, error) {
		return []core.Label{{Name: "bug"}, {Name: "Docs", Color: "0075ca"}, {Name: "feature", Color: "111111"}, {Name: "web", Color: "222222"}}, nil
	}
	api.removeLabel = func(string) ([]core.Label, error) { return server, nil }
	s := seeded(t, api, labeled)

	op := s.EditLabels(repo, 1, []core.Label{feature, ghost}, []string{"bug"})
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}

	after := take(t, s)
	if got := after.first.Items[0].Labels; !reflect.DeepEqual(got, server) {
		t.Errorf("list labels = %+v, want GitHub's %+v", got, server)
	}
	if got := after.detail.Labels; !reflect.DeepEqual(got, server) {
		t.Errorf("detail labels = %+v, want GitHub's %+v", got, server)
	}
	want := []string{"AddIssueLabels feature,ghost", "RemoveIssueLabel bug"}
	if got := api.labelCalls(); !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n := api.count("get"); n != 1 {
		t.Errorf("get called %d times, want 1: the confirmed detail is fresh", n)
	}
}

func TestEditLabelsFailureRollsBack(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(api *fakeAPI)
	}{
		{"add fails", func(api *fakeAPI) {
			api.addLabels = func([]string) ([]core.Label, error) { return nil, core.ErrConflict }
		}},
		{"remove fails after the add landed", func(api *fakeAPI) {
			api.removeLabel = func(string) ([]core.Label, error) { return nil, core.ErrConflict }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{}
			tc.set(api)
			s := seeded(t, api, labeled)
			before := take(t, s)

			err := s.EditLabels(repo, 1, []core.Label{feature}, []string{"bug"}).Do(t.Context())
			if !errors.Is(err, core.ErrConflict) {
				t.Fatalf("Do = %v, want ErrConflict", err)
			}
			if !strings.Contains(err.Error(), "label pull eggzec/gh-tui#1") {
				t.Errorf("error %q lacks context", err)
			}
			if after := take(t, s); !reflect.DeepEqual(after, before) {
				t.Errorf("after rollback =\n%+v\nwant\n%+v", after, before)
			}
			if _, err := s.Get(t.Context(), repo, 1); err != nil {
				t.Fatalf("Get: %v", err)
			}
			if n := api.count("get"); n != 2 {
				t.Errorf("get called %d times, want 2: the detail must be stale after a failure", n)
			}
		})
	}
}

func TestEditLabelsRefused(t *testing.T) {
	api := &fakeAPI{}
	s := seeded(t, api, labeled)
	s.access, s.repos = tokenWith(classic("gist"), true), nil
	before, sent := take(t, s), api.callsSoFar()

	err := s.EditLabels(repo, 1, []core.Label{feature}, []string{"bug"}).Do(t.Context())
	if _, ok := errors.AsType[*core.ScopeError](err); !ok {
		t.Fatalf("Do = %v, want a ScopeError", err)
	}
	if after := take(t, s); !reflect.DeepEqual(after, before) {
		t.Errorf("a refused edit was shown:\n%+v\nwant\n%+v", after, before)
	}
	if got := api.callsSoFar(); !maps.Equal(got, sent) || len(api.labelCalls()) != 0 {
		t.Errorf("calls = %v %v, want nothing sent", got, api.labelCalls())
	}
}

func TestEditLabelsRemoveNotFoundIsDone(t *testing.T) {
	api := &fakeAPI{}
	api.removeLabel = func(name string) ([]core.Label, error) {
		if name == "gone" {
			return nil, core.ErrNotFound
		}
		return []core.Label{{Name: "Docs"}}, nil
	}
	s := seeded(t, api, labeled)

	if err := s.EditLabels(repo, 1, nil, []string{"gone", "bug"}).Do(t.Context()); err != nil {
		t.Fatalf("Do = %v, want nil: a label already gone is not an error", err)
	}
	if got := labelNames(take(t, s).detail.Labels); !slices.Equal(got, []string{"Docs"}) {
		t.Errorf("labels = %v, want [Docs]", got)
	}
	if got := api.labelCalls(); len(got) != 2 {
		t.Errorf("calls = %v, want both removes tried", got)
	}
}

func TestEditNothingSendsNothing(t *testing.T) {
	api := &fakeAPI{}
	s := seeded(t, api, labeled)
	before := take(t, s)

	if err := s.EditLabels(repo, 1, nil, nil).Do(t.Context()); err != nil {
		t.Errorf("EditLabels Do = %v", err)
	}
	if err := s.EditAssignees(repo, 1, nil, nil).Do(t.Context()); err != nil {
		t.Errorf("EditAssignees Do = %v", err)
	}
	if got := api.labelCalls(); len(got) != 0 {
		t.Errorf("calls = %v, want none", got)
	}
	if after := take(t, s); !reflect.DeepEqual(after, before) {
		t.Error("an empty edit changed the cache")
	}
}

var carol = core.User{Login: "carol", Name: "Carol"}

func TestEditAssigneesShownBeforeDo(t *testing.T) {
	api := &fakeAPI{}
	s := seeded(t, api, labeled)
	before := take(t, s)
	old := before.first.Items[0].Assignees
	oldCopy := slices.Clone(old)

	s.EditAssignees(repo, 1, []core.User{carol, {Login: "ALICE"}}, []string{"bob"})

	after := take(t, s)
	want := []core.User{{Login: "alice"}, carol}
	if got := after.first.Items[0].Assignees; !reflect.DeepEqual(got, want) {
		t.Errorf("list assignees = %+v, want %+v", got, want)
	}
	if got := after.detail.Assignees; !reflect.DeepEqual(got, want) {
		t.Errorf("detail assignees = %+v, want %+v", got, want)
	}
	if !reflect.DeepEqual(after.second, before.second) {
		t.Error("the edit touched other pull requests")
	}
	if !slices.Equal(old, oldCopy) {
		t.Errorf("the old assignees are now %+v, want %+v: the edit changed them in place", old, oldCopy)
	}
	if got := api.labelCalls(); len(got) != 0 {
		t.Errorf("calls = %v, want none before Do", got)
	}
}

func TestEditAssigneesSuccessTakesTheReturnedIssue(t *testing.T) {
	api := &fakeAPI{}
	// GitHub skips carol, who may not be assigned, and the issue it returns
	// has the labels it has now.
	left := core.Issue{
		Assignees: []core.User{{Login: "alice"}},
		Labels:    []core.Label{{Name: "web", Color: "222222"}},
	}
	api.addAssignees = func([]string) (core.Issue, error) {
		return core.Issue{Assignees: []core.User{{Login: "alice"}, {Login: "Bob"}, carol}}, nil
	}
	api.removeAssignees = func([]string) (core.Issue, error) { return left, nil }
	s := seeded(t, api, labeled)

	if err := s.EditAssignees(repo, 1, []core.User{carol}, []string{"Bob"}).Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}

	after := take(t, s)
	for name, pr := range map[string]core.PullRequest{"list": after.first.Items[0], "detail": after.detail.PullRequest} {
		if got := logins(pr.Assignees); !slices.Equal(got, []string{"alice"}) {
			t.Errorf("%s assignees = %v, want GitHub's [alice]", name, got)
		}
		if !reflect.DeepEqual(pr.Labels, left.Labels) {
			t.Errorf("%s labels = %+v, want GitHub's %+v", name, pr.Labels, left.Labels)
		}
	}
	want := []string{"AddIssueAssignees carol", "RemoveIssueAssignees Bob"}
	if got := api.labelCalls(); !slices.Equal(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}
}

func TestEditAssigneesFailureRollsBack(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(api *fakeAPI)
	}{
		{"add fails", func(api *fakeAPI) {
			api.addAssignees = func([]string) (core.Issue, error) { return core.Issue{}, core.ErrConflict }
		}},
		{"remove fails after the add landed", func(api *fakeAPI) {
			api.removeAssignees = func([]string) (core.Issue, error) { return core.Issue{}, core.ErrConflict }
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{}
			tc.set(api)
			s := seeded(t, api, labeled)
			before := take(t, s)

			err := s.EditAssignees(repo, 1, []core.User{carol}, []string{"alice"}).Do(t.Context())
			if !errors.Is(err, core.ErrConflict) {
				t.Fatalf("Do = %v, want ErrConflict", err)
			}
			if !strings.Contains(err.Error(), "assign pull eggzec/gh-tui#1") {
				t.Errorf("error %q lacks context", err)
			}
			if after := take(t, s); !reflect.DeepEqual(after, before) {
				t.Errorf("after rollback =\n%+v\nwant\n%+v", after, before)
			}
			if _, err := s.Get(t.Context(), repo, 1); err != nil {
				t.Fatalf("Get: %v", err)
			}
			if n := api.count("get"); n != 2 {
				t.Errorf("get called %d times, want 2: the detail must be stale after a failure", n)
			}
		})
	}
}

func TestEditAssigneesRefused(t *testing.T) {
	api := &fakeAPI{}
	s := seeded(t, api, labeled)
	s.access, s.repos = tokenWith(classic("gist"), true), nil
	before := take(t, s)

	err := s.EditAssignees(repo, 1, []core.User{carol}, nil).Do(t.Context())
	if _, ok := errors.AsType[*core.ScopeError](err); !ok {
		t.Fatalf("Do = %v, want a ScopeError", err)
	}
	if after := take(t, s); !reflect.DeepEqual(after, before) {
		t.Errorf("a refused edit was shown:\n%+v\nwant\n%+v", after, before)
	}
	if got := api.labelCalls(); len(got) != 0 {
		t.Errorf("calls = %v, want nothing sent", got)
	}
}
