package issues

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	issuesvc "github.com/eggzec/gh-tui/internal/service/issues"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Alt+f lists without the filter and brings it back, f edits the filter that
// is off, applying or F ends the switch, and a list with no filter refuses.
func TestFilterToggle(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 20)

	if _, ok := has[ui.NotifyMsg](press(t, h, "alt+f")); !ok || h.toggle.Off() {
		t.Errorf("alt+f with no filter: off %v; want a toast and no switch", h.toggle.Off())
	}

	apply(t, h, "is:open label:bug")
	filtered := issuesvc.ListQuery{Repo: testRepo, State: core.FilterOpen, Filter: "label:bug"}
	press(t, h, "alt+f")
	if want := (issuesvc.ListQuery{Repo: testRepo, State: core.FilterOpen}); lastList(svc) != want || !h.toggle.Off() {
		t.Errorf("alt+f listed %+v, off %v; want %+v with the filter off", lastList(svc), h.toggle.Off(), want)
	}
	if chips := ansi.Strip(h.Chips()); !strings.Contains(chips, "bug") || !strings.Contains(chips, "off") {
		t.Errorf("chips %q, want the kept filter with off", chips)
	}

	if f, _ := h.Filter(); f.Query != "is:open label:bug" {
		t.Errorf("the form opens on %q while off, want the kept filter", f.Query)
	}

	press(t, h, "alt+f")
	if lastList(svc) != filtered || h.toggle.Off() || h.Chips() != "bug" {
		t.Errorf("alt+f again listed %+v, chips %q, off %v; want %+v on", lastList(svc), h.Chips(), h.toggle.Off(), filtered)
	}

	press(t, h, "alt+f", "F")
	if want := (issuesvc.ListQuery{Repo: testRepo, State: core.FilterOpen}); lastList(svc) != want || h.toggle.Off() || h.Chips() != "" {
		t.Errorf("F while off listed %+v, chips %q, off %v; want it cleared and on", lastList(svc), h.Chips(), h.toggle.Off())
	}
	if _, ok := has[ui.NotifyMsg](press(t, h, "alt+f")); !ok {
		t.Error("alt+f after F said nothing")
	}

	apply(t, h, "is:open label:bug")
	press(t, h, "alt+f")
	apply(t, h, "is:open label:bug")
	if h.toggle.Off() || h.Chips() != "bug" {
		t.Errorf("applying while off left off %v, chips %q; want the filter on", h.toggle.Off(), h.Chips())
	}
}

// Alt+s lists without the sort token and brings it back, keeping the filter,
// and a list with no sort refuses.
func TestSortToggle(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 20)

	if _, ok := has[ui.NotifyMsg](press(t, h, "alt+s")); !ok || h.sortToggle.Off() {
		t.Errorf("alt+s with no sort: off %v; want a toast and no switch", h.sortToggle.Off())
	}

	apply(t, h, "is:open label:bug sort:comments-asc")
	sorted := issuesvc.ListQuery{Repo: testRepo, State: core.FilterOpen, Filter: "label:bug sort:comments-asc"}
	press(t, h, "alt+s")
	if want := (issuesvc.ListQuery{Repo: testRepo, State: core.FilterOpen, Filter: "label:bug"}); lastList(svc) != want || !h.sortToggle.Off() {
		t.Errorf("alt+s listed %+v, off %v; want %+v with the sort off", lastList(svc), h.sortToggle.Off(), want)
	}
	if chips := ansi.Strip(h.Chips()); !strings.Contains(chips, "bug") || !strings.Contains(chips, "sort") || !strings.Contains(chips, "off") {
		t.Errorf("chips %q, want the filter and the sort with off", chips)
	}

	press(t, h, "alt+s")
	if lastList(svc) != sorted || h.sortToggle.Off() || strings.Contains(ansi.Strip(h.Chips()), "off") {
		t.Errorf("alt+s again listed %+v, chips %q, off %v; want %+v on", lastList(svc), h.Chips(), h.sortToggle.Off(), sorted)
	}

	apply(t, h, "is:open label:bug")
	if _, ok := has[ui.NotifyMsg](press(t, h, "alt+s")); !ok || h.sortToggle.Off() {
		t.Errorf("alt+s with a filter and no sort: off %v; want a toast and no switch", h.sortToggle.Off())
	}
}

// A repository opens with no filter, so a filter that was switched off in
// the one before doesn't come with it.
func TestFilterToggleEndsWithTheRepository(t *testing.T) {
	svc := newFakeService(sampleIssues(12))
	h := started(t, svc, 80, 20)
	apply(t, h, "is:open label:bug")
	press(t, h, "alt+f")
	run(t, h, h.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}}))
	if h.toggle.Off() || strings.Contains(ansi.Strip(h.Chips()), "off") {
		t.Errorf("the next repository is off with chips %q", ansi.Strip(h.Chips()))
	}
	if f, _ := h.Filter(); f.Query != "is:open" {
		t.Errorf("the form opens on %q, want the repository's own filter", f.Query)
	}
}
