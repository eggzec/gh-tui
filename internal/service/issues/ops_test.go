package issues

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
)

var enhancement = core.Label{Name: "enhancement", Color: "a2eeef"}

// prime returns a service that has cached issue 7 in two list pages, the
// open and the all one, and in its detail. Issue 8, also in both pages, has
// the enhancement label.
func prime(t *testing.T, api *fakeAPI) *Service {
	t.Helper()
	api.listIssues = func(state core.StateFilter, _ string, _ github.Conditional) (core.Page[core.Issue], github.Response, error) {
		p := page("", 7, 8)
		if state == core.FilterAll {
			p = page("", 9, 7, 8)
		}
		p.Items[len(p.Items)-1].Labels = []core.Label{enhancement}
		return p, ok(`"l1"`), nil
	}
	api.getIssue = func(number int, _ github.Conditional) (core.Issue, github.Response, error) {
		return issue(number), ok(`"i1"`), nil
	}
	api.listComments = func(int, github.Conditional) (core.Page[core.Comment], github.Response, error) {
		return thread(), ok(`"c1"`), nil
	}
	s := New(api, WithViewer("octocat"))
	for _, state := range []core.StateFilter{core.FilterOpen, core.FilterAll} {
		if _, err := s.List(t.Context(), ListQuery{Repo: repo, State: state}); err != nil {
			t.Fatalf("List: %v", err)
		}
	}
	if _, err := s.Get(t.Context(), repo, 7); err != nil {
		t.Fatalf("Get: %v", err)
	}
	api.called()
	return s
}

// view is everything the cache shows about issue 7.
type view struct {
	open, all core.Page[core.Issue]
	detail    core.IssueDetail
}

func look(t *testing.T, s *Service) view {
	t.Helper()
	var v view
	var okOpen, okAll, okDetail bool
	v.open, okOpen = s.CachedList(ListQuery{Repo: repo})
	v.all, okAll = s.CachedList(ListQuery{Repo: repo, State: core.FilterAll})
	v.detail, okDetail = s.CachedIssue(repo, 7)
	if !okOpen || !okAll || !okDetail {
		t.Fatalf("cached = %v, %v, %v; want the lists and the detail", okOpen, okAll, okDetail)
	}
	return v
}

// checkIssue7 reports an error, described by want, for each place the
// cache shows issue 7 in where ok is false.
func checkIssue7(t *testing.T, s *Service, want string, ok func(it core.Issue) bool) {
	t.Helper()
	v := look(t, s)
	if !ok(v.detail.Issue) {
		t.Errorf("detail = %+v, want %s", v.detail.Issue, want)
	}
	for name, p := range map[string]core.Page[core.Issue]{"open list": v.open, "all list": v.all} {
		i := slices.IndexFunc(p.Items, func(it core.Issue) bool { return it.Number == 7 })
		switch {
		case i < 0:
			t.Errorf("%s lost issue 7", name)
		case !ok(p.Items[i]):
			t.Errorf("issue 7 in the %s = %+v, want %s", name, p.Items[i], want)
		}
	}
}

func TestCloseAndReopen(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	var sent []core.State
	api.setState = func(number int, state core.State) (core.Issue, error) {
		sent = append(sent, state)
		it := issue(number)
		it.State = state
		it.Title = "Title from the server"
		return it, nil
	}

	for _, state := range []core.State{core.StateClosed, core.StateOpen} {
		op := s.Close(repo, 7)
		if state == core.StateOpen {
			op = s.Reopen(repo, 7)
		} else {
			checkIssue7(t, s, "the old title before Do", func(it core.Issue) bool { return it.Title == "Issue" })
		}
		checkIssue7(t, s, "the new state before Do", func(it core.Issue) bool { return it.State == state })
		if err := op.Do(t.Context()); err != nil {
			t.Fatalf("Do: %v", err)
		}
		checkIssue7(t, s, "the server's issue after Do", func(it core.Issue) bool {
			return it.State == state && it.Title == "Title from the server"
		})
	}
	if !slices.Equal(sent, []core.State{core.StateClosed, core.StateOpen}) {
		t.Errorf("states sent = %q, want closed then open", sent)
	}
	if v := look(t, s); v.open.Items[1].State != core.StateOpen {
		t.Error("the change reached issue 8")
	}
	api.checkCalls(t, "SetIssueState", "SetIssueState")

	// The lists may have changed order or members, so they revalidate. The
	// detail holds what the server sent, so it stays fresh.
	if _, err := s.List(t.Context(), ListQuery{Repo: repo}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, err := s.Get(t.Context(), repo, 7); err != nil {
		t.Fatalf("Get: %v", err)
	}
	api.checkCalls(t, "ListIssues")
}

func TestAddLabels(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	triage := core.Label{Name: "triage", Color: "ededed"}
	api.addLabels = func(_ int, names []string) ([]core.Label, error) {
		if !slices.Equal(names, []string{"Enhancement", "triage", "BUG"}) {
			t.Errorf("names sent = %q, want those given", names)
		}
		return []core.Label{{Name: "bug", Color: "d73a4a"}, enhancement, triage}, nil
	}

	op := s.AddLabels(repo, 7, []string{"Enhancement", "triage", "BUG"})
	// A label known from issue 8 shows its color; an unknown one only its
	// name; one the issue has, in any case, isn't added twice.
	want := []core.Label{{Name: "bug", Color: "d73a4a"}, enhancement, {Name: "triage"}}
	checkIssue7(t, s, fmt.Sprintf("labels %+v before Do", want), func(it core.Issue) bool { return slices.Equal(it.Labels, want) })
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	want[2] = triage
	checkIssue7(t, s, fmt.Sprintf("the server's labels %+v after Do", want), func(it core.Issue) bool { return slices.Equal(it.Labels, want) })
}

func TestRemoveLabel(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	api.removeLabel = func(_ int, name string) ([]core.Label, error) {
		if name != "Bug" {
			t.Errorf("name sent = %q, want Bug", name)
		}
		return nil, nil
	}

	op := s.RemoveLabel(repo, 7, "Bug")
	checkIssue7(t, s, "no labels before Do", func(it core.Issue) bool { return len(it.Labels) == 0 })
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	checkIssue7(t, s, "the server's nil labels after Do", func(it core.Issue) bool { return it.Labels == nil })
}

func TestComment(t *testing.T) {
	api := &fakeAPI{t: t}
	s := prime(t, api)
	posted := core.Comment{ID: "IC_2", Author: core.User{Login: "octocat"}, Body: "On it", CreatedAt: epoch, UpdatedAt: epoch}
	api.comment = func(_ int, body string) (core.Comment, error) {
		if body != "On it" {
			t.Errorf("body sent = %q, want On it", body)
		}
		return posted, nil
	}

	op := s.Comment(repo, 7, "On it")
	checkIssue7(t, s, "2 comments before Do", func(it core.Issue) bool { return it.Comments == 2 })
	th := look(t, s).detail.Thread
	if len(th) != 2 || th[0].ID != "IC_1" {
		t.Fatalf("thread before Do = %+v, want the old comment and a new one", th)
	}
	if c := th[1]; !IsPending(c) || c.Body != "On it" || c.Author.Login != "octocat" || c.CreatedAt.IsZero() {
		t.Errorf("new comment = %+v, want a pending one by the viewer", c)
	}

	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	th = look(t, s).detail.Thread
	if len(th) != 2 || th[0].ID != "IC_1" || th[1] != posted || IsPending(th[1]) {
		t.Errorf("thread after Do = %+v, want the pending comment replaced by the server's", th)
	}
	checkIssue7(t, s, "2 comments after Do", func(it core.Issue) bool { return it.Comments == 2 })
}

func TestCommentAfterRefetch(t *testing.T) {
	posted := core.Comment{ID: "IC_2", Body: "On it", CreatedAt: epoch, UpdatedAt: epoch}
	for _, refetched := range []bool{false, true} {
		t.Run(fmt.Sprintf("refetch has the comment=%v", refetched), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				api := &fakeAPI{t: t}
				s := prime(t, api)
				api.comment = func(int, string) (core.Comment, error) { return posted, nil }
				op := s.Comment(repo, 7, "On it")

				time.Sleep(pastTTL)
				api.listComments = func(int, github.Conditional) (core.Page[core.Comment], github.Response, error) {
					p := thread()
					if refetched {
						p.Items = append(p.Items, posted)
					}
					return p, ok(`"c2"`), nil
				}
				if _, err := s.Get(t.Context(), repo, 7); err != nil {
					t.Fatalf("Get: %v", err)
				}
				if err := op.Do(t.Context()); err != nil {
					t.Fatalf("Do: %v", err)
				}
				th := look(t, s).detail.Thread
				if len(th) != 2 || th[1] != posted {
					t.Errorf("thread = %+v, want the server's comment once", th)
				}
			})
		})
	}
}

// opCases start each change on issue 7 and name the API method that sends it.
var opCases = []struct {
	name, call string
	start      func(s *Service) *optimistic.Op
	// noop is set for a change that looks the same as the cached state.
	noop bool
}{
	{"Close", "SetIssueState", func(s *Service) *optimistic.Op { return s.Close(repo, 7) }, false},
	{"Reopen", "SetIssueState", func(s *Service) *optimistic.Op { return s.Reopen(repo, 7) }, true},
	{"AddLabels", "AddIssueLabels", func(s *Service) *optimistic.Op { return s.AddLabels(repo, 7, []string{"triage"}) }, false},
	{"RemoveLabel", "RemoveIssueLabel", func(s *Service) *optimistic.Op { return s.RemoveLabel(repo, 7, "bug") }, false},
	{"Comment", "CreateIssueComment", func(s *Service) *optimistic.Op { return s.Comment(repo, 7, "On it") }, false},
}

func TestOpFailureRollsBack(t *testing.T) {
	conflict := fmt.Errorf("github: 422 Validation Failed: %w", core.ErrConflict)
	for _, tc := range opCases {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{t: t}
			s := prime(t, api)
			api.setState = func(int, core.State) (core.Issue, error) { return core.Issue{}, conflict }
			api.addLabels = func(int, []string) ([]core.Label, error) { return nil, conflict }
			api.removeLabel = func(int, string) ([]core.Label, error) { return nil, conflict }
			api.comment = func(int, string) (core.Comment, error) { return core.Comment{}, conflict }

			before := look(t, s)
			op := tc.start(s)
			if changed := !reflect.DeepEqual(look(t, s), before); changed == tc.noop {
				t.Errorf("cache changed before Do = %v, want %v", changed, !tc.noop)
			}
			err := op.Do(t.Context())
			if !errors.Is(err, core.ErrConflict) || !strings.Contains(err.Error(), "octo-org/hello#7") {
				t.Errorf("Do error = %v, want a wrapped ErrConflict", err)
			}
			if after := look(t, s); !reflect.DeepEqual(after, before) {
				t.Errorf("after rollback = %+v, want %+v", after, before)
			}
			api.checkCalls(t, tc.call)
		})
	}
}

func TestOpOnUncachedIssueSends(t *testing.T) {
	for _, tc := range opCases {
		t.Run(tc.name, func(t *testing.T) {
			api := &fakeAPI{
				t:           t,
				setState:    func(n int, _ core.State) (core.Issue, error) { return issue(n), nil },
				addLabels:   func(int, []string) ([]core.Label, error) { return nil, nil },
				removeLabel: func(int, string) ([]core.Label, error) { return nil, nil },
				comment:     func(int, string) (core.Comment, error) { return core.Comment{ID: "IC_2"}, nil },
			}
			s := New(api)
			if err := tc.start(s).Do(t.Context()); err != nil {
				t.Fatalf("Do: %v", err)
			}
			api.checkCalls(t, tc.call)
			if _, cached := s.CachedIssue(repo, 7); cached {
				t.Error("the server's answer was cached without a fetch")
			}
		})
	}
}
