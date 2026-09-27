package tui

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// fakeRepos knows the repositories in cached, in memory, and in remote,
// on GitHub. Any other is not found, unless err says otherwise.
type fakeRepos struct {
	mu     sync.Mutex
	cached map[core.RepoRef]core.Repo
	remote map[core.RepoRef]core.Repo
	err    error
	gets   []core.RepoRef
	// ctxs are the contexts of the reads, to tell whether they were
	// canceled.
	ctxs []context.Context
}

func (f *fakeRepos) CachedGet(ref core.RepoRef) (core.Repo, bool) {
	r, ok := f.cached[ref]
	return r, ok
}

func (f *fakeRepos) Get(ctx context.Context, ref core.RepoRef) (core.Repo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.gets = append(f.gets, ref)
	f.ctxs = append(f.ctxs, ctx)
	if err := ctx.Err(); err != nil {
		return core.Repo{}, err
	}
	if f.err != nil {
		return core.Repo{}, f.err
	}
	for k := range f.remote {
		if k.Same(ref) {
			return f.remote[k], nil
		}
	}
	return core.Repo{}, core.ErrNotFound
}

// errOffline stands for an error of a GitHub that can't be reached.
var errOffline = fmt.Errorf("github: %w: %w", core.ErrOffline,
	&url.Error{Op: "Get", URL: "https://api.github.com", Err: errors.New("no route to host")})

var bubbletea = core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}

func newGotoRepos() *fakeRepos {
	return &fakeRepos{
		cached: map[core.RepoRef]core.Repo{testRepo: {Ref: testRepo}},
		remote: map[core.RepoRef]core.Repo{bubbletea: {Ref: bubbletea}, {Owner: "cli", Name: "cli"}: {Ref: core.RepoRef{Owner: "cli", Name: "cli"}}},
	}
}

// newGotoApp returns an app with a dashboard, opened on it unless opts
// name a repository, whose goto reads repos.
func newGotoApp(t *testing.T, repos *fakeRepos, opts ...Option) (*Model, []*fakeSection) {
	t.Helper()
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}, {title: ui.DashboardTitle}}
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3], Dashboard: fakes[4]}
	opts = append([]Option{WithRepos(repos)}, opts...)
	m := New(t.Context(), config.Default(), layout, opts...)
	m.toast.SetDuration(0)
	m.toast.SetErrorDuration(0)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	run(m, m.Init())
	return m, fakes
}

func TestGotoRepo(t *testing.T) {
	tests := []struct {
		name string
		line string
		err  error
		// want is the repository shown, or zero for none, and toast the
		// text of the toast.
		want  core.RepoRef
		toast string
		// asks reports whether GitHub is asked.
		asks bool
	}{
		{name: "cached", line: "goto eggzec/gh-tui", want: testRepo},
		{name: "on GitHub", line: "goto charmbracelet/bubbletea", want: bubbletea, asks: true},
		{name: "spelled as GitHub does", line: "goto CharmBracelet/BubbleTea", want: bubbletea, asks: true},
		{name: "link", line: "goto https://github.com/cli/cli", want: core.RepoRef{Owner: "cli", Name: "cli"}, asks: true},
		{name: "link to a page of it", line: "goto github.com/cli/cli/actions", want: core.RepoRef{Owner: "cli", Name: "cli"}, asks: true},
		{name: "not found", line: "goto nosuchowner/nosuchrepo", toast: "nosuchowner/nosuchrepo doesn't exist or is private.", asks: true},
		{name: "offline", line: "goto charmbracelet/bubbletea", err: errOffline, toast: "Couldn't open charmbracelet/bubbletea: can't reach GitHub.", asks: true},
		{name: "rate limited", line: "goto charmbracelet/bubbletea", err: core.ErrRateLimited, toast: "Couldn't open charmbracelet/bubbletea: rate limited by GitHub.", asks: true},
		{name: "nothing", line: "goto", toast: "Nothing to open"},
		{name: "not a repository", line: "goto bubbletea", toast: "Not a repository"},
		{name: "another host", line: "goto https://gitlab.com/a/b", toast: "Not a link to github.com"},
		{name: "bare", line: "charmbracelet/bubbletea", toast: "Unknown command: charmbracelet/bubbletea."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repos := newGotoRepos()
			repos.err = tt.err
			m, _ := newGotoApp(t, repos)
			runCommand(t, m, tt.line)
			if tt.want != (core.RepoRef{}) {
				if m.screen != repoScreen || m.repo != tt.want {
					t.Errorf("screen %d with %v, want the repository screen of %v", m.screen, m.repo, tt.want)
				}
			} else if m.screen != dashScreen {
				t.Errorf("screen = %d, want the dashboard still", m.screen)
			}
			if tt.toast != "" && !hasToast(m, tt.toast) {
				t.Errorf("toasts lack %q: %s", tt.toast, toasted(m))
			}
			if asked := len(repos.gets) > 0; asked != tt.asks {
				t.Errorf("asked GitHub = %v, want %v", asked, tt.asks)
			}
			if m.going != nil {
				t.Error("the goto still waits")
			}
		})
	}
}

// submitLine opens the line, types line and submits it, and returns what
// the command asks for, without running it.
func submitLine(t *testing.T, m *Model, line string) tea.Cmd {
	t.Helper()
	drive(m, m.key(press(":")))
	typeKeys(m, line)
	msg := m.key(enter)()
	_, cmd := m.Update(msg)
	return cmd
}

func TestGotoShowsItWaits(t *testing.T) {
	repos := newGotoRepos()
	m, _ := newGotoApp(t, repos)
	cmd := submitLine(t, m, "goto charmbracelet/bubbletea")
	if m.going == nil {
		t.Fatal("the goto doesn't wait for GitHub")
	}
	if s := onScreen(m); !strings.Contains(s, "Opening charmbracelet/bubbletea…") {
		t.Errorf("the footer doesn't say what it opens:\n%s", s)
	}
	drive(m, cmd)
	if m.going != nil || m.repo != bubbletea {
		t.Error("the goto didn't end on the repository")
	}
	if s := onScreen(m); strings.Contains(s, "Opening") || !strings.Contains(s, "? help") {
		t.Errorf("the help should be back:\n%s", s)
	}
}

func TestGotoIsCanceled(t *testing.T) {
	tests := []struct {
		name string
		// then is what the user does while the goto waits.
		then func(m *Model)
	}{
		{name: "another command", then: func(m *Model) { m.runLine("goto cli/cli", nil) }},
		{name: "another screen", then: func(m *Model) { drive(m, m.key(press("n"))) }},
		{name: "another repository", then: func(m *Model) { drive(m, func() tea.Msg { return ui.RepoMsg{Repo: testRepo} }) }},
		{name: "a modal", then: func(m *Model) { m.openModal(&fakeModal{title: "Preview"}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repos := newGotoRepos()
			m, _ := newGotoApp(t, repos)
			cmd := submitLine(t, m, "goto charmbracelet/bubbletea")
			tt.then(m)
			if m.going != nil && m.going.target.Repo == bubbletea {
				t.Fatal("the goto still waits")
			}
			drive(m, cmd)
			if len(repos.ctxs) != 1 || repos.ctxs[0].Err() == nil {
				t.Error("the read wasn't canceled")
			}
			if m.repo == bubbletea {
				t.Error("the canceled goto opened its repository")
			}
			if s := toasted(m); s != "" {
				t.Errorf("the canceled goto told of its error: %s", s)
			}
		})
	}
}

func TestGotoWaitsWhileTheLineIsOpen(t *testing.T) {
	m, _ := newGotoApp(t, newGotoRepos())
	cmd := submitLine(t, m, "goto charmbracelet/bubbletea")
	drive(m, m.key(press(":")))
	if s := onScreen(m); m.going == nil || strings.Contains(s, "Opening") {
		t.Fatalf("opening the line should keep the goto, and show the line:\n%s", s)
	}
	drive(m, m.key(press("esc")))
	if s := onScreen(m); !strings.Contains(s, "Opening charmbracelet/bubbletea…") {
		t.Errorf("the footer should show the goto again:\n%s", s)
	}
	drive(m, cmd)
	if m.repo != bubbletea {
		t.Errorf("repo = %v, want the goto to end on %v", m.repo, bubbletea)
	}
}

func TestProgramGotoRepo(t *testing.T) {
	_, fakes := newGotoApp(t, newGotoRepos())
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3], Dashboard: fakes[4]}
	app := New(t.Context(), config.Default(), layout, WithRepos(newGotoRepos()))
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(80, 24))
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return drawn(out, "Dashboard content")
	}, teatest.WithDuration(5*time.Second))
	tm.Send(press(":"))
	for _, r := range "goto charmbracelet/bubbletea" {
		tm.Send(press(string(r)))
	}
	tm.Send(enter)
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		// Only the repository screen draws its files.
		return drawn(out, "Files content")
	}, teatest.WithDuration(5*time.Second))
	tm.Send(press("q"))
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok || final.screen != repoScreen || final.repo != bubbletea {
		t.Error("the final model should show the repository screen of charmbracelet/bubbletea")
	}
}

// fakeKinds knows the kinds of numbers in cached, in memory, and in
// remote, on GitHub. Any other number is neither, unless err says
// otherwise.
type fakeKinds struct {
	mu     sync.Mutex
	cached map[core.Target]core.NumberKind
	remote map[core.Target]core.NumberKind
	err    error
	asked  []core.Target
}

func (f *fakeKinds) CachedKind(repo core.RepoRef, number int) (core.NumberKind, bool) {
	k, ok := f.cached[core.Target{Repo: repo, Number: number}]
	return k, ok
}

func (f *fakeKinds) Kind(ctx context.Context, repo core.RepoRef, number int) (core.NumberKind, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t := core.Target{Repo: repo, Number: number}
	f.asked = append(f.asked, t)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if f.err != nil {
		return "", f.err
	}
	if k, ok := f.remote[t]; ok {
		return k, nil
	}
	return "", &core.NoNumberError{Repo: repo, Number: number, Err: core.ErrNotFound}
}

func newGotoKinds() *fakeKinds {
	return &fakeKinds{
		cached: map[core.Target]core.NumberKind{{Repo: testRepo, Number: 7}: core.KindPull},
		remote: map[core.Target]core.NumberKind{
			{Repo: bubbletea, Number: 1813}: core.KindPull,
			{Repo: bubbletea, Number: 1698}: core.KindIssue,
			{Repo: testRepo, Number: 12}:    core.KindIssue,
		},
	}
}

// opened returns the message the sections got to open an issue or pull
// request, or nil.
func opened(fakes []*fakeSection) tea.Msg {
	for _, msg := range fakes[1].msgs {
		switch msg.(type) {
		case ui.OpenPullMsg, ui.OpenIssueMsg:
			return msg
		}
	}
	return nil
}

// TestGotoFailures checks that goto tells why it couldn't open a
// repository, in the app's voice, whatever GitHub failed with.
func TestGotoFailures(t *testing.T) {
	// Where goto's toast isn't a change's: a refusal names the
	// repository, which says what the action would, and the longer action
	// leaves no room for the log's path.
	toasts := map[string]string{
		"forbidden": "You don't have access to charmbracelet/bubbletea.",
		"not found": "charmbracelet/bubbletea doesn't exist or is private.",
		"sso":       "charmbracelet requires SSO. Run gh auth refresh, then restart gh-tui.",
		"internal":  "Couldn't open charmbracelet/bubbletea: something went wrong, see the log.",
	}
	cases := append(failures(), failure{
		name: "sso", err: &ghError{is: core.ErrForbidden, reason: "Resource protected by organization SAML enforcement."},
	})
	for _, f := range cases {
		t.Run(f.name, func(t *testing.T) {
			repos := newGotoRepos()
			repos.err = f.err
			m, _ := newGotoApp(t, repos, WithVoice(logVoice(t)))
			m.Update(tea.WindowSizeMsg{Width: 240, Height: 24})
			runCommand(t, m, "goto charmbracelet/bubbletea")
			want := toasts[f.name]
			if want == "" && f.cause != "" {
				want = "Couldn't open charmbracelet/bubbletea: " + f.cause
			}
			switch got := toasted(m); {
			case want == "" && got != "":
				t.Errorf("toast %q, want none", got)
			case want != "" && !hasToast(m, want):
				t.Errorf("toast %q, want %q", got, want)
			}
			checkClean(t, toasted(m))
			if m.screen != dashScreen {
				t.Errorf("screen = %d, want the dashboard still", m.screen)
			}
		})
	}
}

func TestGotoNumber(t *testing.T) {
	cli := core.RepoRef{Owner: "cli", Name: "cli"}
	tests := []struct {
		name string
		// repo is the repository selected, if any, and notif shows the
		// notifications over it.
		repo  core.RepoRef
		notif bool
		line  string
		err   error
		// want is the message that opens the number, or nil, and toast
		// the text of the toast.
		want  tea.Msg
		toast string
		// asks reports whether GitHub is asked what the number is.
		asks bool
	}{
		{name: "pull request", line: "goto charmbracelet/bubbletea#1813", want: ui.OpenPullMsg{Repo: bubbletea, Number: 1813}, asks: true},
		{name: "issue", line: "goto charmbracelet/bubbletea#1698", want: ui.OpenIssueMsg{Repo: bubbletea, Number: 1698}, asks: true},
		{name: "known", line: "goto eggzec/gh-tui#7", want: ui.OpenPullMsg{Repo: testRepo, Number: 7}},
		{name: "number of the repository", repo: testRepo, line: "goto #12", want: ui.OpenIssueMsg{Repo: testRepo, Number: 12}, asks: true},
		{name: "number without a repository", line: "goto #12", toast: "Open a repository first, or use goto owner/name#12."},
		{name: "number off the repository screen", repo: testRepo, notif: true, line: "goto #12", toast: "Open a repository first, or use goto owner/name#12."},
		{name: "link to a pull request", line: "goto https://github.com/cli/cli/pull/1", want: ui.OpenPullMsg{Repo: cli, Number: 1}},
		{name: "link to an issue", line: "goto github.com/cli/cli/issues/5#issuecomment-1", want: ui.OpenIssueMsg{Repo: cli, Number: 5}},
		{name: "neither", line: "goto charmbracelet/bubbletea#99999999", toast: "charmbracelet/bubbletea#99999999 doesn't exist or is private.", asks: true},
		{name: "offline", line: "goto charmbracelet/bubbletea#1813", err: errOffline, toast: "Couldn't open charmbracelet/bubbletea#1813: can't reach GitHub.", asks: true},
		{name: "not a number", repo: testRepo, line: "goto #x", toast: "Not an issue number"},
		{name: "bare", repo: testRepo, line: "#12", toast: "Unknown command: #12."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kinds := newGotoKinds()
			kinds.err = tt.err
			opts := []Option{WithKinds(kinds)}
			if tt.repo != (core.RepoRef{}) {
				opts = append(opts, WithRepo(tt.repo))
			}
			m, fakes := newGotoApp(t, newGotoRepos(), opts...)
			if tt.notif {
				m.showScreen(notifScreen, 0)
			}
			screen := m.screen
			runCommand(t, m, tt.line)
			if got := opened(fakes); got != tt.want {
				t.Errorf("sections got %#v, want %#v", got, tt.want)
			}
			if m.screen != screen {
				t.Errorf("screen = %d, want %d still", m.screen, screen)
			}
			if tt.toast != "" && !hasToast(m, tt.toast) {
				t.Errorf("toasts lack %q: %s", tt.toast, toasted(m))
			}
			if asked := len(kinds.asked) > 0; asked != tt.asks {
				t.Errorf("asked GitHub = %v, want %v", asked, tt.asks)
			}
			if m.going != nil {
				t.Error("the goto still waits")
			}
		})
	}
}

func TestGotoNumberWithoutKinds(t *testing.T) {
	m, fakes := newGotoApp(t, newGotoRepos())
	runCommand(t, m, "goto charmbracelet/bubbletea#1813")
	if got, want := opened(fakes), (ui.OpenIssueMsg{Repo: bubbletea, Number: 1813}); got != want {
		t.Errorf("sections got %#v, want %#v, whose modal shows a pull request too", got, want)
	}
}

func TestGotoNumberIsCanceled(t *testing.T) {
	kinds := newGotoKinds()
	m, fakes := newGotoApp(t, newGotoRepos(), WithKinds(kinds))
	cmd := submitLine(t, m, "goto charmbracelet/bubbletea#1813")
	if s := onScreen(m); !strings.Contains(s, "Opening charmbracelet/bubbletea#1813…") {
		t.Errorf("the footer doesn't say what it opens:\n%s", s)
	}
	// Going to the notifications drops it.
	drive(m, m.key(press("n")))
	drive(m, cmd)
	if got := opened(fakes); got != nil {
		t.Errorf("the canceled goto opened %#v", got)
	}
}

func TestProgramGotoNumber(t *testing.T) {
	_, fakes := newGotoApp(t, newGotoRepos())
	fakes[1].reply = func(msg tea.Msg) tea.Cmd {
		if o, ok := msg.(ui.OpenPullMsg); ok {
			return ui.OpenModal(&fakeModal{title: fmt.Sprintf("Pull request %s#%d", o.Repo, o.Number)})
		}
		return nil
	}
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3], Dashboard: fakes[4]}
	app := New(t.Context(), config.Default(), layout, WithRepos(newGotoRepos()), WithKinds(newGotoKinds()))
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(80, 24))
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return drawn(out, "Dashboard content")
	}, teatest.WithDuration(5*time.Second))
	tm.Send(press(":"))
	for _, r := range "goto charmbracelet/bubbletea#1813" {
		tm.Send(press(string(r)))
	}
	tm.Send(enter)
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		// Only the modal draws its body.
		return drawn(out, "Pull request charmbracelet/bubbletea#1813 body")
	}, teatest.WithDuration(5*time.Second))
	tm.Send(press("esc"))
	tm.Send(ctrlC)
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok || final.screen != dashScreen || final.modal == nil {
		t.Error("the final model should show the modal over the dashboard")
	}
}
