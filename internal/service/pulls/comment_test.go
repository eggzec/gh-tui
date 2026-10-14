package pulls

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

// twoPages serves comments c1 and c2 on a first page of two and c3 on the last.
func twoPages(_ context.Context, _ core.RepoRef, _ int, cursor string, _ int) (core.Page[core.Comment], error) {
	if cursor == "p2" {
		return core.Page[core.Comment]{Items: []core.Comment{{ID: "c3"}}}, nil
	}
	return core.Page[core.Comment]{Items: []core.Comment{{ID: "c1"}, {ID: "c2"}}, Next: "p2"}, nil
}

var (
	firstPage = CommentsQuery{Repo: repo, Number: 1, PageSize: 2}
	lastPage  = CommentsQuery{Repo: repo, Number: 1, PageSize: 2, Cursor: "p2"}
)

// readCommentPages reads both pages, so that they are cached.
func readCommentPages(t *testing.T, s *Service) {
	t.Helper()
	for _, q := range []CommentsQuery{firstPage, lastPage} {
		if _, err := s.Comments(t.Context(), q); err != nil {
			t.Fatalf("Comments: %v", err)
		}
	}
}

func commentIDs(t *testing.T, s *Service, q CommentsQuery) []string {
	t.Helper()
	p, ok := s.CachedComments(q)
	if !ok {
		t.Fatalf("comments %+v aren't cached", q)
	}
	var ids []string
	for _, c := range p.Items {
		ids = append(ids, c.ID)
	}
	return ids
}

func TestCommentShowsAtOnceOnTheLastPage(t *testing.T) {
	api := &fakeAPI{comments: twoPages}
	s := New(api)
	readCommentPages(t, s)

	op := s.Comment(repo, 1, "On it", "octocat")
	if got := commentIDs(t, s, firstPage); !slices.Equal(got, []string{"c1", "c2"}) {
		t.Errorf("first page = %q, want it unchanged", got)
	}
	got := commentIDs(t, s, lastPage)
	if len(got) != 2 || got[0] != "c3" {
		t.Fatalf("last page = %q, want c3 and the pending comment", got)
	}
	p, _ := s.CachedComments(lastPage)
	if c := p.Items[1]; !IsPending(c) || c.Body != "On it" || c.Author.Login != "octocat" || c.CreatedAt.IsZero() {
		t.Errorf("pending comment = %+v, want it pending, by octocat, with the text and a time", c)
	}
	if n := api.count("comment 1"); n != 0 {
		t.Errorf("comments posted before Do = %d, want 0", n)
	}

	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if got := commentIDs(t, s, lastPage); !slices.Equal(got, []string{"c3", "C_real"}) {
		t.Errorf("last page after Do = %q, want GitHub's comment in place of the pending one", got)
	}
	if n := api.count("comment 1"); n != 1 {
		t.Errorf("comments posted = %d, want 1", n)
	}
}

func TestCommentRollsBackWhenRefused(t *testing.T) {
	api := &fakeAPI{comments: twoPages, commentErr: errors.New("nope")}
	s := New(api)
	readCommentPages(t, s)
	before := [2][]string{commentIDs(t, s, firstPage), commentIDs(t, s, lastPage)}

	op := s.Comment(repo, 1, "On it", "octocat")
	if err := op.Do(t.Context()); !errors.Is(err, api.commentErr) {
		t.Fatalf("Do = %v, want the refusal", err)
	}
	after := [2][]string{commentIDs(t, s, firstPage), commentIDs(t, s, lastPage)}
	if !reflect.DeepEqual(after, before) {
		t.Errorf("pages after the failure = %q, want %q", after, before)
	}
}

func TestCommentWithoutTheRightIsNotShown(t *testing.T) {
	api := &fakeAPI{comments: twoPages}
	s := New(api, WithAccess(tokenWith(classic("gist"), true)))
	readCommentPages(t, s)

	op := s.Comment(repo, 1, "On it", "octocat")
	if got := commentIDs(t, s, lastPage); !slices.Equal(got, []string{"c3"}) {
		t.Errorf("last page = %q, want it unchanged", got)
	}
	err := op.Do(t.Context())
	if _, ok := errors.AsType[*core.ScopeError](err); !ok {
		t.Fatalf("Do = %v, want a ScopeError", err)
	}
	if n := api.count("comment 1"); n != 0 {
		t.Errorf("comments posted = %d, want none", n)
	}
}
