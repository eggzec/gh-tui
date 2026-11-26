package tui

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/owner"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// fakeOwners knows the users and organizations in cached, in memory, and
// in remote, on GitHub, by their logins in lower case. Any other is not
// found, unless err says otherwise.
type fakeOwners struct {
	mu     sync.Mutex
	cached map[string]core.Owner
	remote map[string]core.Owner
	err    error
	gets   []string
}

func (f *fakeOwners) CachedHeader(login string) (core.Owner, bool) {
	o, ok := f.cached[strings.ToLower(login)]
	return o, ok
}

func (f *fakeOwners) Header(ctx context.Context, login string) (core.Owner, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, login)
	if err := ctx.Err(); err != nil {
		return core.Owner{}, err
	}
	if f.err != nil {
		return core.Owner{}, f.err
	}
	if o, ok := f.remote[strings.ToLower(login)]; ok {
		return o, nil
	}
	return core.Owner{}, core.ErrNotFound
}

func newFakeOwners() *fakeOwners {
	user := func(login string) core.Owner { return core.Owner{Profile: core.Profile{Login: login}} }
	org := func(login string) core.Owner {
		return core.Owner{Kind: core.OwnerOrg, Profile: core.Profile{Login: login}}
	}
	me := user("mona")
	me.Viewer.IsViewer = true
	return &fakeOwners{
		cached: map[string]core.Owner{"hubot": user("hubot")},
		remote: map[string]core.Owner{"octocat": user("octocat"), "charmbracelet": org("charmbracelet"), "mona": me},
	}
}

// fakeOwnerPage records the owner modals the app opened, in order. Its
// logins are those of the modals; the last is on top unless it was closed.
type fakeOwnerPage struct {
	mods []*fakeOwnerModal
	// sel is what the cursor of every modal opened is on, if set.
	sel *ui.Selection
}

// opener is the option that opens owner modals that the page records.
func (p *fakeOwnerPage) opener() Option {
	return WithOwnerModal(func(_ context.Context, login string) (ui.Modal, tea.Cmd) {
		mod := &fakeOwnerModal{login: login, page: p}
		mod.title = login
		p.mods = append(p.mods, mod)
		return mod, nil
	})
}

// opened returns the logins of the modals opened.
func (p *fakeOwnerPage) opened() []string {
	logins := make([]string, 0, len(p.mods))
	for _, mod := range p.mods {
		logins = append(logins, mod.login)
	}
	return logins
}

// Login returns the login of the last modal opened, or "" for none.
func (p *fakeOwnerPage) Login() string {
	if len(p.mods) == 0 {
		return ""
	}
	return p.mods[len(p.mods)-1].login
}

// ownerOn returns the owner modal on top of m, or nil.
func ownerOn(m *Model) *fakeOwnerModal {
	mod, _ := m.modals.Top().(*fakeOwnerModal)
	return mod
}

// got reports whether the last modal opened got a message that is ok.
func (p *fakeOwnerPage) got(ok func(tea.Msg) bool) bool {
	return len(p.mods) > 0 && slices.ContainsFunc(p.mods[len(p.mods)-1].msgs, ok)
}

// fakeOwnerModal is an owner page in a modal, which acts as the real one
// does: dismiss closes it, back is the manager's to take, and the owner key
// shows the owner of the cursor unless it is the page's own.
type fakeOwnerModal struct {
	fakeModal
	login string
	page  *fakeOwnerPage
}

func (f *fakeOwnerModal) Selected() (ui.Selection, bool) {
	if f.page.sel == nil {
		return ui.Selection{}, false
	}
	return *f.page.sel, true
}

// KeyLayers are those of a modal over which the command key opens the line
// and whose dismiss key reaches Act.
func (f *fakeOwnerModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{{Source: "preview", Context: "preview"}}
}

func (f *fakeOwnerModal) Act(action string) (tea.Cmd, bool) {
	switch action {
	case ui.ActQuit, ui.ActDismiss:
		return ui.CloseModal(f), true
	case "owner":
		sel, ok := f.Selected()
		if !ok || sel.Owner == "" || strings.EqualFold(sel.Owner, f.login) {
			return nil, true
		}
		return ui.ShowOwner(sel.Owner), true
	}
	return nil, false
}

// newOwnerApp returns an app on the dashboard whose goto reads owners, and
// the record of the owner modals it opens.
func newOwnerApp(t *testing.T, owners *fakeOwners, opts ...Option) (*Model, *fakeOwnerPage) {
	t.Helper()
	page := &fakeOwnerPage{}
	layout := Layout{
		Files: &fakeSection{title: "Files"}, Pulls: &fakeSection{title: "Pull requests"}, Issues: &fakeSection{title: "Issues"},
		Notifications: &fakeSection{title: "Notifications"}, Dashboard: &fakeSection{title: ui.DashboardTitle},
	}
	opts = append([]Option{WithRepos(newGotoRepos()), WithOwners(owners), page.opener()}, opts...)
	m := New(t.Context(), config.Default(), layout, opts...)
	rankFakes(m)
	m.toast.SetDuration(0)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(m, m.Init())
	return m, page
}

func TestGotoOwner(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		line  string
		login string
		err   error
		// want is the login of the page shown, or "" for none, and dash
		// reports that the dashboard shows instead.
		want  string
		dash  bool
		toast string
		// asks reports whether GitHub is asked.
		asks bool
	}{
		{name: "user", line: "goto @octocat", want: "octocat", asks: true},
		{name: "organization", line: "goto @charmbracelet", want: "charmbracelet", asks: true},
		{name: "in memory", line: "goto @hubot", want: "hubot"},
		{name: "spelled as GitHub does", line: "goto @OctoCat", want: "octocat", asks: true},
		{name: "profile link", line: "goto https://github.com/octocat?tab=repositories", want: "octocat", asks: true},
		{name: "organization link", line: "goto github.com/orgs/charmbracelet/people", want: "charmbracelet", asks: true},
		{name: "own login", line: "goto @Mona", login: "mona", dash: true},
		{name: "own login GitHub names", line: "goto @mona", dash: true, asks: true},
		{name: "not found", line: "goto @octocta", toast: "Can't open @octocta: no user or organization has that name.", asks: true},
		{name: "offline", line: "goto @octocat", err: errOffline, toast: "Couldn't open @octocat: can't reach GitHub.", asks: true},
		{name: "rate limited", line: "goto @octocat", err: core.ErrRateLimited, toast: "Couldn't open @octocat: rate limited by GitHub.", asks: true},
		{name: "bare login", line: "goto octocat", toast: "Can't open octocat: want owner/name."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owners := newFakeOwners()
			owners.err = tt.err
			var opts []Option
			if tt.login != "" {
				opts = append(opts, WithLogin(tt.login))
			}
			m, page := newOwnerApp(t, owners, opts...)
			runCommand(t, m, tt.line)
			switch {
			case tt.want != "":
				if ownerOn(m) == nil || page.Login() != tt.want {
					t.Errorf("screen %d with the page of %q, want the owner screen of %q", m.screen, page.Login(), tt.want)
				}
				if !strings.Contains(onScreen(m), "─ "+tt.want+" ─") {
					t.Errorf("the header doesn't name %s:\n%s", tt.want, onScreen(m))
				}
			case m.screen != dashScreen:
				t.Errorf("screen = %d, want the dashboard", m.screen)
			}
			if !tt.dash && tt.want == "" && len(page.opened()) > 0 {
				t.Errorf("the page was given %q", page.opened())
			}
			if tt.toast != "" && !hasToast(m, tt.toast) {
				t.Errorf("toasts lack %q: %s", tt.toast, toasted(m))
			}
			if asked := len(owners.gets) > 0; asked != tt.asks {
				t.Errorf("asked GitHub = %v, want %v", asked, tt.asks)
			}
			if m.going != nil {
				t.Error("the goto still waits")
			}
		})
	}
}

func TestGotoOwnerShowsItWaits(t *testing.T) {
	t.Parallel()
	m, page := newOwnerApp(t, newFakeOwners())
	cmd := submitLine(t, m, "goto @octocat")
	if m.going == nil {
		t.Fatal("the goto doesn't wait for GitHub")
	}
	if s := onScreen(m); !strings.Contains(s, "Opening @octocat…") {
		t.Errorf("the footer doesn't say what it opens:\n%s", s)
	}
	drive(m, cmd)
	if m.going != nil || ownerOn(m) == nil || page.Login() != "octocat" {
		t.Error("the goto didn't end on the page")
	}
}

// Another command replaces a goto that waits, and its answer is ignored.
func TestGotoOwnerIsReplaced(t *testing.T) {
	t.Parallel()
	m, page := newOwnerApp(t, newFakeOwners())
	cmd := submitLine(t, m, "goto @octocat")
	runCommand(t, m, "goto eggzec/gh-tui")
	drive(m, cmd)
	if m.screen != repoScreen || len(page.opened()) > 0 {
		t.Errorf("screen %d with pages %q, want the repository alone", m.screen, page.opened())
	}
}

// Without a reader goto opens any page named, and without a page it says
// there is none.
func TestGotoOwnerUnchecked(t *testing.T) {
	t.Parallel()
	m, page := newOwnerApp(t, nil)
	m.owners = nil
	runCommand(t, m, "goto @someone")
	if ownerOn(m) == nil || page.Login() != "someone" {
		t.Errorf("screen %d with the page of %q, want someone's", m.screen, page.Login())
	}

	m, _ = newGotoApp(t, newGotoRepos())
	runCommand(t, m, "goto @octocat")
	if want := "Can't open @octocat: there are no pages of users and organizations here."; !hasToast(m, want) {
		t.Errorf("toasts lack %q: %s", want, toasted(m))
	}
}

// An owner page opened over another replaces it, and backspace returns to
// the one it replaced.
func TestOwnerBack(t *testing.T) {
	t.Parallel()
	m, page := newOwnerApp(t, newFakeOwners())
	runCommand(t, m, "goto @octocat")
	// The fake takes the keys of a text preview, which the command line
	// refuses to open over, so the second page comes by message.
	drive(m, ui.ShowOwner("hubot"))
	if top := ownerOn(m); top == nil || top.login != "hubot" {
		t.Fatalf("on top %v, opened %q, toasts %q; want hubot's page", top, page.opened(), toasted(m))
	}
	drive(m, m.key(press("backspace")))
	if top := ownerOn(m); top == nil || top.login != "octocat" {
		t.Fatalf("on top %v with pages %q, want octocat's page again", m.modals.Top(), page.opened())
	}
}

// esc closes the owner page and shows what was under it.
func TestOwnerEscCloses(t *testing.T) {
	t.Parallel()
	m, _ := newOwnerApp(t, newFakeOwners())
	runCommand(t, m, "goto @octocat")
	if ownerOn(m) == nil {
		t.Fatal("the page didn't open")
	}
	drive(m, m.key(press("esc")))
	if m.modals.Top() != nil || m.screen != dashScreen {
		t.Errorf("on top %v, screen %d; want the dashboard alone", m.modals.Top(), m.screen)
	}
}

// ui.OwnerMsg opens the page of the login, as goto does.
func TestOwnerMsgOpensModal(t *testing.T) {
	t.Parallel()
	m, page := newOwnerApp(t, newFakeOwners())
	drive(m, ui.ShowOwner("octocat"))
	if ownerOn(m) == nil || page.Login() != "octocat" {
		t.Errorf("on top %v with pages %q, want octocat's", m.modals.Top(), page.opened())
	}
}

// From the dashboard to a repository and its owner's page, backspace goes
// back to the repository, and then to the dashboard, and stays there.
func TestBackspaceWalksBackThroughScreens(t *testing.T) {
	t.Parallel()
	m, page := newOwnerApp(t, newFakeOwners())
	runCommand(t, m, "goto eggzec/gh-tui")
	runCommand(t, m, "goto @octocat")
	if ownerOn(m) == nil || page.Login() != "octocat" {
		t.Fatalf("screen %d with the page of %q, want octocat's", m.screen, page.Login())
	}
	drive(m, m.key(press("esc")))
	if m.screen != repoScreen || m.repo != (core.RepoRef{Owner: "eggzec", Name: "gh-tui"}) {
		t.Fatalf("screen %d on %v, want the repository", m.screen, m.repo)
	}
	drive(m, m.key(press("backspace")))
	if m.screen != dashScreen {
		t.Fatalf("screen = %d, want the dashboard", m.screen)
	}
	drive(m, m.key(press("backspace")))
	if m.screen != dashScreen {
		t.Errorf("screen = %d, want backspace at the bottom to do nothing", m.screen)
	}
}

// The page takes the keys of the panes, as the dashboard does.
func TestOwnerTakesPaneKeys(t *testing.T) {
	t.Parallel()
	m, page := newOwnerApp(t, newFakeOwners())
	runCommand(t, m, "goto @hubot")
	drive(m, m.key(press("2")))
	if ownerOn(m) == nil || !page.got(isKey("2")) {
		t.Errorf("screen %d, page got 2 = %v; want the page to take it", m.screen, page.got(isKey("2")))
	}
}

func TestCompleteOwner(t *testing.T) {
	t.Parallel()
	recall := newFakeRecall()
	recall.owners = []string{"charmbracelet", "cli", "octo-org", "charmbracelet"}
	m, _ := newOwnerApp(t, newFakeOwners(), WithRecall(recall))
	runCommand(t, m, "goto @hubot")
	drive(m, m.key(press("esc")))
	tests := []struct {
		word string
		want []string
	}{
		{"@", []string{"@hubot", "@charmbracelet", "@cli", "@octo-org"}},
		{"@c", []string{"@charmbracelet", "@cli", "@octo-org"}},
		{"@CH", []string{"@charmbracelet"}},
		{"@hu", []string{"@hubot"}},
		{"@zz", nil},
	}
	for _, tt := range tests {
		line := "goto " + tt.word
		var got []string
		for _, c := range m.complete(line, len(line)) {
			got = append(got, c.Text)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("complete %q = %q, want %q", line, got, tt.want)
		}
	}
}

// The viewer's own login, which only shows the dashboard, isn't offered.
func TestCompleteOwnerLeavesOutViewer(t *testing.T) {
	t.Parallel()
	recall := newFakeRecall()
	recall.owners = []string{"CLI", "charmbracelet"}
	m, _ := newOwnerApp(t, newFakeOwners(), WithRecall(recall), WithLogin("cli"))
	line := "goto @c"
	cands := m.complete(line, len(line))
	got := make([]string, 0, len(cands))
	for _, c := range cands {
		got = append(got, c.Text)
	}
	if want := []string{"@charmbracelet"}; !slices.Equal(got, want) {
		t.Errorf("complete %q = %q, want %q", line, got, want)
	}
}

// A goto replaced by another goto opens only the second page.
func TestGotoOwnerReplacedByGotoOwner(t *testing.T) {
	t.Parallel()
	m, page := newOwnerApp(t, newFakeOwners())
	first := submitLine(t, m, "goto @octocat")
	second := submitLine(t, m, "goto @charmbracelet")
	drive(m, second)
	drive(m, first)
	if ownerOn(m) == nil || !slices.Equal(page.opened(), []string{"charmbracelet"}) {
		t.Errorf("screen %d with pages %q, want charmbracelet's alone", m.screen, page.opened())
	}
	if s := onScreen(m); !strings.Contains(s, "─ charmbracelet ─") {
		t.Errorf("the header doesn't name charmbracelet:\n%s", s)
	}
}

// TestOwnerKey checks that the owner key shows the page of the owner of
// the selection, or of the repository on view when the pane has nothing
// selected, the dashboard for the viewer's own login, and nothing where
// the selection has no owner, where help shows the key dimmed.
func TestOwnerKey(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		owner string
		// none reports that the pane has nothing selected.
		none bool
		// want is the login of the page shown, or "" for none, and dash
		// reports that the dashboard shows instead.
		want string
		dash bool
	}{
		{name: "user", owner: "octocat", want: "octocat"},
		{name: "organization", owner: "charmbracelet", want: "charmbracelet"},
		{name: "own login", owner: "Mona", dash: true},
		{name: "no owner, such as an app's"},
		{name: "nothing selected", none: true, want: "eggzec"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := &fakeOwnerPage{}
			sel := ui.Selection{What: "pull request", Repo: testRepo, Number: 7, Owner: tt.owner}
			layout := Layout{
				Files: &selectSection{fakeSection: &fakeSection{title: "Files"}, sel: sel, ok: !tt.none}, Pulls: &fakeSection{title: "Pull requests"},
				Dashboard: &fakeSection{title: ui.DashboardTitle},
			}
			owners := newFakeOwners()
			owners.remote["eggzec"] = core.Owner{Kind: core.OwnerOrg, Profile: core.Profile{Login: "eggzec"}}
			m := New(t.Context(), config.Default(), layout, WithRepo(testRepo), WithOwners(owners), WithLogin("mona"), page.opener())
			rankFakes(m)
			m.toast.SetDuration(0)
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			run(m, m.Init())
			if m.screen != repoScreen {
				t.Fatalf("screen = %d, want the repository", m.screen)
			}
			enabled := tt.want != "" || tt.dash
			if got := m.keys.state(m).Owner.Enabled(); got != enabled {
				t.Errorf("the owner key is enabled = %v in help, want %v", got, enabled)
			}
			drive(m, m.key(press("@")))
			switch {
			case tt.want != "":
				if ownerOn(m) == nil || page.Login() != tt.want {
					t.Errorf("screen %d with the page of %q, want the owner screen of %q", m.screen, page.Login(), tt.want)
				}
			case tt.dash:
				if m.screen != dashScreen || len(page.opened()) > 0 {
					t.Errorf("screen %d with pages %q, want the dashboard", m.screen, page.opened())
				}
			case m.screen != repoScreen || len(page.opened()) > 0:
				t.Errorf("screen %d with pages %q, want the repository still", m.screen, page.opened())
			}
			if s := toasted(m); s != "" {
				t.Errorf("toasted %s", s)
			}
		})
	}
}

// TestOwnerKeyOffWhereItShowsWhatIsOnView checks that the owner key does
// nothing on a page for the owner of that page, nor on the dashboard for
// the viewer, which already show there.
func TestOwnerKeyOffWhereItShowsWhatIsOnView(t *testing.T) {
	t.Parallel()
	t.Run("owner page", func(t *testing.T) {
		page := &fakeOwnerPage{sel: &ui.Selection{What: "repository", Repo: core.RepoRef{Owner: "Octocat", Name: "x"}, Owner: "Octocat"}}
		layout := Layout{Files: &fakeSection{title: "Files"}, Dashboard: &fakeSection{title: ui.DashboardTitle}}
		m := New(t.Context(), config.Default(), layout, WithOwners(newFakeOwners()), page.opener())
		rankFakes(m)
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		run(m, m.Init())
		runCommand(t, m, "goto @octocat")
		drive(m, m.key(press("@")))
		if ownerOn(m) == nil || len(page.mods) != 1 {
			t.Errorf("pages %q, want the key to do nothing on octocat's page", page.opened())
		}
		page.sel.Owner = "charmbracelet"
		drive(m, m.key(press("@")))
		if top := ownerOn(m); top == nil || top.login != "charmbracelet" {
			t.Errorf("pages %q, want the key to show another owner's page", page.opened())
		}
	})
	t.Run("dashboard", func(t *testing.T) {
		dash := &selectSection{fakeSection: &fakeSection{title: ui.DashboardTitle}, sel: ui.Selection{Owner: "mona"}, ok: true}
		layout := Layout{Files: &fakeSection{title: "Files"}, Dashboard: dash}
		m := New(t.Context(), config.Default(), layout, WithOwners(newFakeOwners()), WithLogin("mona"), (&fakeOwnerPage{}).opener())
		rankFakes(m)
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
		run(m, m.Init())
		if m.screen != dashScreen || m.keys.state(m).Owner.Enabled() {
			t.Errorf("screen %d, owner key enabled %v; want it off for the viewer on the dashboard", m.screen, m.keys.state(m).Owner.Enabled())
		}
		dash.sel.Owner = "octocat"
		if !m.keys.state(m).Owner.Enabled() {
			t.Error("the owner key is off for another owner on the dashboard")
		}
	})
}

// TestOwnerKeyInTheApp checks @ over the real sections and modals: it
// types where keys are typed, is no answer to a question, and shows the
// organization of a dashboard tab, even an empty one.
func TestOwnerKeyInTheApp(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		repo bool
		keys []string
		// owner is the owner of the selection before @, if the case
		// names one, and check reports what is wrong after it, if
		// anything.
		owner string
		check func(m *Model) string
	}{
		{name: "search query", keys: []string{"S", "/"}, check: func(m *Model) string {
			if got := layerNames(m.keyLayers()); m.searchHost() == nil || got != "always, search_query (types)" || !strings.Contains(onScreen(m), "@") {
				return "the query didn't take @: the keys reach " + got
			}
			return ""
		}},
		{name: "command line", repo: true, keys: []string{":"}, check: func(m *Model) string {
			if !m.line.Focused() || !strings.Contains(m.line.Value(), "@") {
				return "the command line didn't take @"
			}
			return ""
		}},
		{name: "filter field", repo: true, keys: []string{"2", "f", "G", "/"}, check: func(m *Model) string {
			if got := layerNames(m.keyLayers()); m.screen != repoScreen || got != "always, filter_query (types)" {
				return "the filter field didn't take @: the keys reach " + got
			}
			return ""
		}},
		{name: "issue question", repo: true, keys: []string{"3", "enter", "X"}, check: func(m *Model) string {
			if got := layerNames(m.keyLayers()); m.screen != repoScreen || got != "always, confirm" {
				return "the question went away: the keys reach " + got
			}
			return ""
		}},
		{name: "organization tab", keys: []string{"]"}, owner: "charmbracelet", check: onOwnerScreen},
		{name: "empty organization tab", keys: []string{"]", "]"}, owner: "octo-org", check: onOwnerScreen},
	}
	for _, tt := range tests {
		synctest.Test(t, func(t *testing.T) {
			m := newKeysApp(t, tt.repo)
			for _, k := range tt.keys {
				msg, _ := keyPress(k)
				driveKeys(t, m, m.key(msg))
			}
			if got := m.selectedOwner(); tt.owner != "" && got != tt.owner {
				t.Errorf("%s: the owner is %q, want %q", tt.name, got, tt.owner)
			}
			before := m.screen
			driveKeys(t, m, m.key(press("@")))
			if s := tt.check(m); s != "" {
				t.Errorf("%s: %s (screen %d, was %d):\n%s", tt.name, s, before, m.screen, onScreen(m))
			}
		})
	}
}

// onOwnerScreen reports that m doesn't show the page of an owner.
func onOwnerScreen(m *Model) string {
	if _, ok := m.modals.Top().(*owner.Modal); !ok {
		return "@ didn't show the owner's page"
	}
	return ""
}

// Backspace over an open modal goes nowhere: the modal has the key.
func TestBackspaceOverModalDoesNothing(t *testing.T) {
	t.Parallel()
	m, _ := newDashApp(t, core.RepoRef{})
	run(m, func() tea.Msg { return ui.RepoMsg{Repo: testRepo} })
	m.open(&fakeModal{title: "Preview"}, nil)
	run(m, m.key(press("backspace")))
	if m.screen != repoScreen || len(m.back) != 1 {
		t.Errorf("screen %d with %d places to go back to, want the repository and 1", m.screen, len(m.back))
	}
	if m.canGoBack() {
		t.Error("canGoBack = true over a modal")
	}
}

// Going back to a repository left for another opens it again.
func TestBackReturnsToTheEarlierRepository(t *testing.T) {
	t.Parallel()
	a, b := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}, core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}
	m, _ := newDashApp(t, core.RepoRef{})
	run(m, m.selectRepo(ui.RepoMsg{Repo: a}))
	run(m, m.key(press("0")))
	run(m, m.selectRepo(ui.RepoMsg{Repo: b}))
	if m.repo != b {
		t.Fatalf("repo = %v, want %v", m.repo, b)
	}
	run(m, m.key(press("backspace")))
	if m.screen != dashScreen {
		t.Fatalf("screen = %d, want the dashboard", m.screen)
	}
	run(m, m.key(press("backspace")))
	if m.screen != repoScreen || m.repo != a {
		t.Errorf("screen %d on %v, want the repository %v", m.screen, m.repo, a)
	}
}

// The places to go back to are capped.
func TestBackStackIsCapped(t *testing.T) {
	t.Parallel()
	m, _ := newDashApp(t, core.RepoRef{})
	for range 2 * maxBack {
		run(m, func() tea.Msg { return ui.RepoMsg{Repo: testRepo} })
		run(m, m.key(press("0")))
	}
	if len(m.back) != maxBack {
		t.Errorf("%d places to go back to, want %d", len(m.back), maxBack)
	}
}
