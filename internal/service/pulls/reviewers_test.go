package pulls

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/cache"
	"github.com/eggzec/gh-tui/internal/core"
)

// askedOf is a pull request whose reviewers were asked as three requests, one
// of a user, one of a team and one the token can't see, and in which a fourth
// person, bo, reviewed.
func askedOf(number int) core.PullRequestDetail {
	return core.PullRequestDetail{
		Number: number, ID: "PR_1",
		Reviewers: core.Reviewers{
			Requested: []core.ReviewRequest{
				{User: core.User{Login: "ana"}},
				{Team: "octo/core", TeamName: "Core"},
				{Unknown: true},
			},
			RequestedTotal: 3,
			Verdicts:       []core.Verdict{{Author: core.User{Login: "bo"}}},
			VerdictsTotal:  1,
		},
	}
}

// reviewed returns a service that has read askedOf pull request 1.
func reviewed(t *testing.T, api *fakeAPI) *Service {
	t.Helper()
	api.get = func(_ context.Context, _ core.RepoRef, number int) (core.PullRequestDetail, error) {
		return askedOf(number), nil
	}
	s := New(api)
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatalf("Get: %v", err)
	}
	return s
}

func reviewersOf(t *testing.T, s *Service) core.Reviewers {
	t.Helper()
	d, ok := s.CachedGet(repo, 1)
	if !ok {
		t.Fatal("the detail is not cached")
	}
	return d.Reviewers
}

func TestRequestReviewsShownBeforeDo(t *testing.T) {
	api := &fakeAPI{}
	s := reviewed(t, api)
	snapshot := reviewersOf(t, s)

	op := s.RequestReviews(repo, 1, []Reviewer{{Login: "cy"}, {Team: "octo/docs"}})
	got := reviewersOf(t, s)
	want := append(slices.Clone(snapshot.Requested),
		core.ReviewRequest{User: core.User{Login: "cy"}},
		core.ReviewRequest{Team: "octo/docs", TeamName: "docs"})
	if !reflect.DeepEqual(got.Requested, want) || got.RequestedTotal != 5 {
		t.Errorf("reviewers = %+v, want the requests %+v and the total 5", got, want)
	}
	if n := api.reviewCalls(); n != 0 {
		t.Errorf("%d calls before Do, want 0", n)
	}
	if len(snapshot.Requested) != 3 || snapshot.RequestedTotal != 3 {
		t.Errorf("the snapshot is now %+v: the change modified it in place", snapshot)
	}

	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	wantCall := []reviewCall{{1, []string{"cy"}, []string{"docs"}}}
	if !reflect.DeepEqual(api.requested, wantCall) {
		t.Errorf("sent %+v, want %+v: logins, and slugs without the organization", api.requested, wantCall)
	}
}

func TestRequestReviewsAsksAgainWhoReviewed(t *testing.T) {
	api := &fakeAPI{}
	s := reviewed(t, api)
	if err := s.RequestReviews(repo, 1, []Reviewer{{Login: "BO"}}).Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if want := []reviewCall{{1, []string{"BO"}, nil}}; !reflect.DeepEqual(api.requested, want) {
		t.Errorf("sent %+v, want %+v", api.requested, want)
	}
}

func TestRequestReviewsFailureRollsBack(t *testing.T) {
	boom := errors.New("boom")
	api := &fakeAPI{}
	api.reviewErr = boom
	s := reviewed(t, api)
	before := reviewersOf(t, s)

	op := s.RequestReviews(repo, 1, []Reviewer{{Login: "cy"}})
	if reflect.DeepEqual(reviewersOf(t, s), before) {
		t.Fatal("the request was not shown")
	}
	err := op.Do(t.Context())
	if !errors.Is(err, boom) {
		t.Fatalf("Do = %v, want boom", err)
	}
	if got := reviewersOf(t, s); !reflect.DeepEqual(got, before) {
		t.Errorf("reviewers = %+v, want the old %+v", got, before)
	}
}

func TestReviewChangeMarksStale(t *testing.T) {
	for name, run := range map[string]func(s *Service) error{
		"request": func(s *Service) error {
			return s.RequestReviews(repo, 1, []Reviewer{{Login: "cy"}}).Do(t.Context())
		},
		"withdraw": func(s *Service) error {
			return s.RemoveReviewRequests(repo, 1, []Reviewer{{Login: "ana"}}).Do(t.Context())
		},
	} {
		t.Run(name, func(t *testing.T) {
			api := &fakeAPI{}
			s := reviewed(t, api)
			if err := run(s); err != nil {
				t.Fatalf("Do: %v", err)
			}
			if _, ok := s.CachedGet(repo, 1); !ok {
				t.Error("the detail was dropped, want it kept to show until the next read")
			}
			if _, err := s.Get(t.Context(), repo, 1); err != nil || api.count("get") != 2 {
				t.Errorf("Get: err %v after %d reads, want a second read: the detail is stale", err, api.count("get"))
			}
		})
	}
}

func TestReviewChangeForgetsTimeline(t *testing.T) {
	reads := 0
	api := &fakeAPI{timeline: func(string) (core.Page[core.TimelineItem], error) {
		reads++
		return core.Page[core.TimelineItem]{}, nil
	}}
	s := reviewed(t, api)
	q := TimelineQuery{Repo: repo, Number: 1}
	for range 2 {
		if _, err := s.Timeline(t.Context(), q); err != nil {
			t.Fatalf("Timeline: %v", err)
		}
	}
	if reads != 1 {
		t.Fatalf("%d reads before the change, want 1: a fresh page is a hit", reads)
	}
	if err := s.RequestReviews(repo, 1, []Reviewer{{Login: "cy"}}).Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if _, err := s.Timeline(t.Context(), q); err != nil || reads != 2 {
		t.Errorf("Timeline: err %v after %d reads, want a second read: a review_requested event appears", err, reads)
	}
}

func TestRequestReviewsAlreadyPending(t *testing.T) {
	api := &fakeAPI{}
	s := reviewed(t, api)
	before := reviewersOf(t, s)

	err := s.RequestReviews(repo, 1, []Reviewer{{Login: "ANA"}, {Team: "Octo/Core"}}).Do(t.Context())
	if !errors.Is(err, ErrAlreadyRequested) {
		t.Fatalf("Do = %v, want ErrAlreadyRequested", err)
	}
	if n := api.reviewCalls(); n != 0 {
		t.Errorf("%d calls, want 0", n)
	}
	if got := reviewersOf(t, s); !reflect.DeepEqual(got, before) {
		t.Errorf("reviewers = %+v, want %+v", got, before)
	}

	// A mixed list drops the pending ones and keeps the total right.
	if err := s.RequestReviews(repo, 1, []Reviewer{{Login: "ana"}, {Login: "cy"}}).Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if want := []reviewCall{{1, []string{"cy"}, nil}}; !reflect.DeepEqual(api.requested, want) {
		t.Errorf("sent %+v, want %+v", api.requested, want)
	}
}

func TestRequestReviewsMixedListTotal(t *testing.T) {
	api := &fakeAPI{}
	s := reviewed(t, api)
	s.RequestReviews(repo, 1, []Reviewer{{Login: "ana"}, {Login: "cy"}})
	if got := reviewersOf(t, s); len(got.Requested) != 4 || got.RequestedTotal != 4 {
		t.Errorf("reviewers = %+v, want one request and one in the total added", got)
	}
}

func TestRemoveReviewRequestsNotPending(t *testing.T) {
	api := &fakeAPI{}
	s := reviewed(t, api)
	before := reviewersOf(t, s)

	// bo reviewed, and cy was never asked.
	for _, who := range []string{"bo", "cy"} {
		err := s.RemoveReviewRequests(repo, 1, []Reviewer{{Login: who}}).Do(t.Context())
		if !errors.Is(err, ErrNotPending) {
			t.Errorf("remove %s: Do = %v, want ErrNotPending", who, err)
		}
	}
	if n := api.reviewCalls(); n != 0 {
		t.Errorf("%d calls, want 0", n)
	}
	if got := reviewersOf(t, s); !reflect.DeepEqual(got, before) {
		t.Errorf("reviewers = %+v, want %+v", got, before)
	}
}

func TestRemoveReviewRequestsOfATeam(t *testing.T) {
	api := &fakeAPI{}
	s := reviewed(t, api)
	snapshot := reviewersOf(t, s)

	op := s.RemoveReviewRequests(repo, 1, []Reviewer{{Team: "octo/core"}})
	got := reviewersOf(t, s)
	want := []core.ReviewRequest{{User: core.User{Login: "ana"}}, {Unknown: true}}
	if !reflect.DeepEqual(got.Requested, want) || got.RequestedTotal != 2 {
		t.Errorf("reviewers = %+v, want only the team gone from %+v, and the total 2", got, want)
	}
	if len(snapshot.Requested) != 3 || snapshot.Requested[1].Team != "octo/core" {
		t.Errorf("the snapshot is now %+v: the change modified it in place", snapshot)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if want := []reviewCall{{1, nil, []string{"core"}}}; !reflect.DeepEqual(api.removed, want) {
		t.Errorf("sent %+v, want %+v: the slug without the organization", api.removed, want)
	}
}

func TestRemoveReviewRequestsFailureRollsBack(t *testing.T) {
	boom := errors.New("boom")
	api := &fakeAPI{}
	api.reviewErr = boom
	s := reviewed(t, api)
	before := reviewersOf(t, s)

	if err := s.RemoveReviewRequests(repo, 1, []Reviewer{{Login: "ana"}}).Do(t.Context()); !errors.Is(err, boom) {
		t.Fatalf("Do = %v, want boom", err)
	}
	if got := reviewersOf(t, s); !reflect.DeepEqual(got, before) {
		t.Errorf("reviewers = %+v, want the old %+v", got, before)
	}
}

func TestRemoveReviewRequestsTruncatedList(t *testing.T) {
	api := &fakeAPI{}
	api.get = func(_ context.Context, _ core.RepoRef, number int) (core.PullRequestDetail, error) {
		d := askedOf(number)
		d.Reviewers.RequestedTotal = 9
		return d, nil
	}
	s := New(api)
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	// The list holds 3 of 9, so a miss proves nothing.
	if err := s.RemoveReviewRequests(repo, 1, []Reviewer{{Login: "zed"}}).Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if want := []reviewCall{{1, []string{"zed"}, nil}}; !reflect.DeepEqual(api.removed, want) {
		t.Errorf("sent %+v, want %+v", api.removed, want)
	}
}

// readLands stores the old requests as a read that GitHub answered before the
// change does.
func readLands(s *Service) {
	s.details.Set(detailKey(repo, 1), cache.Entry[core.PullRequestDetail]{Value: askedOf(1), FetchedAt: time.Now()})
}

func TestReviewChangeSurvivesAReadInFlight(t *testing.T) {
	t.Run("request", func(t *testing.T) {
		s := reviewed(t, &fakeAPI{})
		op := s.RequestReviews(repo, 1, []Reviewer{{Login: "cy"}})
		readLands(s)
		if err := op.Do(t.Context()); err != nil {
			t.Fatalf("Do: %v", err)
		}
		got := reviewersOf(t, s)
		if want := (core.ReviewRequest{User: core.User{Login: "cy"}}); !slices.Contains(got.Requested, want) || got.RequestedTotal != 4 {
			t.Errorf("reviewers = %+v, want cy listed and the total 4", got)
		}
	})
	t.Run("withdraw", func(t *testing.T) {
		s := reviewed(t, &fakeAPI{})
		op := s.RemoveReviewRequests(repo, 1, []Reviewer{{Login: "ana"}})
		readLands(s)
		if err := op.Do(t.Context()); err != nil {
			t.Fatalf("Do: %v", err)
		}
		got := reviewersOf(t, s)
		if pending(got.Requested, Reviewer{Login: "ana"}) || got.RequestedTotal != 2 {
			t.Errorf("reviewers = %+v, want ana gone and the total 2", got)
		}
	})
	t.Run("applied twice", func(t *testing.T) {
		s := reviewed(t, &fakeAPI{})
		if err := s.RequestReviews(repo, 1, []Reviewer{{Login: "cy"}}).Do(t.Context()); err != nil {
			t.Fatalf("Do: %v", err)
		}
		if got := reviewersOf(t, s); len(got.Requested) != 4 || got.RequestedTotal != 4 {
			t.Errorf("reviewers = %+v, want cy once", got)
		}
	})
}

// A read that wrote the entry meanwhile is newer than the snapshot, so a
// failed change leaves the entry stale instead of restoring it.
func TestReviewChangeFailureAfterAReadInFlight(t *testing.T) {
	api := &fakeAPI{}
	api.reviewErr = errors.New("boom")
	s := reviewed(t, api)
	op := s.RequestReviews(repo, 1, []Reviewer{{Login: "cy"}})
	readLands(s)
	if err := op.Do(t.Context()); err == nil {
		t.Fatal("Do succeeded, want the error")
	}
	if _, err := s.Get(t.Context(), repo, 1); err != nil || api.count("get") != 2 {
		t.Errorf("Get: err %v after %d reads, want a second read: the entry is stale", err, api.count("get"))
	}
}

func TestReviewChangesOfNobody(t *testing.T) {
	api := &fakeAPI{}
	s := reviewed(t, api)
	before := reviewersOf(t, s)
	for name, who := range map[string][]Reviewer{"empty": nil, "zero values": {{}, {}}} {
		for op, do := range map[string]func() error{
			"request":  func() error { return s.RequestReviews(repo, 1, who).Do(t.Context()) },
			"withdraw": func() error { return s.RemoveReviewRequests(repo, 1, who).Do(t.Context()) },
		} {
			if err := do(); err != nil {
				t.Errorf("%s %s: Do = %v, want nil", op, name, err)
			}
		}
	}
	if n := api.reviewCalls(); n != 0 {
		t.Errorf("%d calls, want 0", n)
	}
	if got := reviewersOf(t, s); !reflect.DeepEqual(got, before) {
		t.Errorf("reviewers = %+v, want %+v", got, before)
	}
}

func TestRequestReviewsRepeatedReviewer(t *testing.T) {
	api := &fakeAPI{}
	s := reviewed(t, api)
	op := s.RequestReviews(repo, 1, []Reviewer{{Login: "cy"}, {Login: "CY"}, {}, {Team: "octo/docs", Name: "Docs"}, {Team: "Octo/Docs"}})
	got := reviewersOf(t, s)
	if len(got.Requested) != 5 || got.RequestedTotal != 5 {
		t.Errorf("reviewers = %+v, want cy and the team once", got)
	}
	if want := (core.ReviewRequest{Team: "octo/docs", TeamName: "Docs"}); got.Requested[4] != want {
		t.Errorf("team request = %+v, want %+v: the chip shows the name", got.Requested[4], want)
	}
	if err := op.Do(t.Context()); err != nil {
		t.Fatalf("Do: %v", err)
	}
	if want := []reviewCall{{1, []string{"cy"}, []string{"docs"}}}; !reflect.DeepEqual(api.requested, want) {
		t.Errorf("sent %+v, want %+v", api.requested, want)
	}
}

func TestRequestReviewsTruncatedListTotal(t *testing.T) {
	api := &fakeAPI{}
	api.get = func(_ context.Context, _ core.RepoRef, number int) (core.PullRequestDetail, error) {
		d := askedOf(number)
		d.Reviewers.RequestedTotal = 9
		return d, nil
	}
	s := New(api)
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	// The list holds 3 of 9, so zed may be among the others: the total stays.
	s.RequestReviews(repo, 1, []Reviewer{{Login: "zed"}})
	if got := reviewersOf(t, s); len(got.Requested) != 4 || got.RequestedTotal != 9 {
		t.Errorf("reviewers = %+v, want zed listed and the total still 9", got)
	}
}

func TestReviewChangesWithoutDetail(t *testing.T) {
	api := &fakeAPI{}
	s := New(api)
	if err := s.RequestReviews(repo, 4, []Reviewer{{Login: "cy"}}).Do(t.Context()); err != nil {
		t.Fatalf("request: %v", err)
	}
	if err := s.RemoveReviewRequests(repo, 4, []Reviewer{{Login: "cy"}}).Do(t.Context()); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if len(api.requested) != 1 || len(api.removed) != 1 {
		t.Errorf("requested %+v, removed %+v, want one call each", api.requested, api.removed)
	}
}

func TestReviewChangesRefused(t *testing.T) {
	api := &fakeAPI{}
	s := reviewed(t, api)
	s.access = tokenWith(classic("gist"), true)
	before := reviewersOf(t, s)

	for name, op := range map[string]func() error{
		"request":  func() error { return s.RequestReviews(repo, 1, []Reviewer{{Login: "cy"}}).Do(t.Context()) },
		"withdraw": func() error { return s.RemoveReviewRequests(repo, 1, []Reviewer{{Login: "ana"}}).Do(t.Context()) },
	} {
		err := op()
		if _, ok := errors.AsType[*core.ScopeError](err); !ok {
			t.Errorf("%s: Do = %v, want a ScopeError", name, err)
		}
	}
	if n := api.reviewCalls(); n != 0 {
		t.Errorf("%d calls, want 0", n)
	}
	if got := reviewersOf(t, s); !reflect.DeepEqual(got, before) {
		t.Errorf("reviewers = %+v, want %+v", got, before)
	}
}

var reviewerTeamsQ = ReviewerTeamsQuery{Repo: repo, Text: "co"}

func TestReviewerTeamsCached(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{}
		api.teams = func(_ context.Context, org, text string, first int) ([]core.Team, error) {
			if org != "eggzec" || text != "co" || first != 20 {
				t.Errorf("read %q, %q, %d", org, text, first)
			}
			return []core.Team{{Name: "Core", Slug: "core"}}, nil
		}
		s := New(api, WithTTL(time.Minute), WithAccess(tokenWith(classic("read:org"), true)))
		if _, ok := s.CachedReviewerTeams(reviewerTeamsQ); ok {
			t.Error("cached before the read")
		}
		for range 2 {
			got, err := s.ReviewerTeams(t.Context(), reviewerTeamsQ)
			if err != nil || got.Unavailable || !reflect.DeepEqual(got.Teams, []core.Team{{Name: "Core", Slug: "core"}}) {
				t.Fatalf("ReviewerTeams = %+v, %v; want Core", got, err)
			}
		}
		if api.teamReads != 1 {
			t.Errorf("reads = %d, want 1: the second is served from the cache", api.teamReads)
		}
		if c, ok := s.CachedReviewerTeams(ReviewerTeamsQuery{Repo: core.RepoRef{Owner: "EGGZEC", Name: "x"}, Text: "co"}); !ok || len(c.Teams) != 1 {
			t.Errorf("CachedReviewerTeams = %+v, %v; want the teams, whatever the owner's case", c, ok)
		}
		if _, ok := s.CachedReviewerTeams(ReviewerTeamsQuery{Repo: repo, Text: "do"}); ok {
			t.Error("another text is served from the cache")
		}
	})
}

func TestReviewerTeamsWithoutScope(t *testing.T) {
	api := &fakeAPI{}
	api.teams = func(context.Context, string, string, int) ([]core.Team, error) {
		t.Error("the teams were read without read:org")
		return nil, nil
	}
	s := New(api, WithAccess(tokenWith(classic("repo"), true)))
	got, err := s.ReviewerTeams(t.Context(), reviewerTeamsQ)
	if err != nil || !got.Unavailable || len(got.Teams) != 0 || api.teamReads != 0 {
		t.Errorf("ReviewerTeams = %+v, %v after %d reads; want Unavailable and no read", got, err, api.teamReads)
	}
}

func TestReviewerTeamsRefusedAndMissing(t *testing.T) {
	for name, tc := range map[string]struct {
		err         error
		unavailable bool
	}{
		"forbidden": {fmt.Errorf("members only: %w", core.ErrForbidden), true},
		"not found": {fmt.Errorf("no organization: %w", core.ErrNotFound), false},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				api := &fakeAPI{}
				api.teams = func(context.Context, string, string, int) ([]core.Team, error) { return nil, tc.err }
				s := New(api, WithTTL(time.Minute))
				for range 2 {
					got, err := s.ReviewerTeams(t.Context(), reviewerTeamsQ)
					if err != nil || got.Unavailable != tc.unavailable || len(got.Teams) != 0 {
						t.Fatalf("ReviewerTeams = %+v, %v; want Unavailable %v, no teams and no error", got, err, tc.unavailable)
					}
				}
				if api.teamReads != 1 {
					t.Errorf("reads = %d, want 1: the answer is cached", api.teamReads)
				}
			})
		})
	}
}

// A token whose scopes the access service doesn't know is sent, and GitHub's
// refusal of its scopes leaves the teams out instead of failing the read.
func TestReviewerTeamsScopeRefusedByGitHub(t *testing.T) {
	api := &fakeAPI{}
	api.teams = func(context.Context, string, string, int) ([]core.Team, error) {
		return nil, fmt.Errorf("list teams of octo: %w", &core.ScopeError{Scopes: []string{"read:org"}})
	}
	s := New(api, WithAccess(tokenWith(core.Access{Kind: core.TokenClassic}, true)))
	got, err := s.ReviewerTeams(t.Context(), reviewerTeamsQ)
	if err != nil || !got.Unavailable || len(got.Teams) != 0 || api.teamReads != 1 {
		t.Errorf("ReviewerTeams = %+v, %v after %d reads; want Unavailable, no error and one read", got, err, api.teamReads)
	}
}

func TestReviewerTeamsError(t *testing.T) {
	boom := errors.New("boom")
	api := &fakeAPI{}
	api.teams = func(context.Context, string, string, int) ([]core.Team, error) { return nil, boom }
	if _, err := New(api).ReviewerTeams(t.Context(), reviewerTeamsQ); !errors.Is(err, boom) {
		t.Errorf("err = %v, want boom", err)
	}
}
