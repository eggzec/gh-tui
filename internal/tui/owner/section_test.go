package owner

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keytest"
)

func TestKeyMapComplete(t *testing.T) {
	keytest.Complete(t, newKeyMap(config.Default().Keys))
}

func has[T tea.Msg](msgs []tea.Msg) (T, bool) {
	for _, m := range msgs {
		if v, ok := m.(T); ok {
			return v, true
		}
	}
	var zero T
	return zero, false
}

// GoBack goes back through the pages opened one from another, keeping where
// each was; esc doesn't.
func TestBack(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 120, 40)
	press(t, s, "down", "down")
	run(t, s, s.Update(ui.OwnerMsg{Login: "github"}))
	if s.Login() != "github" {
		t.Fatalf("page of %q, want github's", s.Login())
	}
	if msgs := press(t, s, "esc"); len(msgs) > 0 || s.Login() != "github" {
		t.Fatalf("esc sent %v and shows %q, want it to do nothing", msgs, s.Login())
	}
	if !s.CanGoBack() {
		t.Fatal("CanGoBack = false after opening a second page")
	}
	run(t, s, s.GoBack())
	if s.Login() != "octocat" || s.CanGoBack() {
		t.Fatalf("GoBack shows %q, want octocat's page and none before it", s.Login())
	}
	if r, _ := s.page.repos().Feed.Selected(); r.Ref.Name != "repo-002" {
		t.Errorf("the cursor is on %s, want it where it was", r.Ref)
	}
}

// Opened while the page isn't on view, a page starts a new way back, and
// the same account keeps its page.
func TestOpenFromElsewhere(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 120, 40)
	run(t, s, s.Update(ui.OwnerMsg{Login: "github"}))
	s.Blur()
	run(t, s, s.Update(ui.OwnerMsg{Login: "octocat"}))
	s.Focus()
	if len(s.back) != 0 || s.Login() != "octocat" {
		t.Errorf("back %d pages on %q, want none on octocat's", len(s.back), s.Login())
	}
	p := s.page
	run(t, s, s.Update(ui.OwnerMsg{Login: "OctoCat"}))
	if s.page != p || len(s.back) != 0 {
		t.Error("the same account opened a new page")
	}
}

// The way back keeps maxBack pages.
func TestBackIsBounded(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 120, 40)
	for range maxBack + 5 {
		run(t, s, s.Update(ui.OwnerMsg{Login: "github"}))
		run(t, s, s.Update(ui.OwnerMsg{Login: "octocat"}))
	}
	if len(s.back) != maxBack {
		t.Errorf("back holds %d pages, want %d", len(s.back), maxBack)
	}
}

// esc leaves the zoom before it goes back.
func TestBackUnzooms(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 120, 40)
	press(t, s, "z")
	if msgs := press(t, s, "esc"); len(msgs) > 0 || s.zoom {
		t.Errorf("esc sent %v with zoom %v, want the zoom gone", msgs, s.zoom)
	}
}

func TestDefaultTab(t *testing.T) {
	for _, name := range []string{config.OwnerTabRepositories, ""} {
		s := newSection(t, newFake(), "octocat", 120, 40, WithDefaultTab(name))
		if s.page.tab != reposTab || s.page.focus != listPane {
			t.Errorf("default tab %q opens tab %d of pane %d, want the repositories", name, s.page.tab, s.page.focus)
		}
	}
	s := newSection(t, newFake(), "octocat", 80, 24, WithDefaultTab(config.OwnerTabReadme))
	if s.page.focus != readmePane || s.page.side.pager == nil {
		t.Errorf("default tab readme opens pane %d, want the README read", s.page.focus)
	}
}

func TestSelectOpensTheRepository(t *testing.T) {
	s := newSection(t, newFake(), "github", 120, 40)
	msg, ok := has[ui.RepoMsg](press(t, s, "down", "enter"))
	if want := (core.RepoRef{Owner: "github", Name: "repo-001"}); !ok || msg.Repo != want {
		t.Errorf("enter sent %v, want %v", msg, want)
	}
	msg, ok = has[ui.RepoMsg](press(t, s, "1", "right", "enter"))
	if want := (core.RepoRef{Owner: "github", Name: "gitignore"}); !ok || msg.Repo != want {
		t.Errorf("enter on a card sent %v, want %v", msg, want)
	}
	if sel, ok := s.Selected(); !ok || sel.Repo.Name != "gitignore" {
		t.Errorf("selected %+v, want the card", sel)
	}
}

// An organization's repositories are read as an organization's.
func TestReadsByKind(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "github", 120, 40)
	if s.page.repos().q.Kind != core.OwnerOrg {
		t.Errorf("the repositories are read as kind %d, want an organization's", s.page.repos().q.Kind)
	}
}

func TestRefresh(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 120, 40)
	svc.calls = nil
	press(t, s, "r")
	if !slices.Equal(svc.invalidated, []string{"octocat"}) || !slices.Contains(svc.calls, "header octocat") || !slices.Contains(svc.calls, "repos octocat ") {
		t.Errorf("refresh invalidated %q and read %q", svc.invalidated, svc.calls)
	}
}

// Coming back to the page reads again what went stale meanwhile.
func TestRevisit(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 120, 40)
	svc.calls = nil
	run(t, s, s.Revisit())
	if len(svc.calls) != 0 {
		t.Errorf("a fresh page read %q", svc.calls)
	}
	svc.read = map[string]bool{}
	run(t, s, s.Revisit())
	if !slices.Contains(svc.calls, "header octocat") || !slices.Contains(svc.calls, "repos octocat ") {
		t.Errorf("a stale page read %q, want the header and the repositories", svc.calls)
	}
}

func TestClearFilter(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 80, 24)
	run(t, s, s.ApplyFilter(filterApplied("language:rust")))
	if n := s.page.repos().Feed.Len(); n != 10 {
		t.Fatalf("the filter keeps %d repositories, want 10", n)
	}
	press(t, s, "F")
	if s.page.repos().Filter().Active() || s.page.repos().Feed.Len() != 30 {
		t.Errorf("clearing left the filter %q with %d rows", s.page.repos().Filter().Query(), s.page.repos().Feed.Len())
	}
}

func filterApplied(query string) filterform.AppliedMsg { return filterform.AppliedMsg{Query: query} }

// The help names the keys of the panes the page has: an organization has
// no calendar, so no 4.
func TestHelpNamesPanesOnView(t *testing.T) {
	for _, tt := range []struct{ login, want string }{{"octocat", "1-4"}, {"github", "1-3"}} {
		s := newSection(t, newFake(), tt.login, 120, 40)
		var got string
		for _, b := range s.KeyLayers()[0].Bindings {
			if b.Help().Desc == "focus pane" && b.Enabled() {
				got = b.Help().Key
			}
		}
		if got != tt.want {
			t.Errorf("the help of %s's page names the panes %q, want %q", tt.login, got, tt.want)
		}
	}
}
