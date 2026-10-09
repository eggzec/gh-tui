package pulls

import (
	"context"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/cache/cachetest"
	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

// richDetail is the detail of #1 with what the Overview shows.
func richDetail(v *versioned) core.PullRequestDetail {
	return core.PullRequestDetail{
		PullRequest:     v.pull(),
		FailingChecks:   []core.FailingCheck{{Name: "test", Run: true, ID: 7, Required: true, Reason: "want a frame"}},
		ChecksTruncated: true,
		Merge: core.MergeInfo{
			Mergeable:      core.MergeableYes,
			Status:         core.MergeBlocked,
			Methods:        []core.MergeMethod{core.MergeCommit, core.MergeSquash},
			AutoMerge:      &core.AutoMerge{Method: core.MergeSquash, EnabledBy: "monalisa", EnabledAt: epoch},
			RepoAutoMerge:  true,
			Queue:          core.MergeQueue{Enabled: true, Queued: true, Position: 2, State: "queued", EnqueuedAt: epoch},
			Rules:          core.MergeRules{Known: true, Approvals: 2, Checks: []string{"test"}, CodeOwners: true},
			RequiredChecks: core.CheckCounts{Failed: 1},
		},
		Reviewers: core.Reviewers{
			Requested:      []core.ReviewRequest{{User: core.User{Login: "hubot"}, CodeOwner: true}, {Team: "eggzec/core"}},
			RequestedTotal: 2,
			Verdicts:       []core.Verdict{{Author: core.User{Login: "monalisa"}, State: core.ReviewStateApproved, SubmittedAt: epoch}},
			VerdictsTotal:  1,
			Viewer:         core.ReviewStatePending,
		},
		Threads: core.ThreadSummary{
			Total: 3, Unresolved: 1, Resolved: 1, Outdated: 1, Truncated: true,
			Threads: []core.ReviewThread{{ID: "t1", Path: "a.go", Line: 3, Comments: 2, Author: core.User{Login: "hubot"}, At: epoch, Excerpt: "nit"}},
		},
		BaseSHA: "b", HeadRepo: "octocat/repo", CrossRepo: true, Milestone: "v0.4",
	}
}

// TestDetailKeepsMergeReviewersAndThreads checks that the detail comes
// back from the store, in a later session, as it was read.
func TestDetailKeepsMergeReviewersAndThreads(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksFailure}
	want := richDetail(v)
	api := v.api()
	api.get = func(context.Context, core.RepoRef, int) (core.PullRequestDetail, error) { return want, nil }
	store := openStore(t)

	s := New(api, WithStore(store))
	got, err := s.Get(t.Context(), repo, 1)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Get = %+v, %v; want the detail as read", got, err)
	}

	next := New(v.api(), WithStore(cachetest.Aged(store, time.Hour)))
	next.keptDetails.Warm(next.details, detailKey(repo, 1), true)
	kept, ok := next.CachedGet(repo, 1)
	if !ok || !reflect.DeepEqual(kept, want) {
		t.Errorf("kept detail = %+v, %v; want the detail as read", kept, ok)
	}
}

func TestDetailListSizes(t *testing.T) {
	d := config.Default().PageSize
	byDefault := github.DetailSizes{Threads: d.Threads, Reviewers: d.Reviewers, Rules: d.Rules}
	for _, tt := range []struct {
		name string
		opts []Option
		want github.DetailSizes
	}{
		{"default", nil, byDefault},
		{"set", []Option{WithDetailSizes(github.DetailSizes{Threads: 20, Reviewers: 15, Rules: 30})}, github.DetailSizes{Threads: 20, Reviewers: 15, Rules: 30}},
		{"capped", []Option{WithDetailSizes(github.DetailSizes{Threads: 500, Reviewers: 101, Rules: 100})}, github.DetailSizes{Threads: 100, Reviewers: 100, Rules: 100}},
		{"unset", []Option{WithDetailSizes(github.DetailSizes{Threads: -1, Reviewers: 0, Rules: 25})}, github.DetailSizes{Threads: d.Threads, Reviewers: d.Reviewers, Rules: 25}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := (&versioned{updated: epoch}).api()
			s := New(api, tt.opts...)
			if _, err := s.Get(t.Context(), repo, 1); err != nil {
				t.Fatal(err)
			}
			if got := api.detailSizes; len(got) != 1 || got[0] != tt.want {
				t.Errorf("asked for lists of %v, want [%v]", got, tt.want)
			}
		})
	}
}

// TestRevalidateReadsACurrentDetail checks that Revalidate asks GitHub for
// a detail that Get serves without a request, because it is fresh and the
// list vouches for it, while the cached one is served until the read
// answers, and that it reads once.
func TestRevalidateReadsACurrentDetail(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	api := v.api()
	queue := 1
	api.get = func(context.Context, core.RepoRef, int) (core.PullRequestDetail, error) {
		d := core.PullRequestDetail{PullRequest: v.pull()}
		d.Merge.Queue = core.MergeQueue{Queued: true, Position: queue}
		return d, nil
	}
	s := New(api)
	list(t, s, openList)
	readDetail(t, s)
	if !s.CurrentGet(repo, 1) {
		t.Fatal("CurrentGet = false after a read, want true")
	}
	readDetail(t, s)
	wantCalls(t, api, 1, 1)

	// The queue moves without the pull request updating.
	queue = 2
	if cached, _ := s.CachedGet(repo, 1); cached.Merge.Queue.Position != 1 {
		t.Fatalf("cached queue position = %d, want 1", cached.Merge.Queue.Position)
	}
	d, err := s.Revalidate(t.Context(), repo, 1)
	if err != nil || d.Merge.Queue.Position != 2 {
		t.Fatalf("Revalidate = queue %d, %v; want position 2", d.Merge.Queue.Position, err)
	}
	wantCalls(t, api, 2, 1)
	// The detail it read is current again: one read per open, no more.
	if _, err := s.Get(t.Context(), repo, 1); err != nil {
		t.Fatal(err)
	}
	wantCalls(t, api, 2, 1)
}

// TestRevalidateServesKeptDetailMeanwhile checks that a detail an earlier
// session kept is read again by Revalidate, and is served by CachedGet
// before it answers.
func TestRevalidateOfKeptDetail(t *testing.T) {
	v := &versioned{updated: epoch, checks: core.ChecksSuccess}
	store := openStore(t)
	firstSession(t, v, store)

	api := v.api()
	s := New(api, WithStore(store))
	if _, err := s.Revalidate(t.Context(), repo, 1); err != nil {
		t.Fatalf("Revalidate: %v", err)
	}
	if n := api.count("get"); n != 1 {
		t.Errorf("get called %d times, want 1", n)
	}
}

// TestUnknownMergeabilityKeepsTTL checks that a detail whose merge state
// GitHub was still working out is read again after its TTL, though the list
// still vouches for it, as GitHub settles it without updating the pull
// request.
func TestUnknownMergeabilityKeepsTTL(t *testing.T) {
	for _, tt := range []struct {
		name      string
		state     core.State
		mergeable core.Mergeable
		wantGets  int
	}{
		{"open, working it out", core.StateOpen, core.MergeableUnknown, 2},
		{"open, settled", core.StateOpen, core.MergeableYes, 1},
		// GitHub never works out the mergeability of a closed one.
		{"closed", core.StateClosed, core.MergeableUnknown, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				v := &versioned{updated: clock, checks: core.ChecksSuccess}
				api := v.api()
				api.get = func(context.Context, core.RepoRef, int) (core.PullRequestDetail, error) {
					d := core.PullRequestDetail{PullRequest: v.pull()}
					d.State, d.Merge.Mergeable = tt.state, tt.mergeable
					return d, nil
				}
				s := New(api, WithTTL(time.Minute))
				list(t, s, openFirst)
				readDetail(t, s)

				time.Sleep(2 * time.Minute)
				readDetail(t, s)
				if n := api.count("get"); n != tt.wantGets {
					t.Errorf("get called %d times, want %d", n, tt.wantGets)
				}
			})
		})
	}
}
