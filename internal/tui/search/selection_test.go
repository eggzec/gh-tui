package search

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/core"
)

func TestSelected(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, 120, 30)
	typeText(t, s, "tea")
	press(t, s, "enter")
	got, ok := s.Selected()
	if !ok || got.What != "repository" || got.Repo.Owner == "" || got.URL == "" {
		t.Errorf("repositories: Selected() = %+v, %v", got, ok)
	}
	run(t, s, s.showKind(core.SearchIssues))
	if got, ok := s.Selected(); !ok || got.What != "issue" || got.Number == 0 {
		t.Errorf("issues: Selected() = %+v, %v", got, ok)
	}
	run(t, s, s.showKind(core.SearchCode))
	run(t, s, s.searchCode())
	if got, ok := s.Selected(); !ok || got.What != "file" || got.Path == "" {
		t.Errorf("code: Selected() = %+v, %v", got, ok)
	}
}
