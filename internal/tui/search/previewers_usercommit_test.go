package search

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

type fakeUserPreviews struct {
	p   core.UserProfile
	err error
}

func (f fakeUserPreviews) Profile(context.Context, string) (core.UserProfile, error) {
	return f.p, f.err
}

type fakeCommitPreviews struct {
	p   core.CommitPreview
	err error
}

func (f fakeCommitPreviews) Commit(context.Context, core.RepoRef, string) (core.CommitPreview, error) {
	return f.p, f.err
}

func TestPreviewReaderDispatchesUsersAndCommits(t *testing.T) {
	t.Parallel()
	read := NewPreviewReader(PreviewServices{Users: fakeUserPreviews{}, Commits: fakeCommitPreviews{}})
	for _, k := range []core.SearchKind{core.SearchUsers, core.SearchCommits} {
		if read(core.SearchHit{Kind: k}) == nil {
			t.Errorf("kind %v has no reader, want one", k)
		}
	}
	if read(core.SearchHit{Kind: core.SearchTopics}) != nil {
		t.Error("a topic hit has a reader, want it hit-only")
	}
	none := NewPreviewReader(PreviewServices{})
	for _, k := range []core.SearchKind{core.SearchUsers, core.SearchCommits} {
		if none(core.SearchHit{Kind: k}) != nil {
			t.Errorf("kind %v has a reader without a service, want nil", k)
		}
	}
}

func TestUserExtraHoldsNameBioFollowersAndJoined(t *testing.T) {
	t.Parallel()
	got := userExtra(core.UserProfile{Name: "Tea Man", Bio: "Brews daily", Followers: 42, CreatedAt: time.Date(2015, 6, 7, 0, 0, 0, 0, time.UTC)})
	for _, want := range []string{"Name: Tea Man", "Followers: 42", "Joined: 2015-06-07", "Brews daily"} {
		if !strings.Contains(got, want) {
			t.Errorf("userExtra lacks %q:\n%s", want, got)
		}
	}
}

func TestCommitExtraHoldsFilesAndPatch(t *testing.T) {
	t.Parallel()
	p := core.CommitPreview{
		Detail: core.CommitDetail{
			Commit: core.Commit{Body: "More words.", Committer: core.Signature{Name: "Bo", Date: time.Date(2026, 1, 2, 3, 4, 0, 0, time.UTC)}},
			Stats:  core.CommitStats{Additions: 5, Deletions: 2},
			Files:  []core.CommitFile{{Path: "a.go", Additions: 5, Deletions: 2}},
		},
		Patch: "@@ -1 +1 @@\n-old\n+new\n```\n",
	}
	got := commitExtra(p, "…")
	for _, want := range []string{"Committer: Bo (2026-01-02 03:04)", "+5 -2 in 1 files", "More words.", "`a.go` +5 -2", "````diff\n@@ -1 +1 @@", "+new"} {
		if !strings.Contains(got, want) {
			t.Errorf("commitExtra lacks %q:\n%s", want, got)
		}
	}
	p.Patch, p.PatchTooLarge = "", true
	got = commitExtra(p, "…")
	if !strings.Contains(got, "Patch too large") || !strings.Contains(got, "`a.go` +5 -2") || strings.Contains(got, "diff") {
		t.Errorf("commitExtra over the cap = %q, want the stats and the note, no patch", got)
	}
}

// kindSection returns a wide page on kind with the query tea submitted.
func kindSection(t *testing.T, f *fakeService, kind core.SearchKind, opts ...Option) *Section {
	t.Helper()
	p := config.Default().Prefetch
	p.Search.Preview = 0
	s := newSection(t, f, 140, 40, append(opts, WithPrefetch(p))...)
	press(t, s, "esc")
	press(t, s, strconv.Itoa(slices.Index(kinds, kind)+1))
	typeText(t, s, "tea")
	return s
}

func TestUserPreviewExtrasLandUnderTheHit(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.users = []core.SearchUser{{Login: "teaman", Kind: core.OwnerUser}}
	svc := fakeUserPreviews{p: core.UserProfile{Name: "Tea Man", Followers: 42, Bio: "Brews daily"}}
	s := kindSection(t, f, core.SearchUsers, WithPreviewReader(NewPreviewReader(PreviewServices{Users: svc})))
	got := s.View()
	for _, want := range []string{"Type: User", "Followers: 42", "Brews daily"} {
		if !strings.Contains(got, want) {
			t.Errorf("the preview lacks %q:\n%s", want, got)
		}
	}
}

func TestUserPreviewShowsTheReadFailing(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.users = []core.SearchUser{{Login: "teaman", Kind: core.OwnerUser}}
	svc := fakeUserPreviews{err: errors.New("boom")}
	s := kindSection(t, f, core.SearchUsers, WithPreviewReader(NewPreviewReader(PreviewServices{Users: svc})))
	if got := s.View(); !strings.Contains(got, "Could not load more: boom") || !strings.Contains(got, "Type: User") {
		t.Errorf("a failed profile read shows:\n%s", got)
	}
}

func TestCommitPreviewExtrasLandUnderTheHit(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.commits = []core.SearchCommit{{Repo: core.RepoRef{Owner: "acme", Name: "tea"}, SHA: "0123456789abcdef", Subject: "tea brewing fix", Author: "ana"}}
	svc := fakeCommitPreviews{p: core.CommitPreview{
		Detail: core.CommitDetail{Stats: core.CommitStats{Additions: 3, Deletions: 1}, Files: []core.CommitFile{{Path: "brew.go", Additions: 3, Deletions: 1}}},
		Patch:  "@@ -1 +1 @@\n-cold\n+hot\n",
	}}
	s := kindSection(t, f, core.SearchCommits, WithPreviewReader(NewPreviewReader(PreviewServices{Commits: svc})))
	got := s.View()
	for _, want := range []string{"tea brewing fix", "+3 -1 in 1 files", "brew.go", "+hot"} {
		if !strings.Contains(got, want) {
			t.Errorf("the preview lacks %q:\n%s", want, got)
		}
	}
}

func TestCommitPreviewShowsTheReadFailing(t *testing.T) {
	t.Parallel()
	f := newFake()
	f.commits = []core.SearchCommit{{Repo: core.RepoRef{Owner: "acme", Name: "tea"}, SHA: "0123456789abcdef", Subject: "tea brewing fix", Author: "ana"}}
	svc := fakeCommitPreviews{err: errors.New("boom")}
	s := kindSection(t, f, core.SearchCommits, WithPreviewReader(NewPreviewReader(PreviewServices{Commits: svc})))
	if got := s.View(); !strings.Contains(got, "Could not load more: boom") || !strings.Contains(got, "tea brewing fix") {
		t.Errorf("a failed commit read shows:\n%s", got)
	}
}
