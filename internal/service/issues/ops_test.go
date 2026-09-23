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

// Comment pages that prime caches. Issue 7 has three comments. The pages of
// two come in full, so the thread has a last page cached for sizes 2 and 3,
// but not for size 1.
var (
	firstOfTwo  = CommentsQuery{Repo: repo, Number: 7, PageSize: 2}
	lastOfTwo   = CommentsQuery{Repo: repo, Number: 7, PageSize: 2, Cursor: "offset=2"}
	onlyOfThree = CommentsQuery{Repo: repo, Number: 7, PageSize: 3}
	firstOfOne  = CommentsQuery{Repo: repo, Number: 7, PageSize: 1}
	primed      = []CommentsQuery{firstOfTwo, lastOfTwo, onlyOfThree, firstOfOne}
)

// prime returns a service that has cached issue 7 in two list pages, the
// open and the all one, in its detail and in the primed comment pages.
// Issue 8, also in both lists, has the enhancement label.
func prime(t *testing.T, api *fakeAPI) *Service {
	t.Helper()
	api.listIssues = func(state core.StateFilter, _ string, _ int, _ github.Conditional) (core.Page[core.Issue], github.Response, error) {
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
	api.listComments = func(_ int, cursor string, perPage int, _ github.Conditional) (core.Page[core.Comment], github.Response, error) {
		return commentPage(t, thread(3), cursor, perPage), ok(`"c1"`), nil
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
	for _, q := range primed {
		if _, err := s.Comments(t.Context(), q); err != nil {
			t.Fatalf("Comments: %v", err)
		}
	}
	api.called()
	return s
}

// view is everything the cache shows about issue 7.
type view struct {
	open, all core.Page[core.Issue]
	detail    core.Issue
	comments  map[CommentsQuery]core.Page[core.Comment]
}

func look(t *testing.T, s *Service) view {
	t.Helper()
	v := view{comments: make(map[CommentsQuery]core.Page[core.Comment])}
	var okOpen, okAll, okDetail bool
	v.open, okOpen = s.CachedList(ListQuery{Repo: repo})
	v.all, okAll = s.CachedList(ListQuery{Repo: repo, State: core.FilterAll})
	v.detail, okDetail = s.CachedGet(repo, 7)
	if !okOpen || !okAll || !okDetail {
		t.Fatalf("cached = %v, %v, %v; want the lists and the detail", okOpen, okAll, okDetail)
	}
	for _, q := range primed {
		p, cached := s.CachedComments(q)
		if !cached {
			t.Fatalf("comment page %+v isn't cached", q)
		}
		v.comments[q] = p
	}
	return v
}

// checkIssue7 reports an error, described by want, for each place the
// cache shows issue 7 in where ok is false.
func checkIssue7(t *testing.T, s *Service, want string, ok func(it core.Issue) bool) {
	t.Helper()
	v := look(t, s)
	if !ok(v.detail) {
		t.Errorf("detail = %+v, want %s", v.detail, want)
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

// checkComments reports an error for each primed page whose comment IDs
// differ from want, which holds the pages that differ from before any
// comment was added.
func checkComments(t *testing.T, s *Service, when string, want map[CommentsQuery][]string) {
	t.Helper()
	unchanged := map[CommentsQuery][]string{
		firstOfTwo:  {"IC_1", "IC_2"},
		lastOfTwo:   {"IC_3"},
		onlyOfThree: {"IC_1", "IC_2", "IC_3"},
		firstOfOne:  {"IC_1"},
	}
	for q, p := range look(t, s).comments {
		w, ok := want[q]
		if !ok {
			w = unchanged[q]
		}
		if got := ids(p); !slices.Equal(got, w) {
			t.Errorf("comments %d/%q %s = %q, want %q", q.PageSize, q.Cursor, when, got, w)
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
	posted := core.Comment{ID: "IC_4", Author: core.User{Login: "octocat"}, Body: "On it", CreatedAt: epoch, UpdatedAt: epoch}
	api.comment = func(_ int, body string) (core.Comment, error) {
		if body != "On it" {
			t.Errorf("body sent = %q, want On it", body)
		}
		return posted, nil
	}

	op := s.Comment(repo, 7, "On it")
	checkIssue7(t, s, "2 comments before Do", func(it core.Issue) bool { return it.Comments == 2 })
	// Only the last pages show the new comment.
	v := look(t, s)
	var pendingID string
	for _, q := range []CommentsQuery{lastOfTwo, onlyOfThree} {
		items := v.comments[q].Items
		c := items[len(items)-1]
		if !IsPending(c) || c.Body != "On it" || c.Author.Login != "octocat" || c.CreatedAt.IsZero() {
			t.Errorf("last comment of %d/%q = %+v, want a pending one by the viewer", q.PageSize, q.Cursor, c)
		}
		if pendingID != "" && c.ID != pendingID {
			t.Errorf("pending IDs %q and %q differ between pages", pendingID, c.ID)
		}
		pendingID = c.ID
	}
	checkComments(t, s, "before Do", map[CommentsQuery][]string{
		lastOfTwo:   {"IC_3", pendingID},
		onlyOfThree: {"IC_1", "IC_2", "IC_3", pendingID},
	})

	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	checkComments(t, s, "after Do", map[CommentsQuery][]string{
		lastOfTwo:   {"IC_3", "IC_4"},
		onlyOfThree: {"IC_1", "IC_2", "IC_3", "IC_4"},
	})
	if got := look(t, s).comments[lastOfTwo].Items[1]; got != posted {
		t.Errorf("comment after Do = %+v, want the server's %+v", got, posted)
	}
	checkIssue7(t, s, "2 comments after Do", func(it core.Issue) bool { return it.Comments == 2 })
}

func TestCommentWithoutLastPage(t *testing.T) {
	posted := comment(4)
	server := thread(3)
	api := &fakeAPI{
		t: t,
		getIssue: func(number int, _ github.Conditional) (core.Issue, github.Response, error) {
			return issue(number), ok(`"i1"`), nil
		},
		listComments: func(_ int, cursor string, perPage int, _ github.Conditional) (core.Page[core.Comment], github.Response, error) {
			return commentPage(t, server, cursor, perPage), ok(`"c1"`), nil
		},
		comment: func(int, string) (core.Comment, error) {
			server = append(server, posted)
			return posted, nil
		},
	}
	s := New(api)
	if _, err := s.Get(t.Context(), repo, 7); err != nil {
		t.Fatalf("Get: %v", err)
	}
	cached := func() map[CommentsQuery]core.Page[core.Comment] {
		t.Helper()
		out := make(map[CommentsQuery]core.Page[core.Comment])
		for _, q := range []CommentsQuery{firstOfTwo, firstOfOne} {
			p, ok := s.CachedComments(q)
			if !ok {
				t.Fatalf("comment page %+v isn't cached", q)
			}
			out[q] = p
		}
		return out
	}
	for _, q := range []CommentsQuery{firstOfTwo, firstOfOne} {
		if _, err := s.Comments(t.Context(), q); err != nil {
			t.Fatalf("Comments: %v", err)
		}
	}
	before := cached()

	op := s.Comment(repo, 7, "On it")
	if got, _ := s.CachedGet(repo, 7); got.Comments != 2 {
		t.Errorf("comment count before Do = %d, want 2", got.Comments)
	}
	if got := cached(); !reflect.DeepEqual(got, before) {
		t.Errorf("pages before Do = %+v, want them unchanged", got)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := cached(); !reflect.DeepEqual(got, before) {
		t.Errorf("pages after Do = %+v, want them unchanged", got)
	}
	// Paging to the end shows the comment.
	last, err := s.Comments(t.Context(), CommentsQuery{Repo: repo, Number: 7, PageSize: 2, Cursor: before[firstOfTwo].Next})
	if err != nil {
		t.Fatalf("Comments: %v", err)
	}
	if !slices.Equal(ids(last), []string{"IC_3", "IC_4"}) {
		t.Errorf("last page = %q, want the new comment at the end", ids(last))
	}
}

func TestCommentAfterRefetch(t *testing.T) {
	posted := comment(4)
	tests := []struct {
		name string
		// server is the thread GitHub serves the refetch from.
		server []core.Comment
		want   map[CommentsQuery][]string
	}{
		{
			// The refetch ran before GitHub stored the comment, so it
			// dropped the pending one.
			name:   "without the comment",
			server: thread(3),
			want: map[CommentsQuery][]string{
				lastOfTwo:   {"IC_3", "IC_4"},
				onlyOfThree: {"IC_1", "IC_2", "IC_3", "IC_4"},
			},
		},
		{
			// The refetch brought the comment, which fills the last page of
			// two and starts a new page of three, so it isn't added again.
			name:   "with the comment",
			server: append(thread(3), posted),
			want:   map[CommentsQuery][]string{lastOfTwo: {"IC_3", "IC_4"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				api := &fakeAPI{t: t}
				s := prime(t, api)
				api.comment = func(int, string) (core.Comment, error) { return posted, nil }
				op := s.Comment(repo, 7, "On it")

				time.Sleep(pastTTL)
				api.listComments = func(_ int, cursor string, perPage int, _ github.Conditional) (core.Page[core.Comment], github.Response, error) {
					return commentPage(t, tt.server, cursor, perPage), ok(`"c2"`), nil
				}
				for _, q := range primed {
					if _, err := s.Comments(t.Context(), q); err != nil {
						t.Fatalf("Comments: %v", err)
					}
				}
				if err := op.Do(t.Context()); err != nil {
					t.Fatalf("Do: %v", err)
				}
				checkComments(t, s, "after Do", tt.want)
				if p, _ := s.CachedComments(onlyOfThree); p.Last() != (len(tt.server) == 3) {
					t.Errorf("page of three Next = %q, want the server's", p.Next)
				}
			})
		})
	}
}

func TestCommentRollbackKeepsRefetchedPage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{t: t}
		s := prime(t, api)
		api.comment = func(int, string) (core.Comment, error) {
			return core.Comment{}, fmt.Errorf("github: 422 Validation Failed: %w", core.ErrConflict)
		}
		before := look(t, s)
		op := s.Comment(repo, 7, "On it")

		// Another comment arrives on the page of two while this one is sent.
		time.Sleep(pastTTL)
		api.listComments = func(_ int, cursor string, perPage int, _ github.Conditional) (core.Page[core.Comment], github.Response, error) {
			return commentPage(t, thread(4), cursor, perPage), ok(`"c2"`), nil
		}
		if _, err := s.Comments(t.Context(), lastOfTwo); err != nil {
			t.Fatalf("Comments: %v", err)
		}
		if err := op.Do(t.Context()); !errors.Is(err, core.ErrConflict) {
			t.Fatalf("Do error = %v, want ErrConflict", err)
		}

		after := look(t, s)
		if got := ids(after.comments[lastOfTwo]); !slices.Equal(got, []string{"IC_3", "IC_4"}) {
			t.Errorf("refetched page = %q, want what the server sent", got)
		}
		delete(before.comments, lastOfTwo)
		delete(after.comments, lastOfTwo)
		if !reflect.DeepEqual(after, before) {
			t.Errorf("after rollback = %+v, want %+v", after, before)
		}
	})
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
			if _, cached := s.CachedGet(repo, 7); cached {
				t.Error("the server's answer was cached without a fetch")
			}
			if _, cached := s.CachedComments(CommentsQuery{Repo: repo, Number: 7}); cached {
				t.Error("the server's comment was cached without a fetch")
			}
		})
	}
}
