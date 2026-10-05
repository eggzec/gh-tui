package dashboard

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/service/pulls"
	"github.com/eggzec/gh-tui/internal/tui/threads"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// screen is the view with styles removed.
func screen(s *Section) string { return ansi.Strip(s.View()) }

func TestInitLoadsEverything(t *testing.T) {
	svc, in := newFake(), &fakeInbox{threads: inboxThreads()}
	s := newSection(t, svc, in, 140, 38)
	view := screen(s)
	for _, want := range []string{
		"Mona Lisa Octocat @octocat", "1.2k followers", "3 unread",
		ui.NewIcons(config.IconsNerd).Here, "spoon-knife", "Yours", "github", "charmbracelet",
		"repo-000", "Review requests 3", "bubbletea#1402", "Your pull requests 2",
		"Render only the cells that changed", "contributions in the last 90 days",
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

func TestContributionsRange(t *testing.T) {
	c := contributions()
	tests := []struct {
		days int
		want string
	}{
		{0, commas(c.Total) + " contributions in the last year"},
		{90, commas(recent(c, 90)) + " contributions in the last 90 days"},
		{30, commas(recent(c, 30)) + " contributions in the last 30 days"},
	}
	for _, tt := range tests {
		s := newSection(t, newFake(), nil, 140, 38, WithContributions(tt.days))
		if view := screen(s); !strings.Contains(view, tt.want) {
			t.Errorf("range %d: the calendar doesn't say %q:\n%s", tt.days, tt.want, view)
		}
	}
}

// recent sums the counts of the last days days of c.
func recent(c core.Contributions, days int) int {
	var all []core.ContributionDay
	for _, w := range c.Weeks {
		all = append(all, w...)
	}
	n := 0
	for _, d := range all[max(len(all)-days, 0):] {
		n += d.Count
	}
	return n
}

// commas formats n as the calendar does, as in 1,208.
func commas(n int) string {
	s := strconv.Itoa(n)
	if len(s) > 3 {
		s = s[:len(s)-3] + "," + s[len(s)-3:]
	}
	return s
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
	for _, tt := range []struct {
		name    string
		limited bool
		status  string
	}{
		{"offline", false, "offline · showing the last visit"},
		{"rate limited", true, "rate limited · showing the last visit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testServedEarlier(t, tt.limited, tt.status)
		})
	}
}

// testServedEarlier checks what the dashboard says when its reads are
// served what was read earlier, offline or rate limited: the profile says
// so, in the words of ui.SayKept, and no toast does, since the status bar
// tells the connection.
func testServedEarlier(t *testing.T, limited bool, status string) {
	t.Helper()
	svc := newFake()
	svc.offline, svc.limited = !limited, limited
	s := New(t.Context(), svc, nil)
	s.SetSize(140, 38)
	s.Focus()
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
			t.Errorf("toast %q, want none", msg.Text)
		case loadedMsg:
			queue = append(queue, s.Update(msg))
		}
	}
	if !strings.Contains(screen(s), status) {
		t.Errorf("the profile should say %q:\n%s", status, screen(s))
	}

	if limited {
		// A rate limit lifts by itself, with no answer to wait for.
		return
	}
	// Once GitHub answers again, what was served earlier is read again,
	// and the profile no longer says it.
	svc.mu.Lock()
	svc.offline, svc.limited = false, false
	svc.mu.Unlock()
	run(t, s, s.Update(ui.OnlineMsg{}))
	if strings.Contains(screen(s), "showing the last visit") {
		t.Errorf("the profile still shows the last visit once online:\n%s", screen(s))
	}
}

// Each pane says why its read failed the way the user should read it:
// the cause alone, since the pane's title says what failed, without the
// error's chain, request or status code. The open key opens the row under
// the cursor, not what failed, so no hint names it.
func TestErrorsAreInline(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		text, hint string
	}{
		{"offline", fmt.Errorf("viewer header: github: GET /graphql: %w", core.ErrOffline), errMark + " Can't reach GitHub", "r to retry"},
		{"forbidden", fmt.Errorf("viewer header: github: 403 Forbidden: %w", core.ErrForbidden), errMark + " You don't have access to this", ""},
		{"not found", fmt.Errorf("viewer header: github: 404 Not Found: %w", core.ErrNotFound), errMark + " This doesn't exist or is private.", ""},
		{"internal", fmt.Errorf("viewer header: github: decode: %s", termtexttest.Hostile), errMark + " Something went wrong", "r to retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFake()
			// The contributions fail in TestViewStates, since their
			// narrow pane wraps the error.
			for _, what := range []string{"header", "work"} {
				svc.fail[what] = tt.err
			}
			in := &fakeInbox{threads: inboxThreads(), err: tt.err}
			s := newSection(t, svc, in, 140, 38)
			termtexttest.AssertClean(t, s.View(), 140)
			view := screen(s)
			// The profile, and the pinned, work and notifications panes.
			if n := strings.Count(view, tt.text); n != 4 {
				t.Errorf("the dashboard shows %q %d times, want 4:\n%s", tt.text, n, view)
			}
			if n := strings.Count(view, "to retry"); tt.hint == "" && n > 0 || tt.hint != "" && n != 4 {
				t.Errorf("the dashboard names the retry key %d times, want the hint %q:\n%s", n, tt.hint, view)
			}
			for _, leak := range []string{"github", "viewer header", "GET", "403", "404", "decode", "open on GitHub"} {
				if strings.Contains(view, leak) {
					t.Errorf("the dashboard shows %q:\n%s", leak, view)
				}
			}
		})
	}
}

// The profile's error keeps to its two lines and keeps its hint: when the
// hint needs a line of its own, the text is cut to one.
func TestProfileErrorKeepsHint(t *testing.T) {
	svc := newFake()
	svc.fail["header"] = errors.New("github: decode: unexpected EOF")
	s := newSection(t, svc, nil, 40, 24, WithVoice(logVoice(t)))
	lines := strings.Split(screen(s), "\n")
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), errMark+" Something went wrong.") || !strings.HasSuffix(strings.TrimSpace(lines[0]), "…") {
		t.Errorf("the profile's first line = %q, want the text cut to it", lines[0])
	}
	if got := strings.TrimSpace(lines[1]); got != "r to retry" {
		t.Errorf("the profile's second line = %q, want the hint", got)
	}
}

// A read that was canceled says nothing of the error, no mark and no
// cause, but its pane still offers to read again.
func TestCanceledReadIsQuiet(t *testing.T) {
	svc := newFake()
	err := fmt.Errorf("viewer work: github: POST /graphql: %w", context.Canceled)
	svc.fail["work"] = err
	s := newSection(t, svc, &fakeInbox{threads: inboxThreads(), err: err}, 140, 38)
	view := screen(s)
	for _, bad := range []string{errMark, "canceled", "context", "POST", "went wrong"} {
		if strings.Contains(view, bad) {
			t.Errorf("the dashboard shows %q for a canceled read:\n%s", bad, view)
		}
	}
	// The work and the notifications.
	if n := strings.Count(view, "r to retry"); n != 2 {
		t.Errorf("the dashboard offers to retry %d times, want 2:\n%s", n, view)
	}
}

func TestRefreshRetries(t *testing.T) {
	svc := newFake()
	svc.fail["header"] = errors.New("github: 502 Bad Gateway")
	s := newSection(t, svc, nil, 140, 38)
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
		{"tab", calendarPane},
		{"shift+tab", workPane},
		{"4", calendarPane},
		{"5", inboxPane},
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

func TestZoom(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	tests := []struct {
		// keys are pressed after the dashboard is resized to width, if
		// it is set.
		keys          []string
		width, height int
		zoom          bool
		focus         paneID
		shows, hides  []string
	}{
		{keys: []string{"z"}, zoom: true, focus: reposPane, shows: []string{"[2] Repositories", "3 Waiting on you", "repo-000"}, hides: []string{"Review requests"}},
		{keys: []string{"3"}, zoom: true, focus: workPane, shows: []string{"[3] Waiting on you", "Review requests 3"}, hides: []string{"repo-000"}},
		{keys: []string{"tab"}, zoom: true, focus: calendarPane, shows: []string{"contributions in the last 90 days"}, hides: []string{"Review requests"}},
		{keys: []string{"shift+tab"}, zoom: true, focus: workPane, shows: []string{"Review requests 3"}, hides: []string{"contributions"}},
		{width: 180, height: 44, zoom: true, focus: workPane, shows: []string{"Review requests 3"}, hides: []string{"repo-000"}},
		// The full help of the app takes rows, which leaves the dashboard
		// too short for every pane, and gives them back.
		{width: 180, height: 26, zoom: true, focus: workPane, shows: []string{"Review requests 3"}, hides: []string{"repo-000"}},
		{width: 180, height: 44, zoom: true, focus: workPane, shows: []string{"Review requests 3"}, hides: []string{"repo-000"}},
		// Where the zoom doesn't show, esc leaves it be.
		{width: 80, height: 22, keys: []string{"esc"}, zoom: true, focus: workPane, shows: []string{"Review requests 3"}, hides: []string{"repo-000"}},
		{width: 180, height: 44, zoom: true, focus: workPane, shows: []string{"Review requests 3"}, hides: []string{"repo-000"}},
		{keys: []string{"esc"}, zoom: false, focus: workPane, shows: []string{"Review requests 3", "repo-000", "contributions"}},
		{keys: []string{"esc"}, zoom: false, focus: workPane, shows: []string{"Review requests 3", "repo-000"}},
		{keys: []string{"z", "z"}, zoom: false, focus: workPane, shows: []string{"Review requests 3", "repo-000"}},
	}
	for i, tt := range tests {
		if tt.width > 0 {
			s.SetSize(tt.width, tt.height)
		}
		press(t, s, tt.keys...)
		if s.zoom != tt.zoom || s.focus != tt.focus {
			t.Errorf("step %d: zoom %v on pane %d, want %v on %d", i, s.zoom, s.focus, tt.zoom, tt.focus)
		}
		view := screen(s)
		if w := s.boxes[s.focus].w; tt.zoom && w != s.width {
			t.Errorf("step %d: the zoomed pane is %d wide, want the %d of the dashboard", i, w, s.width)
		}
		for _, want := range tt.shows {
			if !strings.Contains(view, want) {
				t.Errorf("step %d: the dashboard doesn't show %q:\n%s", i, want, view)
			}
		}
		for _, bad := range tt.hides {
			if strings.Contains(view, bad) {
				t.Errorf("step %d: the dashboard shows %q:\n%s", i, bad, view)
			}
		}
	}
}

func TestZoomOnlyWhereItShows(t *testing.T) {
	tests := []struct {
		name string
		// The dashboard is height rows high when z is pressed, then 38.
		height int
		zoom   bool
	}{
		{"wide", 38, true},
		// The full help of the app leaves the dashboard too short for
		// every pane.
		{"under the full help", 26, false},
		{"narrow", 22, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newSection(t, newFake(), nil, 140, tt.height)
			press(t, s, "z", "esc")
			s.SetSize(140, 38)
			if s.zoom || !strings.Contains(screen(s), "Review requests") {
				t.Errorf("zoom %v after z and esc; want every pane", s.zoom)
			}
			s.SetSize(140, tt.height)
			press(t, s, "z")
			s.SetSize(140, 38)
			if s.zoom != tt.zoom || strings.Contains(screen(s), "Review requests") != !tt.zoom {
				t.Errorf("zoom %v after z; want %v:\n%s", s.zoom, tt.zoom, screen(s))
			}
		})
	}
}

func TestZoomHelp(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	has := func(desc string) bool {
		return slices.ContainsFunc((ui.Hints{Layers: s.KeyLayers()}).ShortHelp(), func(b key.Binding) bool {
			return b.Enabled() && b.Help().Desc == desc
		})
	}
	if !has("zoom") || has("unzoom") {
		t.Error("the help should show the zoom key, and no way back while not zoomed")
	}
	press(t, s, "z")
	if !has("zoom") || !has("unzoom") {
		t.Error("the zoomed help should show the keys that zoom out")
	}
	s.SetSize(80, 22)
	if has("zoom") || has("unzoom") {
		t.Error("the help of a dashboard that shows one pane anyway should leave out the zoom")
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

func TestPinned(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	press(t, s, "1")
	// The repository of the current directory comes first, on its pin.
	if c, _ := s.pinned.Selected(); !c.Here || c.Repo.Description == "" {
		t.Errorf("the first card is %+v, want the repository here, with its pin", c)
	}
	if n := len(s.pinned.Items); n != 5 {
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
	s := newSection(t, newFake(), nil, 140, 38, WithHere(other, &fakeRepos{get: get}))
	c, _ := s.pinned.Selected()
	if !c.Here || c.Repo.Ref != other || c.Repo.Description != "Read for its card" {
		t.Errorf("the first card is %+v, want the repository here as read", c)
	}
	if n := len(s.pinned.Items); n != 6 {
		t.Errorf("%d cards, want the repository here and 5 pins", n)
	}
}

func TestNoHere(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38, WithHere(core.RepoRef{}, nil))
	if app := press(t, s, "."); len(app) != 0 {
		t.Errorf(". sent %v without a repository here", app)
	}
	if c, _ := s.pinned.Selected(); c.Here {
		t.Error("without a repository here, no card is marked here")
	}
}

func TestWork(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	app := press(t, s, "3", "enter")
	if !slices.Contains(app, tea.Msg(ui.OpenPullMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}, Number: 1402})) {
		t.Errorf("enter on the first review request sent %v", app)
	}
	app = press(t, s, "]", "]", "enter")
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
	press(t, s, "3", "[")
	view += screen(s)
	for _, want := range []string{"and 12 more on GitHub", "No open issues assigned to you.", "Waiting on you · 16", "Assigned issues 0"} {
		if !strings.Contains(view, want) {
			t.Errorf("the work doesn't show %q:\n%s", want, view)
		}
	}
}

func TestInbox(t *testing.T) {
	in := &fakeInbox{threads: inboxThreads()}
	s := newSection(t, newFake(), in, 140, 38)
	bubbletea := core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	app := press(t, s, "5", "enter")
	if !slices.Contains(app, tea.Msg(ui.OpenPullMsg{Repo: bubbletea, Number: 1})) {
		t.Errorf("enter on the first notification sent %v, want its pull request opened", app)
	}
	if !slices.Contains(app, tea.Msg(ui.DoneMsg{From: ui.NotificationsTitle, What: "mark read"})) || !slices.Equal(in.marks(), []string{"1"}) {
		t.Errorf("enter marked %v read and sent %v, want the thread marked read", in.marks(), app)
	}
	// The thread read leaves the unread ones, and the cursor stays on the
	// first row.
	if v := screen(s); !strings.Contains(v, "2 unread") || !strings.Contains(v, "▌ ● hello-world") {
		t.Errorf("after opening the first thread the pane shows:\n%s", v)
	}
	app = press(t, s, "down", "o")
	if !slices.Equal(app, []tea.Msg{ui.OpenMsg{URL: "https://github.com/cli/cli/pull/3"}}) {
		t.Errorf("o on the second thread sent %v, want it opened in the browser", app)
	}
	if len(in.marks()) != 1 {
		t.Error("o marked a thread read")
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
	if !strings.Contains(screen(s), "No unread notifications.") {
		t.Errorf("with nothing unread the dashboard should say so:\n%s", screen(s))
	}
	if in.lists != 1 {
		t.Errorf("the inbox was listed %d times, want once: changes come from the cache", in.lists)
	}
}

func TestRefresh(t *testing.T) {
	svc := newFake()
	s := newSection(t, svc, nil, 140, 38)
	press(t, s, "r")
	if svc.invalidated != 1 {
		t.Errorf("refresh invalidated %d times, want once", svc.invalidated)
	}
	for _, what := range []string{"header", "work", "contributions", "repos @me@"} {
		if n := svc.count(what); n != 2 {
			t.Errorf("%s read %d times, want twice", what, n)
		}
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

func TestWorkChecks(t *testing.T) {
	s := newSection(t, newFake(), nil, 140, 38)
	app := press(t, s, "3", "C")
	if !slices.Contains(app, tea.Msg(ui.OpenPullMsg{Repo: core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}, Number: 1402, Checks: true})) {
		t.Errorf("C on the first review request sent %v", app)
	}
	if app := press(t, s, "]", "]", "C"); len(app) != 0 {
		t.Errorf("C on an issue sent %v", app)
	}
}

func TestInboxOpensAsTheNotificationsDo(t *testing.T) {
	release := thread("9", "charmbracelet/glow", "v3.0.0", true, time.Minute)
	release.Subject = core.Subject{Title: "v3.0.0", Type: core.SubjectRelease, ReleaseID: 368759772, WebURL: "https://github.com/charmbracelet/glow/releases"}
	discussion := thread("8", "charmbracelet/glow", "Themes?", true, time.Hour)
	discussion.Subject = core.Subject{Title: "Themes?", Type: core.SubjectDiscussion, WebURL: "https://github.com/charmbracelet/glow/discussions"}
	in := &fakeInbox{threads: []core.Notification{release, discussion}}
	s := newSection(t, newFake(), in, 140, 38, WithOpener(threads.New(t.Context(), threads.WithMarkRead(false))))
	for i, n := range in.threads {
		keys := []string{"5", "enter"}
		if i > 0 {
			keys = []string{"down", "enter"}
		}
		got := press(t, s, keys...)
		// The same thread opens the same way from the notifications screen.
		want := run(t, s, threads.New(t.Context()).Open(n))
		if !slices.Equal(got, want) {
			t.Errorf("enter on %s sent %v, want %v", n.Subject.Type, got, want)
		}
	}
	if m := in.marks(); len(m) != 0 {
		t.Errorf("marked %v read with mark_read_on_open off", m)
	}
	if h := (ui.Hints{Layers: s.KeyLayers()}).ShortHelp(); !slices.ContainsFunc(h, func(b key.Binding) bool { return b.Help().Desc == "open" }) {
		t.Error("the help doesn't say enter opens without marking read")
	}
}

// aheadPulls records the pull requests read ahead.
type aheadPulls struct {
	mu    sync.Mutex
	reads []int
}

func (f *aheadPulls) Get(_ context.Context, _ core.RepoRef, number int) (core.PullRequestDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reads = append(f.reads, number)
	return core.PullRequestDetail{}, nil
}

func (f *aheadPulls) Comments(context.Context, pulls.CommentsQuery) (core.Page[core.Comment], error) {
	return core.Page[core.Comment]{}, nil
}

func (f *aheadPulls) CurrentGet(_ core.RepoRef, number int) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.reads, number)
}

// CurrentComments counts the comments as cached with the detail.
func (f *aheadPulls) CurrentComments(q pulls.CommentsQuery) bool {
	return f.CurrentGet(q.Repo, q.Number)
}

func (*aheadPulls) Changed(core.RepoRef, int, time.Time) {}

func (f *aheadPulls) got() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.reads)
}

// inboxPrefetch returns the default settings of reading ahead, with the
// inbox's window after threads below the cursor, read once the cursor
// rests for rest.
func inboxPrefetch(after int, rest time.Duration) config.PrefetchLayers {
	p := config.Default().Prefetch
	p.Dashboard.Inbox.Window.After, p.Dashboard.Inbox.Rest = &after, &rest
	return p
}

func TestInboxReadsAhead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ps := &aheadPulls{}
		in := &fakeInbox{threads: inboxThreads()}
		o := threads.New(t.Context(), threads.WithPulls(ps), threads.WithPrefetch(inboxPrefetch(1, 150*time.Millisecond)))
		s := newSection(t, newFake(), in, 140, 38, WithOpener(o))
		// The first two unread threads, once the inbox loads.
		if got := slices.Sorted(slices.Values(ps.got())); !slices.Equal(got, []int{1, 2}) {
			t.Errorf("read %v ahead, want the first two threads", got)
		}
		// The thread under the cursor, once the pane has the focus and
		// the cursor rests.
		press(t, s, "5", "down", "down")
		if got := ps.got(); len(got) != 3 || got[2] != 3 {
			t.Errorf("read %v, want the third thread last", got)
		}
	})
}

// TestInboxNotLoadedReadsAtOnceWhenItIs checks that an inbox that hasn't
// loaded tells the reads ahead so, and that the window it first shows once
// it loads is read at once rather than after the cursor rests.
func TestInboxNotLoadedReadsAtOnceWhenItIs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ps := &aheadPulls{}
		in := &fakeInbox{threads: inboxThreads()}
		o := threads.New(t.Context(), threads.WithPulls(ps), threads.WithPrefetch(inboxPrefetch(1, time.Hour)))
		s := newSection(t, newFake(), in, 140, 38, WithOpener(o))
		// As if the inbox hadn't loaded yet: a new list, not loaded.
		loaded := s.notes
		o.Stop()
		s.notes.ok = false
		// Nothing read so far, so that the first window has rows to read.
		ps.mu.Lock()
		ps.reads = nil
		ps.mu.Unlock()
		if cmd := s.readAhead(); cmd != nil {
			t.Fatal("an inbox that hasn't loaded read ahead")
		}
		s.notes = loaded
		start := time.Now()
		run(t, s, s.readAhead())
		if waited := time.Since(start); waited != 0 {
			t.Errorf("the inbox as it first showed was read after %v, want at once", waited)
		}
		if got := ps.got(); len(got) == 0 {
			t.Errorf("read %v, want the threads the inbox first shows", got)
		}
	})
}

// A change to the inbox while another screen is on view, such as the one
// a poll reports, reads nothing ahead, and nor does the cursor's delay
// that fires meanwhile.
func TestInboxReadsNothingAheadOffScreen(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ps := &aheadPulls{}
		in := &fakeInbox{threads: inboxThreads()}
		// The window reaches the row above the cursor, where a new thread
		// comes.
		o := threads.New(t.Context(), threads.WithPulls(ps), threads.WithPrefetch(inboxPrefetch(1, 150*time.Millisecond)))
		s := newSection(t, newFake(), in, 140, 38, WithOpener(o))
		before := len(ps.got())
		s.Blur()
		in.mu.Lock()
		in.threads = append([]core.Notification{thread("9", "cli/cli", "New", true, time.Minute)}, in.threads...)
		in.mu.Unlock()
		run(t, s, s.Update(ui.SyncMsg{Key: notifications.SyncKey}))
		if got := ps.got(); len(got) != before {
			t.Errorf("read %v ahead off screen, want nothing more", got[before:])
		}
		s.Focus()
		run(t, s, s.Update(ui.SyncMsg{Key: "other"}))
		if got := ps.got(); !slices.Contains(got, 9) {
			t.Errorf("read %v ahead back on screen, want the new thread", got)
		}
	})
}

// On an Enterprise host, a repository without a URL links to its pages.
func TestRepoURLOnHost(t *testing.T) {
	s := &Section{host: "ghe.example.com"}
	got := s.repoURL(core.Repo{Ref: core.RepoRef{Owner: "o", Name: "r"}})
	if want := "https://ghe.example.com/o/r"; got != want {
		t.Errorf("repoURL = %q, want %q", got, want)
	}
}

// The list of repositories says what went wrong the way the user should
// read it, without the error's chain, request or status code.
func TestReposErrorWords(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"offline", fmt.Errorf("list repos: github: GET /user/repos: %w", core.ErrOffline), errMark + " Can't reach GitHub · r to retry"},
		{"forbidden", fmt.Errorf("list repos: github: 403 Forbidden: %w", core.ErrForbidden), errMark + " You don't have access to this · o to open on GitHub"},
		{"internal", errors.New("list repos: github: decode: unexpected EOF"), errMark + " Something went wrong · r to retry"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newFake()
			svc.fail["repos @me@"] = tt.err
			s := newSection(t, svc, nil, 140, 38)
			if v := screen(s); !strings.Contains(v, tt.want) || strings.Contains(v, "github:") || strings.Contains(v, "403") {
				t.Errorf("screen = %q, want %q", v, tt.want)
			}
		})
	}
}
