package search

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/service/refs"
)

type fakeIssuePreviews struct {
	r   core.References
	m   []core.Reference
	err error
}

func (f fakeIssuePreviews) References(context.Context, refs.Query) (core.References, error) {
	return f.r, f.err
}

func (f fakeIssuePreviews) Mentions(context.Context, refs.MentionsQuery) (core.Page[core.Reference], error) {
	return core.Page[core.Reference]{Items: f.m}, f.err
}

type fakePullPreviews struct {
	d   core.PullRequestDetail
	c   core.PullCommits
	err error
}

func (f fakePullPreviews) Get(context.Context, core.RepoRef, int) (core.PullRequestDetail, error) {
	return f.d, f.err
}

func (f fakePullPreviews) PullCommits(context.Context, pulls.PullCommitsQuery) (core.PullCommits, error) {
	return f.c, f.err
}

func TestPreviewReaderDispatchesIssuesAndPulls(t *testing.T) {
	t.Parallel()
	read := NewPreviewReader(PreviewServices{Issues: fakeIssuePreviews{}, Pulls: fakePullPreviews{}})
	for _, k := range []core.SearchKind{core.SearchIssues, core.SearchPulls} {
		if read(core.SearchHit{Kind: k}) == nil {
			t.Errorf("kind %v has no reader, want one", k)
		}
	}
	none := NewPreviewReader(PreviewServices{})
	for _, k := range []core.SearchKind{core.SearchIssues, core.SearchPulls} {
		if none(core.SearchHit{Kind: k}) != nil {
			t.Errorf("kind %v has a reader without a service, want nil", k)
		}
	}
}

func TestIssueExtraListsEachPullOnce(t *testing.T) {
	t.Parallel()
	repo := core.RepoRef{Owner: "o", Name: "r"}
	pr := core.Reference{Target: core.Target{Repo: repo, Number: 7, Kind: core.KindPull}, Title: "Fix it", State: "open"}
	got := issueExtra(repo, core.References{Closing: []core.Reference{pr}}, []core.Reference{pr})
	if strings.Count(got, "Fix it") != 1 {
		t.Errorf("issueExtra lists the pull request not once:\n%s", got)
	}
	if got := issueExtra(repo, core.References{}, nil); !strings.Contains(got, "none") {
		t.Errorf("issueExtra with no references = %q, want none", got)
	}
}

func TestPullExtraHoldsBranchesAndCommits(t *testing.T) {
	t.Parallel()
	var d core.PullRequestDetail
	d.HeadRef, d.BaseRef = "feat", "main"
	d.Additions, d.Deletions, d.ChangedFiles = 5, 2, 3
	c := core.PullCommits{Commits: []core.PullCommit{{OID: "0123456789", Headline: "Do it"}}}
	got := pullExtra(d, c, "…")
	for _, want := range []string{"`feat` -> `main`", "+5 -2 in 3 files", "`0123456` Do it", "Review status: none"} {
		if !strings.Contains(got, want) {
			t.Errorf("pullExtra lacks %q:\n%s", want, got)
		}
	}
}

func TestIssueReaderFailsWithTheServiceError(t *testing.T) {
	t.Parallel()
	read := issueReader(fakeIssuePreviews{err: errors.New("boom")}, core.SearchHit{Kind: core.SearchIssues})
	if _, err := read(context.Background()); err == nil || err.Error() != "boom" {
		t.Errorf("read error = %v, want boom", err)
	}
}
