package issues

import (
	"testing"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

func TestSelected(t *testing.T) {
	if _, ok := newSection(t, newFakeService(sampleIssues(3)), 80, 20).Selected(); ok {
		t.Error("a section without a repository selected something")
	}
	h := started(t, newFakeService(sampleIssues(3)), 80, 20)
	it, _ := h.list.Selected()
	got, ok := h.Selected()
	want := ui.Selection{What: "issue", URL: it.URL, Repo: testRepo, Number: it.Number, Owner: it.Author.Login}
	if !ok || got != want || it.Number == 0 || it.Author.Login == "" {
		t.Errorf("Selected() = %+v, %v, want %+v", got, ok, want)
	}
}
