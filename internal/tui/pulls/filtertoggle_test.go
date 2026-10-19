package pulls

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// toasted reports whether msgs hold a toast.
func toasted(msgs []tea.Msg) bool {
	for _, m := range msgs {
		if _, ok := m.(ui.NotifyMsg); ok {
			return true
		}
	}
	return false
}

// lastFirst returns the last first page that was listed.
func lastFirst(svc *fakeService) pulls.ListQuery {
	got := firstLists(svc)
	return got[len(got)-1]
}

// Alt+f lists without the filter and brings it back, f edits the filter that
// is off, applying or F ends the switch, and a list with no filter refuses.
func TestFilterToggle(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 20)

	if msgs := press(t, h, "alt+f"); !toasted(msgs) || h.toggle.Off() {
		t.Errorf("alt+f with no filter: toast %v, off %v; want a toast and no switch", toasted(msgs), h.toggle.Off())
	}

	apply(t, h, "is:open author:octocat")
	filtered := pulls.ListQuery{Repo: repo, State: core.StateOpen, Filter: "author:octocat"}
	press(t, h, "alt+f")
	if want := (pulls.ListQuery{Repo: repo, State: core.StateOpen}); lastFirst(svc) != want || !h.toggle.Off() {
		t.Errorf("alt+f listed %+v, off %v; want %+v with the filter off", lastFirst(svc), h.toggle.Off(), want)
	}
	if chips := ansi.Strip(h.Chips()); !strings.Contains(chips, "@octocat") || !strings.Contains(chips, "off") {
		t.Errorf("chips %q, want the kept filter with off", chips)
	}

	if f, _ := h.Filter(); f.Query != "is:open author:octocat" {
		t.Errorf("the form opens on %q while off, want the kept filter", f.Query)
	}

	press(t, h, "alt+f")
	if lastFirst(svc) != filtered || h.toggle.Off() || h.Chips() != "@octocat" {
		t.Errorf("alt+f again listed %+v, chips %q, off %v; want %+v on", lastFirst(svc), h.Chips(), h.toggle.Off(), filtered)
	}

	press(t, h, "alt+f")
	press(t, h, "F")
	if want := (pulls.ListQuery{Repo: repo, State: core.StateOpen}); lastFirst(svc) != want || h.toggle.Off() || h.Chips() != "" {
		t.Errorf("F while off listed %+v, chips %q, off %v; want it cleared and on", lastFirst(svc), h.Chips(), h.toggle.Off())
	}
	if msgs := press(t, h, "alt+f"); !toasted(msgs) {
		t.Error("alt+f after F said nothing")
	}

	apply(t, h, "is:open label:cache")
	press(t, h, "alt+f")
	apply(t, h, "is:open label:cache")
	if h.toggle.Off() || h.Chips() != "cache" {
		t.Errorf("applying while off left off %v, chips %q; want the filter on", h.toggle.Off(), h.Chips())
	}
}

// A repository opens with no filter, so a filter that was switched off in
// the one before doesn't come with it.
func TestFilterToggleEndsWithTheRepository(t *testing.T) {
	svc := newFakeService()
	h := started(t, svc, 80, 20)
	apply(t, h, "is:open author:alice")
	press(t, h, "alt+f")
	drain(t, h, h.Update(ui.RepoMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}}))
	if h.toggle.Off() || strings.Contains(ansi.Strip(h.Chips()), "off") {
		t.Errorf("the next repository is off with chips %q", ansi.Strip(h.Chips()))
	}
	if f, _ := h.Filter(); f.Query != "is:open" {
		t.Errorf("the form opens on %q, want the repository's own filter", f.Query)
	}
}
