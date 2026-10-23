package search

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
)

type fakeRepoPreviews struct{ p core.RepoPreview }

func (f fakeRepoPreviews) Preview(context.Context, core.RepoRef) (core.RepoPreview, error) {
	return f.p, nil
}

func TestPreviewReaderDispatchesByKind(t *testing.T) {
	t.Parallel()
	read := NewPreviewReader(PreviewServices{Repos: fakeRepoPreviews{}})
	if read(core.SearchHit{Kind: core.SearchRepos}) == nil {
		t.Error("a repository hit has no reader, want one")
	}
	for _, k := range []core.SearchKind{core.SearchTopics, core.SearchUsers, core.SearchCommits} {
		if read(core.SearchHit{Kind: k}) != nil {
			t.Errorf("kind %v has a reader, want nil until it has one", k)
		}
	}
	if NewPreviewReader(PreviewServices{})(core.SearchHit{Kind: core.SearchRepos}) != nil {
		t.Error("a repository hit has a reader without a service, want nil")
	}
}

func TestRepoExtraHoldsCountsReleaseAndReadme(t *testing.T) {
	t.Parallel()
	got := repoExtra(core.RepoPreview{
		Readme: "# Tea", OpenIssues: 12, OpenPulls: 3,
		Release: core.Release{Tag: "v1.2.0", PublishedAt: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)},
	})
	for _, want := range []string{"Open issues: 12", "Open pull requests: 3", "Latest release: v1.2.0 (2026-03-04)", "# Tea"} {
		if !strings.Contains(got, want) {
			t.Errorf("repoExtra lacks %q:\n%s", want, got)
		}
	}
	if got := repoExtra(core.RepoPreview{}); strings.Contains(got, "Latest release") || strings.Contains(got, "---") {
		t.Errorf("repoExtra of a repository without a release or README shows them:\n%s", got)
	}
}

func TestRepoPreviewExtrasLandUnderTheHit(t *testing.T) {
	t.Parallel()
	svc := fakeRepoPreviews{p: core.RepoPreview{Readme: "Brew **fine** tea", OpenIssues: 12, OpenPulls: 3}}
	p := config.Default().Prefetch
	p.Search.Preview = 0
	s := newSection(t, newFake(), 140, 40, WithPreviewReader(NewPreviewReader(PreviewServices{Repos: svc})), WithPrefetch(p))
	typeText(t, s, "tea")
	got := s.View()
	for _, want := range []string{"Stars:", "Open issues: 12", "Brew"} {
		if !strings.Contains(got, want) {
			t.Errorf("the preview lacks %q:\n%s", want, got)
		}
	}
}
