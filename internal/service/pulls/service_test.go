package pulls

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/github"
)

var repo = core.RepoRef{Owner: "eggzec", Name: "gh-tui"}

// fakeAPI is an API whose methods are set per test. It counts the calls of
// each method.
type fakeAPI struct {
	labelsFake
	reviewersFake
	fakeLinked
	list func(ctx context.Context, repo core.RepoRef, state core.State, cursor string, first int) (core.Page[core.PullRequest], error)
	get  func(ctx context.Context, repo core.RepoRef, number int) (core.PullRequestDetail, error)
	// comments and reviews back the timeline reads. The comments answer
	// without validators, unless restComments is set, which backs
	// ListIssueComments in full.
	comments     func(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.Comment], error)
	restComments func(ctx context.Context, repo core.RepoRef, number int, cursor string, perPage int, cond github.Conditional) (core.Page[core.Comment], github.Response, error)
	reviews      func(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.Review], error)
	files        func(ctx context.Context, repo core.RepoRef, number int, cursor string, cond github.Conditional) (core.Page[core.CommitFile], github.Response, error)
	threads      func(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.ThreadPage, error)
	id           func(ctx context.Context, repo core.RepoRef, number int) (string, error)
	// timeline backs ListPullRequestTimeline, by cursor.
	timeline func(cursor string) (core.Page[core.TimelineItem], error)
	// compare and compareFiles back the two comparisons.
	compare      func(ctx context.Context, repo core.RepoRef, base, head string, cond github.Conditional) (core.Compare, github.Response, error)
	compareFiles func(ctx context.Context, repo core.RepoRef, base, head string, cond github.Conditional) (core.CompareDetail, github.Response, error)
	// behind backs BehindCommits.
	behind func(ctx context.Context, repo core.RepoRef, head, base string) (core.BaseCommits, error)
	// commits, commit and commitFiles back the commit reads.
	commits     func(ctx context.Context, repo core.RepoRef, number int, cond github.Conditional) (core.Page[core.Commit], github.Response, error)
	commit      func(ctx context.Context, repo core.RepoRef, sha string) (core.CommitDetail, error)
	commitFiles func(ctx context.Context, repo core.RepoRef, sha, cursor string) (core.Page[core.CommitFile], error)
	// mutate backs every mutation. Method names the mutation; merge also
	// passes the merge method.
	mutate func(ctx context.Context, method, id string, how core.MergeMethod) (core.PullRequest, error)
	// probe backs ProbePullRequests. Without it, every probe answers
	// without an ETag, so it vouches for nothing.
	probe func(ctx context.Context, repo core.RepoRef, cond github.Conditional) (github.Response, error)
	// codeowners and teamMembers back the owners reads.
	codeowners  func(ctx context.Context, repo core.RepoRef, base string) (core.CodeownersFile, error)
	teamMembers func(ctx context.Context, org, slug string) (core.Page[core.Person], error)
	// filter and search back the filtered lists.
	filter func(ctx context.Context, repo core.RepoRef, f github.PullFilter, cursor string, first int) (core.Page[core.PullRequest], error)
	search func(ctx context.Context, query, cursor string, first int) (core.Page[core.PullRequest], error)

	mu    sync.Mutex
	calls map[string]int
	// heads are the heads that merges were pinned to, in order.
	heads []string
	// messages are the commit messages that merges were sent with, in order.
	messages []core.MergeMessage
	// detailSizes are the list sizes that details asked for.
	detailSizes []github.DetailSizes
	// updateErr fails UpdatePullRequestBranch, and updates are the pulls and
	// heads it was sent, as "number@head".
	updateErr error
	updates   []string
	// threadSetErr fails SetReviewThreadResolved.
	threadSetErr error
	// commentErr fails CreateIssueComment.
	commentErr error
	// submitErr fails SubmitReview, and submits are what it was sent.
	submitErr error
	submits   []submitted
	// pendingReview, pendingFound and pendingErr are what PendingReview
	// answers. A pending submission is recorded in submits, with the review's
	// ID in the commit field.
	pendingReview core.PendingReview
	pendingFound  bool
	pendingErr    error
	// pullCommits and pullCommitsErr are what ListPullCommits answers, and
	// viewers the logins it was asked for. viewer and viewerErr back
	// ViewerLogin.
	pullCommits    core.PullCommits
	pullCommitsErr error
	viewers        []string
	viewer         string
	viewerErr      error
	// viewedFake backs the viewed marks.
	viewedFake
	// commentOpsFake backs the edits, replies and deletes of comments.
	commentOpsFake
}

func (f *fakeAPI) ListPullCommits(_ context.Context, _ core.RepoRef, _ int, viewer string) (core.PullCommits, error) {
	f.called("pullcommits")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.viewers = append(f.viewers, viewer)
	return f.pullCommits, f.pullCommitsErr
}

func (f *fakeAPI) ViewerLogin(context.Context) (string, error) {
	f.called("viewer")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.viewer, f.viewerErr
}

// submitted is one call of SubmitReview.
type submitted struct {
	pull, commit, body string
	event              core.ReviewEvent
	threads            []github.ReviewThreadDraft
	replies            []github.ReviewReplyDraft
}

func (f *fakeAPI) SubmitReview(_ context.Context, pullID, commit string, event core.ReviewEvent, body string, threads []github.ReviewThreadDraft, replies []github.ReviewReplyDraft) (string, error) {
	f.called("submit")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits = append(f.submits, submitted{pullID, commit, body, event, threads, replies})
	if f.submitErr != nil {
		return "", f.submitErr
	}
	return "PRR_1", nil
}

func (f *fakeAPI) PendingReview(context.Context, core.RepoRef, int) (core.PendingReview, bool, error) {
	f.called("pending")
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.pendingReview, f.pendingFound, f.pendingErr
}

func (f *fakeAPI) SubmitPendingReview(_ context.Context, pullID, reviewID string, event core.ReviewEvent, body string, threads []github.ReviewThreadDraft, replies []github.ReviewReplyDraft) error {
	f.called("submitpending")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.submits = append(f.submits, submitted{pullID, reviewID, body, event, threads, replies})
	return f.submitErr
}

func (f *fakeAPI) called(method string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = make(map[string]int)
	}
	f.calls[method]++
}

// count returns how often method was called.
func (f *fakeAPI) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

func (f *fakeAPI) ListPullRequests(ctx context.Context, repo core.RepoRef, state core.State, cursor string, first int) (core.Page[core.PullRequest], error) {
	f.called("list")
	return f.list(ctx, repo, state, cursor, first)
}

func (f *fakeAPI) FilterPullRequests(ctx context.Context, repo core.RepoRef, filter github.PullFilter, cursor string, first int) (core.Page[core.PullRequest], error) {
	f.called("filter")
	return f.filter(ctx, repo, filter, cursor, first)
}

func (f *fakeAPI) SearchPullRequests(ctx context.Context, query, cursor string, first int) (core.Page[core.PullRequest], error) {
	f.called("search")
	return f.search(ctx, query, cursor, first)
}

func (f *fakeAPI) GetPullRequest(ctx context.Context, repo core.RepoRef, number int, sizes github.DetailSizes) (core.PullRequestDetail, error) {
	f.called("get")
	f.mu.Lock()
	f.detailSizes = append(f.detailSizes, sizes)
	f.mu.Unlock()
	return f.get(ctx, repo, number)
}

func (f *fakeAPI) ListIssueComments(ctx context.Context, repo core.RepoRef, number int, cursor string, perPage int, cond github.Conditional) (core.Page[core.Comment], github.Response, error) {
	f.called("comments")
	if f.restComments != nil {
		return f.restComments(ctx, repo, number, cursor, perPage, cond)
	}
	p, err := f.comments(ctx, repo, number, cursor, perPage)
	return p, github.Response{}, err
}

func (f *fakeAPI) ListPullRequestReviews(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.Page[core.Review], error) {
	f.called("reviews")
	return f.reviews(ctx, repo, number, cursor, first)
}

func (f *fakeAPI) ListPullRequestFiles(ctx context.Context, repo core.RepoRef, number int, cursor string, cond github.Conditional) (core.Page[core.CommitFile], github.Response, error) {
	f.called("files")
	return f.files(ctx, repo, number, cursor, cond)
}

func (f *fakeAPI) ListPullRequestCommits(ctx context.Context, repo core.RepoRef, number int, cond github.Conditional) (core.Page[core.Commit], github.Response, error) {
	f.called("commits")
	return f.commits(ctx, repo, number, cond)
}

func (f *fakeAPI) GetCommit(ctx context.Context, repo core.RepoRef, sha string) (core.CommitDetail, error) {
	f.called("commit")
	return f.commit(ctx, repo, sha)
}

func (f *fakeAPI) ListCommitFiles(ctx context.Context, repo core.RepoRef, sha, cursor string) (core.Page[core.CommitFile], error) {
	f.called("commitfiles")
	return f.commitFiles(ctx, repo, sha, cursor)
}

func (f *fakeAPI) ListPullRequestThreads(ctx context.Context, repo core.RepoRef, number int, cursor string, first int) (core.ThreadPage, error) {
	f.called("threads")
	return f.threads(ctx, repo, number, cursor, first)
}

func (f *fakeAPI) Compare(ctx context.Context, repo core.RepoRef, base, head string, cond github.Conditional) (core.Compare, github.Response, error) {
	f.called("compare")
	return f.compare(ctx, repo, base, head, cond)
}

func (f *fakeAPI) CompareFiles(ctx context.Context, repo core.RepoRef, base, head string, cond github.Conditional) (core.CompareDetail, github.Response, error) {
	f.called("comparefiles")
	return f.compareFiles(ctx, repo, base, head, cond)
}

func (f *fakeAPI) BehindCommits(ctx context.Context, repo core.RepoRef, head, base string) (core.BaseCommits, error) {
	f.called("behind")
	return f.behind(ctx, repo, head, base)
}

func (f *fakeAPI) PullRequestID(ctx context.Context, repo core.RepoRef, number int) (string, error) {
	f.called("id")
	return f.id(ctx, repo, number)
}

func (f *fakeAPI) MergePullRequest(ctx context.Context, id string, method core.MergeMethod, msg core.MergeMessage, head string) (core.PullRequest, error) {
	f.called("merge")
	f.mu.Lock()
	f.heads = append(f.heads, head)
	f.messages = append(f.messages, msg)
	f.mu.Unlock()
	return f.mutate(ctx, "merge", id, method)
}

func (f *fakeAPI) AutoMergePullRequest(ctx context.Context, id string, method core.MergeMethod, msg core.MergeMessage, head string) (core.PullRequest, error) {
	f.called("automerge")
	f.mu.Lock()
	f.heads = append(f.heads, head)
	f.messages = append(f.messages, msg)
	f.mu.Unlock()
	return f.mutate(ctx, "automerge", id, method)
}

func (f *fakeAPI) StopAutoMergePullRequest(ctx context.Context, id string) (core.PullRequest, error) {
	f.called("stopautomerge")
	return f.mutate(ctx, "stopautomerge", id, "")
}

func (f *fakeAPI) EnqueuePullRequest(ctx context.Context, id, head string) (core.PullRequest, error) {
	f.called("enqueue")
	f.mu.Lock()
	f.heads = append(f.heads, head)
	f.mu.Unlock()
	return f.mutate(ctx, "enqueue", id, "")
}

func (f *fakeAPI) ClosePullRequest(ctx context.Context, id string) (core.PullRequest, error) {
	f.called("close")
	return f.mutate(ctx, "close", id, "")
}

func (f *fakeAPI) ReopenPullRequest(ctx context.Context, id string) (core.PullRequest, error) {
	f.called("reopen")
	return f.mutate(ctx, "reopen", id, "")
}

func (f *fakeAPI) MarkPullRequestReady(ctx context.Context, id string) (core.PullRequest, error) {
	f.called("ready")
	return f.mutate(ctx, "ready", id, "")
}

func (f *fakeAPI) UpdatePullRequestBranch(_ context.Context, _ core.RepoRef, number int, head string) error {
	f.called("updatebranch")
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updates = append(f.updates, fmt.Sprintf("%d@%s", number, head))
	return f.updateErr
}

func (f *fakeAPI) ConvertPullRequestToDraft(ctx context.Context, id string) (core.PullRequest, error) {
	f.called("draft")
	return f.mutate(ctx, "draft", id, "")
}

func (f *fakeAPI) SetReviewThreadResolved(_ context.Context, id string, resolved bool) error {
	f.called(fmt.Sprintf("thread %s %t", id, resolved))
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.threadSetErr
}

func (f *fakeAPI) CreateIssueComment(_ context.Context, _ core.RepoRef, number int, body string) (core.Comment, error) {
	f.called(fmt.Sprintf("comment %d", number))
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.commentErr != nil {
		return core.Comment{}, f.commentErr
	}
	return core.Comment{ID: "C_real", Body: body}, nil
}

func (f *fakeAPI) ProbePullRequests(ctx context.Context, repo core.RepoRef, cond github.Conditional) (github.Response, error) {
	f.called("probe")
	if f.probe == nil {
		return github.Response{StatusCode: 200}, nil
	}
	return f.probe(ctx, repo, cond)
}

func openPull(number int) core.PullRequest {
	return core.PullRequest{ID: fmt.Sprintf("PR_%d", number), Repo: repo, Number: number, State: core.StateOpen}
}

// listing returns pages of open pull requests: the first page holds 1 and 2
// and continues at cursor "c1", which holds 3.
func listing(_ context.Context, _ core.RepoRef, _ core.State, cursor string, _ int) (core.Page[core.PullRequest], error) {
	if cursor == "c1" {
		return core.Page[core.PullRequest]{Items: []core.PullRequest{openPull(3)}}, nil
	}
	return core.Page[core.PullRequest]{Items: []core.PullRequest{openPull(1), openPull(2)}, Next: "c1"}, nil
}

func detail(_ context.Context, _ core.RepoRef, number int) (core.PullRequestDetail, error) {
	return core.PullRequestDetail{
		PullRequest: openPull(number),
		CheckCounts: core.CheckCounts{Passed: 1},
	}, nil
}

func TestListFreshHitMakesNoCall(t *testing.T) {
	api := &fakeAPI{list: listing}
	s := New(api)
	q := ListQuery{Repo: repo, State: core.StateOpen}

	if _, ok := s.CachedList(q); ok {
		t.Error("CachedList reported a page before any fetch")
	}
	first, err := s.List(t.Context(), q)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	second, err := s.List(t.Context(), q)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if n := api.count("list"); n != 1 {
		t.Errorf("API called %d times, want 1", n)
	}
	if !reflect.DeepEqual(first, second) {
		t.Errorf("cached page = %+v, want %+v", second, first)
	}
	if got, ok := s.CachedList(q); !ok || !reflect.DeepEqual(got, first) {
		t.Errorf("Cached = %+v, %v; want the fetched page", got, ok)
	}
}

func TestListStaleRefetches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		title := "before"
		api := &fakeAPI{list: func(context.Context, core.RepoRef, core.State, string, int) (core.Page[core.PullRequest], error) {
			pr := openPull(1)
			pr.Title = title
			return core.Page[core.PullRequest]{Items: []core.PullRequest{pr}}, nil
		}}
		s := New(api, WithTTL(time.Minute))
		q := ListQuery{Repo: repo, State: core.StateOpen}
		if _, err := s.List(t.Context(), q); err != nil {
			t.Fatalf("List: %v", err)
		}

		time.Sleep(2 * time.Minute)
		title = "after"
		if got, ok := s.CachedList(q); !ok || got.Items[0].Title != "before" {
			t.Errorf("Cached = %+v, %v; want the stale page", got, ok)
		}
		got, err := s.List(t.Context(), q)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if n := api.count("list"); n != 2 {
			t.Errorf("API called %d times, want 2", n)
		}
		if got.Items[0].Title != "after" {
			t.Errorf("title = %q, want the refetched one", got.Items[0].Title)
		}
	})
}

func TestListKeysByQuery(t *testing.T) {
	type call struct {
		state  core.State
		cursor string
		first  int
	}
	var (
		mu    sync.Mutex
		calls []call
	)
	api := &fakeAPI{list: func(ctx context.Context, r core.RepoRef, state core.State, cursor string, first int) (core.Page[core.PullRequest], error) {
		mu.Lock()
		calls = append(calls, call{state, cursor, first})
		mu.Unlock()
		return listing(ctx, r, state, cursor, first)
	}}
	s := New(api)

	first, err := s.List(t.Context(), ListQuery{Repo: repo, State: core.StateOpen})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if first.Next != "c1" {
		t.Fatalf("next = %q, want c1", first.Next)
	}
	second, err := s.List(t.Context(), ListQuery{Repo: repo, State: core.StateOpen, Cursor: first.Next})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Number != 3 || !second.Last() {
		t.Errorf("second page = %+v, want the last page with #3", second)
	}
	if _, err := s.List(t.Context(), ListQuery{Repo: repo, State: core.StateClosed}); err != nil {
		t.Fatalf("List: %v", err)
	}
	// The same query with other casing is the same repository.
	upper := core.RepoRef{Owner: "EggZec", Name: "GH-TUI"}
	if _, err := s.List(t.Context(), ListQuery{Repo: upper, State: core.StateOpen}); err != nil {
		t.Fatalf("List: %v", err)
	}
	// Zero and negative sizes mean the default, and sizes above GitHub's
	// maximum are clamped to it, so each pair shares a page.
	for _, size := range []int{30, -1, 10, 10, 500, maxPageSize} {
		if _, err := s.List(t.Context(), ListQuery{Repo: repo, State: core.StateOpen, PageSize: size}); err != nil {
			t.Fatalf("List: %v", err)
		}
	}

	want := []call{
		{core.StateOpen, "", 30},
		{core.StateOpen, "c1", 30},
		{core.StateClosed, "", 30},
		{core.StateOpen, "", 10},
		{core.StateOpen, "", maxPageSize},
	}
	if !reflect.DeepEqual(calls, want) {
		t.Errorf("calls = %+v, want %+v", calls, want)
	}
}

func TestListErrorWrappedAndNotCached(t *testing.T) {
	api := &fakeAPI{list: func(context.Context, core.RepoRef, core.State, string, int) (core.Page[core.PullRequest], error) {
		return core.Page[core.PullRequest]{}, fmt.Errorf("graphql: %w", core.ErrNotFound)
	}}
	s := New(api)
	q := ListQuery{Repo: repo}

	for range 2 {
		_, err := s.List(t.Context(), q)
		if !errors.Is(err, core.ErrNotFound) {
			t.Errorf("error = %v, want ErrNotFound", err)
		}
		if err != nil && !strings.Contains(err.Error(), "list pulls of eggzec/gh-tui") {
			t.Errorf("error %q lacks context", err)
		}
	}
	if n := api.count("list"); n != 2 {
		t.Errorf("API called %d times, want 2: errors must not be cached", n)
	}
	if _, ok := s.CachedList(q); ok {
		t.Error("CachedList reported a page after a failed fetch")
	}
}

func TestGetFreshHitMakesNoCall(t *testing.T) {
	api := &fakeAPI{get: detail}
	s := New(api)

	if _, ok := s.CachedGet(repo, 1); ok {
		t.Error("CachedGet reported a detail before any fetch")
	}
	for range 2 {
		d, err := s.Get(t.Context(), repo, 1)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if d.Number != 1 || d.CheckCounts.Total() != 1 {
			t.Errorf("detail = %+v, want #1 with one check", d)
		}
	}
	if n := api.count("get"); n != 1 {
		t.Errorf("API called %d times, want 1", n)
	}
	if d, ok := s.CachedGet(repo, 1); !ok || d.Number != 1 {
		t.Errorf("CachedGet = %+v, %v; want #1", d, ok)
	}
	if _, err := s.Get(t.Context(), repo, 2); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if n := api.count("get"); n != 2 {
		t.Errorf("API called %d times, want 2: another number is another entry", n)
	}
}

func TestGetStaleRefetches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		api := &fakeAPI{get: detail}
		s := New(api, WithTTL(time.Minute))
		if _, err := s.Get(t.Context(), repo, 1); err != nil {
			t.Fatalf("Get: %v", err)
		}
		time.Sleep(2 * time.Minute)
		if _, ok := s.CachedGet(repo, 1); !ok {
			t.Error("CachedGet lost the stale detail")
		}
		if _, err := s.Get(t.Context(), repo, 1); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if n := api.count("get"); n != 2 {
			t.Errorf("API called %d times, want 2", n)
		}
	})
}

func TestGetErrorWrapped(t *testing.T) {
	api := &fakeAPI{get: func(context.Context, core.RepoRef, int) (core.PullRequestDetail, error) {
		return core.PullRequestDetail{}, &core.RateLimitError{Reset: time.Now()}
	}}
	s := New(api)

	_, err := s.Get(t.Context(), repo, 7)
	if !errors.Is(err, core.ErrRateLimited) {
		t.Errorf("error = %v, want ErrRateLimited", err)
	}
	if _, ok := errors.AsType[*core.RateLimitError](err); !ok {
		t.Errorf("error %v does not hold the RateLimitError", err)
	}
	if err != nil && !strings.Contains(err.Error(), "get pull eggzec/gh-tui#7") {
		t.Errorf("error %q lacks context", err)
	}
}

func TestWithCapacity(t *testing.T) {
	api := &fakeAPI{get: detail}
	s := New(api, WithCapacity(1))
	for _, n := range []int{1, 2} {
		if _, err := s.Get(t.Context(), repo, n); err != nil {
			t.Fatalf("Get: %v", err)
		}
	}
	if _, ok := s.CachedGet(repo, 1); ok {
		t.Error("#1 is still cached beyond the capacity of 1")
	}
}

func TestInvalidateRefetchesRepo(t *testing.T) {
	api := &fakeAPI{list: listing, get: detail}
	api.comments = func(context.Context, core.RepoRef, int, string, int) (core.Page[core.Comment], error) {
		return core.Page[core.Comment]{Items: []core.Comment{{ID: "c"}}}, nil
	}
	api.reviews = func(context.Context, core.RepoRef, int, string, int) (core.Page[core.Review], error) {
		return core.Page[core.Review]{Items: []core.Review{{ID: "r"}}}, nil
	}
	s := New(api)
	other := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	read := func(repo core.RepoRef) {
		t.Helper()
		ctx := t.Context()
		if _, err := s.List(ctx, ListQuery{Repo: repo, State: core.StateOpen}); err != nil {
			t.Fatalf("List: %v", err)
		}
		if _, err := s.Get(ctx, repo, 1); err != nil {
			t.Fatalf("Get: %v", err)
		}
		if _, err := s.Comments(ctx, CommentsQuery{Repo: repo, Number: 1}); err != nil {
			t.Fatalf("Comments: %v", err)
		}
		if _, err := s.Reviews(ctx, ReviewsQuery{Repo: repo, Number: 1}); err != nil {
			t.Fatalf("Reviews: %v", err)
		}
	}
	read(repo)
	read(other)

	s.Invalidate(core.RepoRef{Owner: "EggZec", Name: "GH-TUI"})
	if _, ok := s.CachedList(ListQuery{Repo: repo, State: core.StateOpen}); !ok {
		t.Error("list page was dropped, want it kept stale")
	}
	if _, ok := s.CachedGet(repo, 1); !ok {
		t.Error("detail was dropped, want it kept stale")
	}
	read(repo)
	read(other)
	for _, m := range []string{"list", "get", "comments", "reviews"} {
		// One call per repository before, and one more for the
		// invalidated one; the other repository stays fresh.
		if n := api.count(m); n != 3 {
			t.Errorf("%s called %d times, want 3", m, n)
		}
	}
}
