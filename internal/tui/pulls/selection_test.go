package pulls

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestSelected(t *testing.T) {
	if _, ok := newTest(t, newFakeService(), 80, 20).Selected(); ok {
		t.Error("a section without a repository selected something")
	}
	h := started(t, newFakeService(), 80, 20)
	pr, _ := h.feed.Selected()
	got, ok := h.Selected()
	want := ui.Selection{What: "pull request", URL: pr.URL, Repo: repo, Number: pr.Number}
	if !ok || got != want || pr.URL == "" {
		t.Errorf("Selected() = %+v, %v, want %+v", got, ok, want)
	}
}
