package owner

import (
	"slices"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
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

// esc goes back through the pages opened one from another, keeping where
// each was, and then asks the app to go back.
func TestBack(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 120, 40)
	press(t, s, "down", "down")
	run(t, s, s.Update(ui.OwnerMsg{Login: "github"}))
	if s.Login() != "github" {
		t.Fatalf("page of %q, want github's", s.Login())
	}
	if b, src, ok := uitest.Winner(s.KeyLayers(), "esc"); !ok || src != "profile" || b.Help().Desc != "previous page" {
		t.Errorf("esc reaches %v %q %q, want the previous page", ok, src, b.Help().Desc)
	}
	if msgs := press(t, s, "esc"); len(msgs) > 0 || s.Login() != "octocat" {
		t.Fatalf("esc sent %v and shows %q, want octocat's page", msgs, s.Login())
	}
	if r, _ := s.page.repos.Feed.Selected(); r.Ref.Name != "repo-002" {
		t.Errorf("the cursor is on %s, want it where it was", r.Ref)
	}
	if _, ok := has[ui.BackMsg](press(t, s, "esc")); !ok {
		t.Error("esc on the first page didn't ask the app to go back")
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
	for _, name := range []string{config.OwnerTabRepositories, config.OwnerTabReadme, config.OwnerTabPeople, ""} {
		s := newSection(t, newFake(), "octocat", 120, 40, WithDefaultTab(name))
		if s.page.tab != reposTab || s.page.focus != listPane {
			t.Errorf("default tab %q opens tab %d of pane %d, want the repositories", name, s.page.tab, s.page.focus)
		}
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
	if s.page.repos.q.Kind != core.OwnerOrg {
		t.Errorf("the repositories are read as kind %d, want an organization's", s.page.repos.q.Kind)
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
	if n := s.page.repos.Feed.Len(); n != 10 {
		t.Fatalf("the filter keeps %d repositories, want 10", n)
	}
	press(t, s, "F")
	if s.page.repos.Filter().Active() || s.page.repos.Feed.Len() != 30 {
		t.Errorf("clearing left the filter %q with %d rows", s.page.repos.Filter().Query(), s.page.repos.Feed.Len())
	}
}

func filterApplied(query string) filterform.AppliedMsg { return filterform.AppliedMsg{Query: query} }
