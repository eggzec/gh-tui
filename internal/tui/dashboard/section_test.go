package dashboard

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// screen is the view with styles removed.
func screen(s *Section) string { return ansi.Strip(s.View()) }

func TestInitLoadsEverything(t *testing.T) {
	svc, in := newFake(), &fakeInbox{threads: inboxThreads()}
	s := newSection(t, svc, in, 140, 38)
	view := screen(s)
	for _, want := range []string{
		"Mona Lisa Octocat @octocat", "1.2k followers", "3 unread",
		"⌂ here", "spoon-knife", "Yours", "github", "charmbracelet",
		"repo-000", "Review requests 3", "bubbletea#1402", "Your pull requests 2",
		"Render only the cells that changed", "contributions in the last year",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the dashboard doesn't show %q:\n%s", want, view)
		}
	}
	for _, what := range []string{"header", "work", "contributions", "repos @me@"} {
		if n := svc.count(what); n != 1 {
			t.Errorf("%s read %d times, want once", what, n)
		}
	}
	if in.lists != 1 {
		t.Errorf("the inbox was listed %d times, want once", in.lists)
	}
}

func TestCachedPaintsAtOnce(t *testing.T) {
	svc := newFake()
	svc.cached = true
	s := New(t.Context(), svc, nil)
	s.SetSize(140, 38)
	if view := screen(s); !strings.Contains(view, "Mona Lisa Octocat") || !strings.Contains(view, "Review requests 3") {
		t.Errorf("before any read the dashboard should show the cache:\n%s", view)
	}
	if len(svc.calls) != 0 {
		t.Errorf("New made reads %v, want none", svc.calls)
	}
}

func TestStaleIsReadAgain(t *testing.T) {
	svc := newFake()
	svc.stale = true
	s := newSection(t, svc, nil, 140, 38)
	if n := svc.count("header"); n != 2 {
		t.Errorf("a stale header was read %d times, want twice", n)
	}
	if s.header.value.Stale || s.updating() {
		t.Error("the fresh header should replace the stale one")
	}
}

func TestOffline(t *testing.T) {
	svc := newFake()
	svc.offline = true
	s := New(t.Context(), svc, nil)
	s.SetSize(140, 38)
	s.Focus()
	var notified bool
	queue := []tea.Cmd{s.Init()}
	for len(queue) > 0 {
		c := queue[0]
		queue = queue[1:]
		if c == nil {
			continue
		}
		switch msg := c().(type) {
		case tea.BatchMsg:
			queue = append(queue, msg...)
		case ui.NotifyMsg:
			notified = notified || msg.Text == ui.OfflineText
		case loadedMsg:
			queue = append(queue, s.Update(msg))
		}
	}
	if !notified {
		t.Error("an offline read should tell the user once")
	}
	if !strings.Contains(screen(s), "offline · showing the last visit") {
		t.Errorf("the profile should say it is offline:\n%s", screen(s))
	}
}

func TestErrorsAreInline(t *testing.T) {
	svc := newFake()
	svc.fail["header"] = errors.New("github: 502 Bad Gateway")
	svc.fail["work"] = errors.New("github: 502 Bad Gateway")
	s := newSection(t, svc, nil, 140, 38)
	view := screen(s)
	for _, want := range []string{"Couldn't load your profile: github: 502 Bad Gateway · r retries", "Couldn't load your work"} {
		if !strings.Contains(view, want) {
			t.Errorf("the dashboard doesn't show %q:\n%s", want, view)
		}
	}

	// A refresh tries again.
	clear(svc.fail)
	press(t, s, "r")
	if !strings.Contains(screen(s), "Mona Lisa Octocat") {
		t.Errorf("a refresh should load the profile:\n%s", screen(s))
	}
}

func TestPaneFocus(t *testing.T) {
	s := newSection(t, newFake(), nil, 80, 22)
	tests := []struct {
		key  string
		want paneID
	}{
		{"tab", workPane},
		{"tab", inboxPane},
		{"shift+tab", workPane},
		{"5", calendarPane},
		{"tab", pinnedPane},
		{"1", pinnedPane},
		{"2", reposPane},
	}
	// The repositories are focused first.
	if s.focus != reposPane {
		t.Fatalf("focus = %d, want the repositories", s.focus)
	}
	for _, tt := range tests {
		press(t, s, tt.key)
		if s.focus != tt.want {
			t.Errorf("after %s the focus is %d, want %d", tt.key, s.focus, tt.want)
		}
	}
}

func TestNarrowShowsTheFocusedPane(t *testing.T) {
	s := newSection(t, newFake(), nil, 80, 22)
	if view := screen(s); !strings.Contains(view, "[2] Repositories") || !strings.Contains(view, "3 Work") || strings.Contains(view, "Review requests") {
		t.Errorf("the narrow dashboard should show the repositories and name the other panes:\n%s", view)
	}
	press(t, s, "3")
	if view := screen(s); !strings.Contains(view, "Review requests 3") || strings.Contains(view, "repo-000") {
		t.Errorf("focusing the work should show it alone:\n%s", view)
	}
}

func TestOwnerTabs(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, nil, 140, 38)
	press(t, s, "]")
	if got := s.repos.current().label; got != "github" {
		t.Fatalf("after ] the tab is %q, want github", got)
	}
	if svc.count("repos github@") != 1 || !strings.Contains(screen(s), "Repository number 0 of github") {
		t.Errorf("the tab of an organization should list its repositories:\n%s", screen(s))
	}
	press(t, s, "]", "]")
	if got := s.repos.current().label; got != yours {
		t.Errorf("the tabs should wrap around: after ]]] the tab is %q, want %s", got, yours)
	}
	press(t, s, "[", "[")
	if got := s.repos.current().label; got != "github" {
		t.Errorf("after [[ the tab is %q, want github", got)
	}
	// Each tab reads its first page once.
	if n := svc.count("repos github@"); n != 1 {
		t.Errorf("the first page of github was read %d times, want once", n)
	}
}

func TestOpenRepository(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	app := press(t, s, "down", "enter")
	want := ui.RepoMsg{Repo: core.RepoRef{Owner: "octocat", Name: "repo-001"}}
	if !slices.Contains(app, tea.Msg(want)) {
		t.Errorf("enter sent %v, want %v", app, want)
	}
	app = press(t, s, "o")
	if !slices.Contains(app, tea.Msg(ui.OpenMsg{URL: "https://github.com/octocat/repo-001"})) {
		t.Errorf("o sent %v, want the repository in the browser", app)
	}
}

func TestFilter(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, nil, 140, 38)
	press(t, s, "f")
	if !s.Capturing() {
		t.Fatal("the filter should take every key")
	}
	// The filter reads the pages the list hasn't, up to the last.
	for _, page := range []string{"repos @me@100", "repos @me@200"} {
		if n := svc.count(page); n != 1 {
			t.Errorf("%s read %d times, want once", page, n)
		}
	}
	if got := s.repos.picker.Len(); got != 250 {
		t.Errorf("the filter lists %d repositories, want every one of 250", got)
	}
	if !strings.Contains(screen(s), "250 repositories") {
		t.Errorf("the tabs should count what the filter holds:\n%s", screen(s))
	}
	typeText(t, s, "repo-249")
	it, ok := s.repos.picker.Selected()
	if !ok || it.Title != "repo-249" {
		t.Fatalf("the best match is %q, want repo-249", it.Title)
	}
	app := press(t, s, "enter")
	if !slices.Contains(app, tea.Msg(ui.RepoMsg{Repo: core.RepoRef{Owner: "octocat", Name: "repo-249"}})) {
		t.Errorf("enter sent %v, want repo-249 opened", app)
	}
	if s.Capturing() {
		t.Error("choosing a repository should close the filter")
	}

	// Opening it again reads nothing more, and esc goes back to the list.
	calls := len(svc.calls)
	press(t, s, "f")
	if len(svc.calls) != calls {
		t.Errorf("the filter read %v again", svc.calls[calls:])
	}
	press(t, s, "esc")
	if s.Capturing() || !s.repos.current().feed.Focused() {
		t.Error("esc should close the filter and focus the list")
	}
}

func TestFilterIsCappedAndStops(t *testing.T) {
	svc := newFake()
	svc.repos["@me"] = repos("octocat", 1500)
	s := newSection(t, svc, nil, 140, 38)
	press(t, s, "f")
	if got := len(s.repos.current().all); got != 1000 {
		t.Errorf("the filter read %d repositories, want the cap of 1000", got)
	}
	if n := svc.count("repos @me@1000"); n != 0 {
		t.Error("the filter should stop reading at the cap")
	}
}

func TestFilterFailure(t *testing.T) {
	svc := newFake()
	svc.fail["repos @me@100"] = errors.New("github: 502 Bad Gateway")
	s := newSection(t, svc, nil, 140, 38)
	app := press(t, s, "f")
	if len(app) != 1 || !strings.Contains(app[0].(ui.NotifyMsg).Text, "Couldn't read every repository of yours") {
		t.Errorf("a failed page should say so in a toast, got %v", app)
	}
	if got := s.repos.picker.Len(); got != 100 {
		t.Errorf("the filter lists %d repositories, want the first page of 100", got)
	}
}

func TestPinned(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	press(t, s, "1")
	// The repository of the current directory comes first, on its pin.
	if c, _ := s.pinned.selected(); !c.here || c.repo.Description == "" {
		t.Errorf("the first card is %+v, want the repository here, with its pin", c)
	}
	if n := len(s.pinned.items); n != 5 {
		t.Errorf("%d cards, want the 5 pins with hello-world first", n)
	}
	app := press(t, s, "right", "enter")
	if !slices.Contains(app, tea.Msg(ui.RepoMsg{Repo: core.RepoRef{Owner: "octocat", Name: "spoon-knife"}})) {
		t.Errorf("enter on the second card sent %v", app)
	}
	app = press(t, s, "o")
	if !slices.Contains(app, tea.Msg(ui.OpenMsg{URL: "https://github.com/octocat/spoon-knife"})) {
		t.Errorf("o on the second card sent %v", app)
	}
	// . opens the repository here from any pane.
	for _, pane := range []string{"1", "3", "5"} {
		app = press(t, s, pane, ".")
		if !slices.Contains(app, tea.Msg(ui.RepoMsg{Repo: here})) {
			t.Errorf("on pane %s . sent %v, want the repository here", pane, app)
		}
	}
}

func TestHereWithoutPin(t *testing.T) {
	other := core.RepoRef{Owner: "someone", Name: "elsewhere"}
	get := func(_ context.Context, r core.RepoRef) (core.Repo, error) {
		return core.Repo{Ref: r, Description: "Read for its card", Stars: 9}, nil
	}
	s := newSection(t, newFake(), nil, 140, 38, WithHere(other, get))
	c, _ := s.pinned.selected()
	if !c.here || c.repo.Ref != other || c.repo.Description != "Read for its card" {
		t.Errorf("the first card is %+v, want the repository here as read", c)
	}
	if n := len(s.pinned.items); n != 6 {
		t.Errorf("%d cards, want the repository here and 5 pins", n)
	}
}

func TestNoHere(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38, WithHere(core.RepoRef{}, nil))
	if app := press(t, s, "."); len(app) != 0 {
		t.Errorf(". sent %v without a repository here", app)
	}
	if c, _ := s.pinned.selected(); c.here {
		t.Error("without a repository here, no card is marked here")
	}
}

func TestWork(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	app := press(t, s, "3", "enter")
	if !slices.Contains(app, tea.Msg(ui.OpenPullMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}, Number: 1402})) {
		t.Errorf("enter on the first review request sent %v", app)
	}
	app = press(t, s, "down", "down", "down", "down", "down", "enter")
	if !slices.Contains(app, tea.Msg(ui.OpenIssueMsg{Repo: here, Number: 40})) {
		t.Errorf("enter on the assigned issue sent %v", app)
	}
	app = press(t, s, "o")
	if !slices.Contains(app, tea.Msg(ui.OpenMsg{URL: "https://github.com/octocat/hello-world/issues/40"})) {
		t.Errorf("o sent %v", app)
	}
}

func TestWorkMore(t *testing.T) {
	svc := newFake()
	svc.work.ReviewRequested.Count = 14
	svc.work.Assigned = core.WorkList{}
	s := newSection(t, svc, nil, 140, 50)
	view := screen(s)
	for _, want := range []string{"and 12 more on GitHub", "No open issue is assigned to you.", "Waiting on you · 16"} {
		if !strings.Contains(view, want) {
			t.Errorf("the work doesn't show %q:\n%s", want, view)
		}
	}
}

func TestInbox(t *testing.T) {
	in := &fakeInbox{threads: inboxThreads()}
	s := newSection(t, newFake(), in, 140, 38)
	app := press(t, s, "4", "enter")
	if !slices.Contains(app, tea.Msg(ui.ShowMsg{Title: ui.NotificationsTitle})) {
		t.Errorf("enter on the notifications sent %v", app)
	}

	// A poll that finds a change updates the count from the cache.
	in.set(inboxThreads()[0])
	s.Update(ui.SyncMsg{Key: notifications.SyncKey})
	if !strings.Contains(screen(s), "1 unread") {
		t.Errorf("after the poll the dashboard should count 1 unread:\n%s", screen(s))
	}
	// So does marking threads read in the notifications.
	in.set()
	s.Update(ui.DoneMsg{From: ui.NotificationsTitle, What: "mark all read"})
	if !strings.Contains(screen(s), "All caught up") {
		t.Errorf("with nothing unread the dashboard should say so:\n%s", screen(s))
	}
	if in.lists != 1 {
		t.Errorf("the inbox was listed %d times, want once: changes come from the cache", in.lists)
	}
}

func TestRefresh(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, nil, 140, 38)
	press(t, s, "f")
	press(t, s, "esc", "r")
	if svc.invalidated != 1 {
		t.Errorf("refresh invalidated %d times, want once", svc.invalidated)
	}
	for _, what := range []string{"header", "work", "contributions", "repos @me@"} {
		if n := svc.count(what); n != 2 {
			t.Errorf("%s read %d times, want twice", what, n)
		}
	}
	if o := s.repos.current(); o.all != nil || o.complete {
		t.Error("a refresh should forget what the filter read")
	}
}

func TestStaleReplyIsDropped(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	old := loadedMsg{id: s.id, gen: s.gen, kind: kindHeader, value: core.Header{Profile: core.Profile{Login: "old"}}}
	press(t, s, "r")
	s.Update(old)
	if s.header.value.Profile.Login != "octocat" {
		t.Error("a reply to a read before the refresh should be dropped")
	}
}
