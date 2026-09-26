package tui

import (
	"context"
	"errors"
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
var errOffline = &url.Error{Op: "Get", URL: "https://api.github.com", Err: errors.New("no route to host")}

func unreachable(_ context.Context, err error) bool {
	_, ok := errors.AsType[*url.Error](err)
	return ok
}

var bubbletea = core.RepoRef{Owner: "charmbracelet", Name: "bubbletea"}

func newGotoRepos() *fakeRepos {
	return &fakeRepos{
		cached: map[core.RepoRef]core.Repo{testRepo: {Ref: testRepo}},
		remote: map[core.RepoRef]core.Repo{bubbletea: {Ref: bubbletea}, {Owner: "cli", Name: "cli"}: {Ref: core.RepoRef{Owner: "cli", Name: "cli"}}},
	}
}

// newGotoApp returns an app with a dashboard, opened on it, whose goto
// reads repos.
func newGotoApp(t *testing.T, repos *fakeRepos) (*Model, []*fakeSection) {
	t.Helper()
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}, {title: "Notifications"}, {title: ui.DashboardTitle}}
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3], Dashboard: fakes[4]}
	m := New(t.Context(), config.Default(), layout, WithRepos(repos), WithUnreachable(unreachable))
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
		{name: "not found", line: "goto nosuchowner/nosuchrepo", toast: "Repository not found: nosuchowner/nosuchrepo.", asks: true},
		{name: "offline", line: "goto charmbracelet/bubbletea", err: errOffline, toast: "Can't reach GitHub to open charmbracelet/bubbletea.", asks: true},
		{name: "rate limited", line: "goto charmbracelet/bubbletea", err: core.ErrRateLimited, toast: "Couldn't open charmbracelet/bubbletea: rate limited.", asks: true},
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
			if tt.toast != "" && !strings.Contains(toasted(m), tt.toast) {
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
		{name: "another command", then: func(m *Model) { m.runLine("goto cli/cli") }},
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
