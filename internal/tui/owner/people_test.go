package owner

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
)

// The tabs of the people and the stars of a user, and of the members and
// the teams of an organization, wide and narrow, in every icon set.
func TestViewTabs(t *testing.T) {
	tabs := []struct {
		name, login string
		keys        []string
	}{
		{"user stars", "octocat", []string{"]"}},
		{"user followers", "octocat", []string{"]", "]"}},
		{"org members", "github", []string{"]"}},
		{"org teams", "github", []string{"]", "]"}},
	}
	sizes := []struct{ w, h int }{{120, 40}, {80, 24}}
	for _, tb := range tabs {
		for _, sz := range sizes {
			for _, icons := range []string{config.IconsNerd, config.IconsUnicode, config.IconsASCII} {
				name := fmt.Sprintf("%s %dx%d %s", tb.name, sz.w, sz.h, icons)
				t.Run(name, func(t *testing.T) {
					s := newSection(t, newFake(), tb.login, sz.w, sz.h, WithIcons(ui.NewIcons(icons)))
					press(t, s, tb.keys...)
					golden.RequireEqual(t, s.View())
				})
			}
		}
	}
}

// What the tabs show on their own: the other lists of a user, an
// organization seen from outside, an organization that requires SSO, and
// lists that are empty or failed.
func TestViewTabStates(t *testing.T) {
	tests := []struct {
		name  string
		login string
		setup func(f *fakeService)
		keys  []string
	}{
		{"following", "octocat", nil, []string{"]", "]", "]"}},
		{"organizations", "octocat", nil, []string{"["}},
		{"outsider members", "github", func(f *fakeService) {
			f.owners["github"] = outsider()
			f.people["github members"] = []core.Person{{Login: "mona", Name: "Mona Lisa"}, {Login: "hubot", Bio: "A robot"}}
		}, []string{"]"}},
		{"outsider teams", "github", func(f *fakeService) { f.owners["github"] = outsider() }, []string{"["}},
		{"sso members", "github", func(f *fakeService) {
			f.fail["people"] = &core.SSOError{URL: "https://github.com/orgs/github/sso"}
		}, []string{"]"}},
		{"no stars", "octocat", func(f *fakeService) { f.stars["octocat"] = nil }, []string{"]"}},
		{"followers failed", "octocat", func(f *fakeService) {
			f.fail["people"] = errors.New("github: decode: unexpected EOF")
		}, []string{"]", "]"}},
		{"narrow tabs", "octocat", nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFake()
			if tt.setup != nil {
				tt.setup(svc)
			}
			w, h := 120, 40
			if tt.name == "narrow tabs" {
				w, h = 60, 24
			}
			s := newSection(t, svc, tt.login, w, h)
			press(t, s, tt.keys...)
			golden.RequireEqual(t, s.View())
		})
	}
}

// ] and [ go round the tabs of the page, which differ by the kind of the
// account, and read each list once its tab is on view.
func TestSwitchTabs(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 120, 40)
	if slices.ContainsFunc(svc.calls, func(c string) bool { return c != "header octocat" && c != "repos octocat " }) {
		t.Errorf("opening read %q, want only the header and the repositories", svc.calls)
	}
	want := []tab{starsTab, followersTab, followingTab, orgsTab, reposTab}
	for _, w := range want {
		press(t, s, "]")
		if s.page.tab != w {
			t.Fatalf("] shows tab %d, want %d", s.page.tab, w)
		}
	}
	for _, c := range []string{"stars octocat ", "people octocat followers ", "people octocat following ", "people octocat orgs "} {
		if !slices.Contains(svc.calls, c) {
			t.Errorf("the tabs read %q, want %q among them", svc.calls, c)
		}
	}
	press(t, s, "[")
	if s.page.tab != orgsTab {
		t.Errorf("[ from the repositories shows tab %d, want the organizations", s.page.tab)
	}

	run(t, s, s.Update(ui.OwnerMsg{Login: "github"}))
	for _, w := range []tab{membersTab, teamsTab, reposTab} {
		press(t, s, "]")
		if s.page.tab != w {
			t.Fatalf("] on an organization shows tab %d, want %d", s.page.tab, w)
		}
	}
}

// ] and [ switch tabs only where the list pane has the focus; elsewhere
// they do nothing, and tab still moves between the panes.
func TestTabKeysOnPinned(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 120, 40)
	press(t, s, "1", "]")
	if s.page.focus != pinnedPane || s.page.tab != reposTab {
		t.Errorf("] on the pins focused pane %d on tab %d, want the pins on the repositories", s.page.focus, s.page.tab)
	}
	if b, _, ok := uitest.Winner(s.KeyLayers(), "]"); ok && b.Enabled() {
		t.Errorf("] reaches %q on the pins, want nothing", b.Help().Desc)
	}
	press(t, s, "tab")
	if s.page.focus != listPane || s.page.tab != reposTab {
		t.Errorf("tab on the pins focused pane %d on tab %d, want the list on the repositories", s.page.focus, s.page.tab)
	}
}

// A tab gone back to reads its list again only if it went stale.
func TestSwitchBackReadsStale(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 120, 40)
	press(t, s, "]", "[")
	svc.calls = nil
	press(t, s, "]")
	if len(svc.calls) != 0 {
		t.Errorf("a fresh tab read %q", svc.calls)
	}
	press(t, s, "[")
	delete(svc.read, "stars octocat")
	press(t, s, "]")
	if !slices.Contains(svc.calls, "stars octocat ") {
		t.Errorf("a stale tab read %q, want the stars again", svc.calls)
	}
}

// enter opens a person or an organization on its page, a starred
// repository on the repository screen, and a team in the browser; o
// opens each in the browser.
func TestEnterOnTabs(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 120, 40)
	repo, ok := has[ui.RepoMsg](press(t, s, "]", "down", "enter"))
	if want := (core.RepoRef{Owner: "cli", Name: "cli"}); !ok || repo.Repo != want {
		t.Errorf("enter on a star sent %v, want %v", repo, want)
	}
	owner, ok := has[ui.OwnerMsg](press(t, s, "]", "down", "enter"))
	if !ok || owner.Login != "person-01" {
		t.Errorf("enter on a follower sent %v, want person-01's page", owner)
	}
	if sel, ok := s.Selected(); !ok || sel.What != "user" || sel.URL != "https://github.com/person-01" {
		t.Errorf("selected %+v, want the follower", sel)
	}
	open, ok := has[ui.OpenMsg](press(t, s, "o"))
	if !ok || open.URL != "https://github.com/person-01" {
		t.Errorf("o on a follower sent %v, want the profile", open)
	}
	owner, ok = has[ui.OwnerMsg](press(t, s, "]", "]", "enter"))
	if !ok || owner.Login != "github" {
		t.Errorf("enter on an organization sent %v, want github's page", owner)
	}

	run(t, s, s.Update(ui.OwnerMsg{Login: "github"}))
	if msgs := press(t, s, "]", "]", "down", "enter"); len(msgs) > 0 {
		t.Errorf("enter on a team sent %v, want nothing: a team has no page here", msgs)
	}
	if s.keys.state(s).Select.Enabled() {
		t.Error("enter shows as enabled on a team")
	}
	open, ok = has[ui.OpenMsg](press(t, s, "o"))
	if !ok || open.URL != "https://github.com/orgs/github/teams/security" {
		t.Errorf("o on a team sent %v, want it in the browser", open)
	}
	if sel, ok := s.Selected(); !ok || sel.What != "team" {
		t.Errorf("selected %+v, want the team", sel)
	}
}

// Someone outside an organization sees its public members without roles,
// and no teams, which aren't read.
func TestOutsider(t *testing.T) {
	svc := newFake()
	svc.owners["github"] = outsider()
	s := newSection(t, svc, "github", 120, 40)
	press(t, s, "]")
	if l := s.page.list().(*peopleList); l.roles {
		t.Error("the members show roles to someone outside the organization")
	}
	press(t, s, "]")
	if slices.ContainsFunc(svc.calls, func(c string) bool { return c == "teams github " }) {
		t.Errorf("the teams were read for someone outside: %q", svc.calls)
	}
	if _, ok := has[ui.OpenMsg](press(t, s, "enter")); ok {
		t.Error("enter opened something on the teams of an outsider")
	}
	if sel, ok := s.Selected(); ok {
		t.Errorf("selected %+v on the hidden teams, want nothing", sel)
	}
}

// GitHub refusing the teams, other than for SSO, says that only members
// see them, though the header said the viewer is a member; the teams it
// may have shown before can't be opened.
func TestTeamsRefused(t *testing.T) {
	svc := newFake()
	svc.fail["teams"] = fmt.Errorf("list teams of github: only members see the teams: %w", core.ErrForbidden)
	s := newSection(t, svc, "github", 120, 40)
	press(t, s, "]", "]")
	if v := s.View(); !strings.Contains(v, membersOnlyText("github")) {
		t.Errorf("the refused teams don't say only members see them:\n%s", v)
	}
	l := s.page.list().(*teamList)
	if _, ok := l.selection(s); ok || l.enter(s) != nil {
		t.Error("the hidden teams can be selected or opened")
	}

	svc.fail["teams"] = &core.SSOError{}
	s = newSection(t, svc, "github", 120, 40)
	press(t, s, "]", "]")
	if v := s.View(); strings.Contains(v, membersOnlyText("github")) || !strings.Contains(v, "requires SSO") {
		t.Errorf("SSO on the teams doesn't say so:\n%s", v)
	}
}

// o with nothing under the cursor, as when the list failed or is empty,
// opens the tab on GitHub.
func TestOpenTabWithoutSelection(t *testing.T) {
	svc := newFake()
	svc.fail["people"] = errors.New("boom")
	svc.fail["teams"] = errors.New("boom")
	s := newSection(t, svc, "octocat", 120, 40)
	tests := []struct {
		login string
		keys  []string
		want  string
	}{
		{"octocat", []string{"]", "]"}, "https://github.com/octocat?tab=followers"},
		{"octocat", []string{"["}, "https://github.com/octocat"},
		{"github", []string{"]"}, "https://github.com/orgs/github/people"},
		{"github", []string{"]", "]"}, "https://github.com/orgs/github/teams"},
	}
	for _, tt := range tests {
		run(t, s, s.Update(ui.OwnerMsg{Login: tt.login}))
		s.page.tab = reposTab
		press(t, s, tt.keys...)
		msg, ok := has[ui.OpenMsg](press(t, s, "o"))
		if !ok || msg.URL != tt.want {
			t.Errorf("o on %s after %q sent %v, want %s", tt.login, tt.keys, msg, tt.want)
		}
	}
}

// The members say they are public ones once a new header says the viewer
// left the organization.
func TestMembersEmptyFollowsRoles(t *testing.T) {
	svc := newFake()
	svc.people["github members"] = nil
	s := newSection(t, svc, "github", 120, 40)
	press(t, s, "]")
	l := s.page.list().(*peopleList)
	if got := l.Feed.EmptyText(); got != "github has no members." {
		t.Errorf("a member reads %q", got)
	}
	svc.owners["github"] = outsider()
	press(t, s, "r")
	if got := l.Feed.EmptyText(); l.roles || got != "github has no public members." {
		t.Errorf("an outsider reads %q, roles %v", got, l.roles)
	}
}

// Before the header says whose the page is, the tab of people goes by no
// title, since an organization has no followers.
func TestPeopleTabBeforeHeader(t *testing.T) {
	s := New(t.Context(), newFake(), config.Default().Keys, WithDefaultTab(config.OwnerTabPeople))
	s.SetSize(80, 24)
	s.Update(ui.OwnerMsg{Login: "github"})
	s.Focus()
	if v := s.View(); strings.Contains(v, "Followers") || !strings.Contains(v, "Loading…") {
		t.Errorf("the page before its header:\n%s", v)
	}
}

// A list of people reads its next page as the cursor nears its end.
func TestPeoplePages(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 120, 40)
	press(t, s, "]", "]")
	for range 35 {
		press(t, s, "down")
	}
	if !slices.Contains(svc.calls, "people octocat followers 30") {
		t.Errorf("scrolling read %q, want the second page of followers", svc.calls)
	}
	if p, ok := s.page.list().(*peopleList).Feed.Selected(); !ok || p.Login != "person-35" {
		t.Errorf("the cursor is on %+v, want person-35", p)
	}
}

// The filter and sort are of the repositories alone.
func TestFilterOnlyOnRepositories(t *testing.T) {
	s := newSection(t, newFake(), "octocat", 120, 40)
	if _, ok := s.Filter(); !ok {
		t.Error("the repositories have no filter")
	}
	press(t, s, "]")
	if _, ok := s.Filter(); ok {
		t.Error("the stars have the repositories' filter")
	}
}

// The people tab of owner.default_tab is a user's followers and an
// organization's members.
func TestDefaultTabPeople(t *testing.T) {
	for login, want := range map[string]tab{"octocat": followersTab, "github": membersTab} {
		s := newSection(t, newFake(), login, 120, 40, WithDefaultTab(config.OwnerTabPeople))
		if s.page.tab != want {
			t.Errorf("%s opens on tab %d, want %d", login, s.page.tab, want)
		}
	}
}

// r reads the tab on view again, and the others once they are on view.
func TestRefreshTab(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, "octocat", 120, 40)
	press(t, s, "]", "]")
	svc.calls = nil
	press(t, s, "r")
	if !slices.Contains(svc.calls, "people octocat followers ") || slices.Contains(svc.calls, "stars octocat ") {
		t.Errorf("refresh read %q, want the followers alone of the lists", svc.calls)
	}
	press(t, s, "[")
	if !slices.Contains(svc.calls, "stars octocat ") {
		t.Errorf("the stars weren't read again after the refresh: %q", svc.calls)
	}
}
