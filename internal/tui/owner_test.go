package tui

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
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

// fakeOwnerPage is a page of owners that keeps the logins it was given,
// and goes back through them with esc, as the owner page does.
type fakeOwnerPage struct {
	fakeSection
	logins []string
}

func (p *fakeOwnerPage) Update(msg tea.Msg) tea.Cmd {
	p.msgs = append(p.msgs, msg)
	switch msg := msg.(type) {
	case ui.OwnerMsg:
		if !p.focused {
			p.logins = nil
		}
		p.logins = append(p.logins, msg.Login)
	case tea.KeyPressMsg:
		if msg.String() != "esc" {
			return nil
		}
		if len(p.logins) > 1 {
			p.logins = p.logins[:len(p.logins)-1]
			return nil
		}
		return func() tea.Msg { return ui.BackMsg{} }
	}
	return nil
}

func (p *fakeOwnerPage) View() string { return "page of " + p.Login() }

func (p *fakeOwnerPage) Login() string {
	if len(p.logins) == 0 {
		return ""
	}
	return p.logins[len(p.logins)-1]
}

// newOwnerApp returns an app on the dashboard whose goto reads owners, and
// its page of owners.
func newOwnerApp(t *testing.T, owners *fakeOwners, opts ...Option) (*Model, *fakeOwnerPage) {
	t.Helper()
	page := &fakeOwnerPage{}
	page.title = ui.OwnerTitle
	layout := Layout{
		Files: &fakeSection{title: "Files"}, Pulls: &fakeSection{title: "Pull requests"}, Issues: &fakeSection{title: "Issues"},
		Notifications: &fakeSection{title: "Notifications"}, Dashboard: &fakeSection{title: ui.DashboardTitle}, Owner: page,
	}
	opts = append([]Option{WithRepos(newGotoRepos()), WithOwners(owners)}, opts...)
	m := New(t.Context(), config.Default(), layout, opts...)
	m.toast.SetDuration(0)
	m.toast.SetErrorDuration(0)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(m, m.Init())
	return m, page
}

func TestGotoOwner(t *testing.T) {
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
				if m.screen != ownerScreen || page.Login() != tt.want {
					t.Errorf("screen %d with the page of %q, want the owner screen of %q", m.screen, page.Login(), tt.want)
				}
				if !strings.Contains(onScreen(m), "─ "+tt.want+" ─") {
					t.Errorf("the header doesn't name %s:\n%s", tt.want, onScreen(m))
				}
			case m.screen != dashScreen:
				t.Errorf("screen = %d, want the dashboard", m.screen)
			}
			if !tt.dash && tt.want == "" && len(page.logins) > 0 {
				t.Errorf("the page was given %q", page.logins)
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
	m, page := newOwnerApp(t, newFakeOwners())
	cmd := submitLine(t, m, "goto @octocat")
	if m.going == nil {
		t.Fatal("the goto doesn't wait for GitHub")
	}
	if s := onScreen(m); !strings.Contains(s, "Opening @octocat…") {
		t.Errorf("the footer doesn't say what it opens:\n%s", s)
	}
	drive(m, cmd)
	if m.going != nil || m.screen != ownerScreen || page.Login() != "octocat" {
		t.Error("the goto didn't end on the page")
	}
}

// Another command replaces a goto that waits, and its answer is ignored.
func TestGotoOwnerIsReplaced(t *testing.T) {
	m, page := newOwnerApp(t, newFakeOwners())
	cmd := submitLine(t, m, "goto @octocat")
	runCommand(t, m, "goto eggzec/gh-tui")
	drive(m, cmd)
	if m.screen != repoScreen || len(page.logins) > 0 {
		t.Errorf("screen %d with pages %q, want the repository alone", m.screen, page.logins)
	}
}

// Without a reader goto opens any page named, and without a page it says
// there is none.
func TestGotoOwnerUnchecked(t *testing.T) {
	m, page := newOwnerApp(t, nil)
	m.owners = nil
	runCommand(t, m, "goto @someone")
	if m.screen != ownerScreen || page.Login() != "someone" {
		t.Errorf("screen %d with the page of %q, want someone's", m.screen, page.Login())
	}

	m, _ = newGotoApp(t, newGotoRepos())
	runCommand(t, m, "goto @octocat")
	if want := "Can't open @octocat: there are no pages of users and organizations here."; !hasToast(m, want) {
		t.Errorf("toasts lack %q: %s", want, toasted(m))
	}
}

// esc goes back through the pages opened one from another, and then to
// the screen the first was opened from; opened from elsewhere, a page
// starts a new way back.
func TestOwnerBack(t *testing.T) {
	m, page := newOwnerApp(t, newFakeOwners())
	runCommand(t, m, "goto @octocat")
	runCommand(t, m, "goto @hubot")
	if s := onScreen(m); !strings.Contains(s, "─ hubot ─") {
		t.Errorf("the header doesn't name hubot:\n%s", s)
	}
	drive(m, m.key(press("esc")))
	if m.screen != ownerScreen || page.Login() != "octocat" {
		t.Fatalf("screen %d with the page of %q, want octocat's", m.screen, page.Login())
	}
	if s := onScreen(m); !strings.Contains(s, "─ octocat ─") {
		t.Errorf("the header doesn't name octocat again:\n%s", s)
	}
	drive(m, m.key(press("esc")))
	if m.screen != dashScreen {
		t.Fatalf("screen = %d, want the dashboard it was opened from", m.screen)
	}
	runCommand(t, m, "goto eggzec/gh-tui")
	runCommand(t, m, "goto @charmbracelet")
	if !slices.Equal(page.logins, []string{"charmbracelet"}) {
		t.Errorf("pages = %q, want charmbracelet's alone", page.logins)
	}
	drive(m, m.key(press("esc")))
	if m.screen != repoScreen {
		t.Errorf("screen = %d, want the repository it was opened from", m.screen)
	}
}

// The page takes the keys of the panes, as the dashboard does.
func TestOwnerTakesPaneKeys(t *testing.T) {
	m, page := newOwnerApp(t, newFakeOwners())
	runCommand(t, m, "goto @hubot")
	drive(m, m.key(press("2")))
	if m.screen != ownerScreen || !page.got(isKey("2")) {
		t.Errorf("screen %d, page got 2 = %v; want the page to take it", m.screen, page.got(isKey("2")))
	}
}

func TestCompleteOwner(t *testing.T) {
	recall := newFakeRecall()
	recall.owners = []string{"charmbracelet", "cli", "octo-org", "charmbracelet"}
	m, _ := newOwnerApp(t, newFakeOwners(), WithRecall(recall))
	runCommand(t, m, "goto @hubot")
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

// A goto replaced by another goto opens only the second page.
func TestGotoOwnerReplacedByGotoOwner(t *testing.T) {
	m, page := newOwnerApp(t, newFakeOwners())
	first := submitLine(t, m, "goto @octocat")
	second := submitLine(t, m, "goto @charmbracelet")
	drive(m, second)
	drive(m, first)
	if m.screen != ownerScreen || !slices.Equal(page.logins, []string{"charmbracelet"}) {
		t.Errorf("screen %d with pages %q, want charmbracelet's alone", m.screen, page.logins)
	}
	if s := onScreen(m); !strings.Contains(s, "─ charmbracelet ─") {
		t.Errorf("the header doesn't name charmbracelet:\n%s", s)
	}
}

// A page without an account yet is named by its title, without a link.
func TestOwnerHeaderWithoutLogin(t *testing.T) {
	m, _ := newOwnerApp(t, newFakeOwners())
	run(m, m.showScreen(ownerScreen, 0))
	if m.screen != ownerScreen {
		t.Fatalf("screen = %d, want the owner screen", m.screen)
	}
	if strings.Contains(m.header, "\x1b]8;;http") {
		t.Errorf("the header links somewhere: %q", m.header)
	}
	if s := onScreen(m); !strings.Contains(s, "─ "+ui.OwnerTitle+" ─") {
		t.Errorf("the header doesn't name the page:\n%s", s)
	}
}

// TestOwnerKey checks that the owner key shows the page of the owner of
// the selection, the dashboard for the viewer's own login, and nothing
// where the selection has no owner, where help shows the key dimmed.
func TestOwnerKey(t *testing.T) {
	tests := []struct {
		name  string
		owner string
		// want is the login of the page shown, or "" for none, and dash
		// reports that the dashboard shows instead.
		want  string
		dash  bool
		toast string
	}{
		{name: "user", owner: "octocat", want: "octocat"},
		{name: "organization", owner: "charmbracelet", want: "charmbracelet"},
		{name: "own login", owner: "Mona", dash: true},
		{name: "app", owner: "dependabot[bot]", toast: "dependabot[bot] is an app; apps have no page here."},
		{name: "no owner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := &fakeOwnerPage{}
			page.title = ui.OwnerTitle
			sel := ui.Selection{What: "pull request", Repo: testRepo, Number: 7, Owner: tt.owner}
			layout := Layout{
				Files: &selectSection{fakeSection: &fakeSection{title: "Files"}, sel: sel, ok: true}, Pulls: &fakeSection{title: "Pull requests"},
				Dashboard: &fakeSection{title: ui.DashboardTitle}, Owner: page,
			}
			m := New(t.Context(), config.Default(), layout, WithRepo(testRepo), WithOwners(newFakeOwners()), WithLogin("mona"))
			m.toast.SetDuration(0)
			m.toast.SetErrorDuration(0)
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
			run(m, m.Init())
			if m.screen != repoScreen {
				t.Fatalf("screen = %d, want the repository", m.screen)
			}
			if got := m.keys.state(m).Owner.Enabled(); got != (tt.owner != "") {
				t.Errorf("the owner key is enabled = %v in help, want %v", got, tt.owner != "")
			}
			drive(m, m.key(press("@")))
			switch {
			case tt.want != "":
				if m.screen != ownerScreen || page.Login() != tt.want {
					t.Errorf("screen %d with the page of %q, want the owner screen of %q", m.screen, page.Login(), tt.want)
				}
			case tt.dash:
				if m.screen != dashScreen || len(page.logins) > 0 {
					t.Errorf("screen %d with pages %q, want the dashboard", m.screen, page.logins)
				}
			case m.screen != repoScreen || len(page.logins) > 0:
				t.Errorf("screen %d with pages %q, want the repository still", m.screen, page.logins)
			}
			if tt.toast != "" && !hasToast(m, tt.toast) {
				t.Errorf("toasts lack %q: %s", tt.toast, toasted(m))
			}
		})
	}
}
