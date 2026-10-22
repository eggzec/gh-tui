package search

import (
	"strings"
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestIssueTextHoldsTitleStateAuthorLabelsBody(t *testing.T) {
	t.Parallel()
	h := core.SearchHit{Issue: core.Issue{
		Title:  "Crash on start",
		Author: core.User{Login: "ana"},
		Labels: []core.Label{{Name: "bug"}, {Name: "ui"}},
		Body:   "It crashes.",
	}}
	got := issueText(h)
	for _, want := range []string{"Crash on start", "Author: ana", "Labels: bug, ui", "It crashes."} {
		if !strings.Contains(got, want) {
			t.Errorf("issueText lacks %q:\n%s", want, got)
		}
	}
}

func TestCodeTextHoldsPathAndFragments(t *testing.T) {
	t.Parallel()
	h := core.CodeHit{Path: "a/b.go", Fragments: []core.Fragment{{Text: "func main() {}"}}}
	got := codeText(h)
	if !strings.Contains(got, "a/b.go") || !strings.Contains(got, "func main() {}") {
		t.Errorf("codeText = %q", got)
	}
}

func TestRepoTextHoldsWhatTheHitHolds(t *testing.T) {
	t.Parallel()
	got := repoText(core.Repo{Description: "A tea timer", Language: "Go", Stars: 42})
	for _, want := range []string{"A tea timer", "Language: Go", "Stars: 42"} {
		if !strings.Contains(got, want) {
			t.Errorf("repoText lacks %q:\n%s", want, got)
		}
	}
}

func TestPreviewClearsWhenNothingIsSelected(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 140, 40)
	typeText(t, s, "tea")
	if !strings.Contains(s.View(), "Stars:") {
		t.Fatalf("the preview does not show the repository under the cursor:\n%s", s.View())
	}
	typeText(t, s, "zzzq")
	if got := s.View(); strings.Contains(got, "Stars:") || strings.Contains(got, "Language:") {
		t.Errorf("the preview kept the last hit with no result selected:\n%s", got)
	}
}

func TestPreviewFollowsTheSelection(t *testing.T) {
	t.Parallel()
	s := newSection(t, newFake(), 140, 40)
	typeText(t, s, "tea")
	press(t, s, "esc")
	if !strings.Contains(s.View(), "Stars:") {
		t.Errorf("the preview does not show the repository under the cursor:\n%s", s.View())
	}
	before := s.pvKey
	press(t, s, "j")
	if s.pvKey == before {
		t.Errorf("the preview kept %q after the cursor moved", before)
	}
}
