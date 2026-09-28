package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/service/access"
	notifsvc "github.com/eggzec/gh-tui/internal/service/notifications"
	"github.com/eggzec/gh-tui/internal/service/optimistic"
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// fakeAccess is what the app knows of the token, which a refresh by gh
// changes to next.
type fakeAccess struct {
	uitest.Checker
	mu      sync.Mutex
	changes chan core.Access
	plan    access.Plan
	// next is what the token may do once it is read again, or nil for
	// the same; reloadErr fails the read.
	next      *core.Access
	reloadErr error
	reloads   int
	off       bool
}

func newFakeAccess(a core.Access) *fakeAccess {
	return &fakeAccess{A: a, changes: make(chan core.Access, 1)}
}

func (f *fakeAccess) Access() core.Access {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.Checker.Access()
}

func (f *fakeAccess) Check(n core.Need) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.off {
		return nil
	}
	return f.Checker.Check(n)
}

func (f *fakeAccess) Changes() <-chan core.Access { return f.changes }

func (f *fakeAccess) Refresh(...core.Need) access.Plan {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.plan
}

func (f *fakeAccess) Reload(context.Context) (core.Access, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.reloads++
	if f.reloadErr != nil {
		return f.A, f.reloadErr
	}
	if f.next != nil {
		f.A, f.next = *f.next, nil
		f.plan = access.Plan{Why: "The token may do everything gh-tui does."}
	}
	return f.A, nil
}

func init() {
	// The question names the gh on the PATH by its name alone.
	lookGH = func() string { return "/usr/bin/gh" }
}

// refreshPlan is what gh does for a token of gh's that lacks workflow.
var refreshPlan = access.Plan{
	Cmd:    []string{"/usr/bin/gh", "auth", "refresh", "--hostname=github.com", "-s", "workflow"},
	Why:    "gh opens the browser to grant the token the workflow scope.",
	Scopes: []string{"workflow"},
}

func TestNotice(t *testing.T) {
	tests := []struct {
		name   string
		access core.Access
		off    bool
		want   string
	}{
		{name: "unknown", access: core.Access{}},
		{name: "scopes not listed yet", access: core.Access{Kind: core.TokenClassic}},
		{name: "gh's scopes", access: uitest.Classic("gist", "read:org", "repo")},
		{
			name: "no notifications", access: uitest.Classic("public_repo"),
			want: "Notifications need a classic token with the notifications scope · :auth",
		},
		{
			name: "fine-grained", access: core.Access{Kind: core.TokenFineGrained},
			want: "Notifications need a classic token with the notifications scope · :auth",
		},
		{
			name: "no scope to change anything", access: uitest.Classic("notifications", "gist"),
			want: "This token can't change anything on GitHub · :auth",
		},
		{
			name: "no repo", access: uitest.Classic("notifications", "public_repo"),
			want: "Private repositories, re-runs and cancelling need the repo scope · :auth",
		},
		{
			name: "SSO", access: core.Access{Kind: core.TokenClassic, Known: true, Scopes: []string{"repo"}, SSO: []string{"1"}},
			want: "Some organizations need SSO authorization · :auth",
		},
		{name: "workflow only", access: uitest.Classic("notifications", "repo")},
		{name: "checks turned off", access: uitest.Classic(), off: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			acc := newFakeAccess(tt.access)
			acc.off = tt.off
			m, _ := newTestApp(t, WithAccess(acc))
			if got := m.notice(tt.access); got != tt.want {
				t.Errorf("notice = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNoticeOnce(t *testing.T) {
	acc := newFakeAccess(core.Access{Kind: core.TokenClassic})
	m, fakes := newTestApp(t, WithAccess(acc))
	// run waits for every command, and the changes never end.
	m.accessChanges = nil
	want := "Private repositories, re-runs and cancelling need the repo scope · :auth"
	// GitHub hasn't said yet.
	run(m, m.startAccess())
	if hasToast(m, want) {
		t.Fatalf("the notice shows before GitHub said what the token may do")
	}
	a := uitest.Classic("notifications", "public_repo")
	acc.A = a
	run(m, m.updateAccess(accessChangedMsg{access: a}))
	if !hasToast(m, want) {
		t.Fatalf("toasts %q, want %q", toasted(m), want)
	}
	if !fakes[3].got(func(msg tea.Msg) bool { _, ok := msg.(ui.AccessMsg); return ok }) {
		t.Error("the sections weren't told what the token may do")
	}
	run(m, m.toast.Dismiss())
	run(m, m.updateAccess(accessChangedMsg{access: uitest.Classic("public_repo")}))
	if got := toasted(m); got != "" {
		t.Errorf("toasts %q after the first notice, want none", got)
	}
}

func TestAuthCommand(t *testing.T) {
	acc := newFakeAccess(uitest.Classic("gist", "read:org", "repo"))
	acc.plan = refreshPlan
	var ran [][]string
	var done func(error) tea.Msg
	m, fakes := newTestApp(t, WithAccess(acc), WithLogin("octocat"), withExec(func(argv []string, fn func(error) tea.Msg) tea.Cmd {
		ran, done = append(ran, argv), fn
		return nil
	}))
	runCommand(t, m, "auth")
	mod, ok := m.modal.(*authModal)
	if !ok {
		t.Fatalf("modal = %T, want the auth modal", m.modal)
	}
	if acc.reloads != 1 || mod.checking {
		t.Fatalf("the token was read %d times, checking=%v; want read once", acc.reloads, mod.checking)
	}
	view := ansi.Strip(m.View().Content)
	for _, want := range []string{
		"octocat@github.com · classic token", "Scopes: gist, read:org, repo",
		"✓ Read and mark notifications", "✗ Merge changes to workflows · needs workflow",
		"Run gh auth refresh --hostname=github.com -s workflow?",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("the modal doesn't show %q:\n%s", want, view)
		}
	}
	if !fakes[3].got(func(msg tea.Msg) bool { _, ok := msg.(ui.AccessMsg); return ok }) {
		t.Error("the sections weren't asked to read again what the token was refused")
	}

	run(m, m.key(press("y")))
	if m.modal != nil || !slices.EqualFunc(ran, [][]string{refreshPlan.Cmd}, slices.Equal) {
		t.Fatalf("ran %v with modal %T, want the plan's command with the modal closed", ran, m.modal)
	}
	next := uitest.Classic("gist", "read:org", "repo", "workflow")
	acc.next = &next
	run(m, func() tea.Msg { return done(nil) })
	if acc.reloads != 2 || !hasToast(m, "The token has the workflow scope now.") {
		t.Errorf("after gh: %d reads, toasts %q; want the token read again and the new scope told", acc.reloads, toasted(m))
	}
}

func TestAuthCommandRefused(t *testing.T) {
	acc := newFakeAccess(uitest.Classic("gist", "read:org", "repo"))
	acc.plan = refreshPlan
	var done func(error) tea.Msg
	m, _ := newTestApp(t, WithAccess(acc), withExec(func(_ []string, fn func(error) tea.Msg) tea.Cmd {
		done = fn
		return nil
	}))
	runCommand(t, m, "auth")
	run(m, m.key(press("y")))
	run(m, func() tea.Msg { return done(errors.New("exit status 1")) })
	if acc.reloads != 1 || !hasToast(m, "gh auth refresh didn't finish; nothing changed.") {
		t.Errorf("after a failed gh: %d reads, toasts %q", acc.reloads, toasted(m))
	}

	// Another account's token is refused, and the user told why.
	acc.reloadErr = &core.RefusedError{Action: "use the new token", Reason: "gh is logged in as mona now, not octocat; restart gh-tui to switch accounts"}
	run(m, m.reloadToken(&acc.A))
	if !hasToast(m, "gh is logged in as mona now") {
		t.Errorf("toasts %q, want the other account told", toasted(m))
	}
}

func TestAuthCommandWithNothingToRun(t *testing.T) {
	acc := newFakeAccess(core.Access{Kind: core.TokenFineGrained})
	acc.plan = access.Plan{Why: "A fine-grained token has no scopes to grant; run gh auth login to use a classic token."}
	m, _ := newTestApp(t, WithAccess(acc))
	runCommand(t, m, "auth")
	if _, ok := m.modal.(*authModal); !ok {
		t.Fatalf("modal = %T, want the auth modal", m.modal)
	}
	run(m, m.key(press("y")))
	if m.modal == nil {
		t.Fatal("y closed a modal that asks nothing")
	}
	run(m, m.key(press("esc")))
	if m.modal != nil {
		t.Errorf("esc left %T open", m.modal)
	}
}

func TestAuthModalView(t *testing.T) {
	tests := []struct {
		name   string
		access core.Access
		plan   access.Plan
		err    error
	}{
		{name: "a token of gh", access: uitest.Classic("gist", "read:org", "repo"), plan: refreshPlan},
		{
			name: "fine-grained", access: core.Access{Kind: core.TokenFineGrained},
			plan: access.Plan{Why: "A fine-grained token has no scopes to grant; run gh auth login to use a classic token."},
		},
		{
			name: "from GH_TOKEN", access: uitest.Classic("public_repo"),
			plan: access.Plan{
				URL: "https://github.com/settings/tokens", Scopes: []string{"notifications", "repo", "workflow"},
				Why: "The token comes from GH_TOKEN: update GH_TOKEN to a token with the notifications, repo and workflow scopes, or unset it and run gh auth login.",
			},
		},
		{
			name: "another account", access: uitest.Classic("repo", "notifications"), plan: refreshPlan,
			err: &core.RefusedError{Action: "use the new token", Reason: "gh is logged in as mona now, not octocat; restart gh-tui to switch accounts"},
		},
	}
	for _, tt := range tests {
		for _, dark := range []bool{false, true} {
			name := tt.name + "/light"
			if dark {
				name = tt.name + "/dark"
			}
			t.Run(name, func(t *testing.T) {
				p, err := config.Default().Palette(dark)
				if err != nil {
					t.Fatal(err)
				}
				tok := uitest.Token(&uitest.Checker{A: tt.access})
				mod := newAuthModal(config.Default().Keys, "octocat@github.com", core.Access{}, access.Plan{}, tok, nil)
				mod.SetTheme(ui.NewTheme(p, dark))
				w, h := mod.Fit(72, 30)
				mod.SetSize(w, h)
				mod.checked(tt.access, tt.plan, tt.err)
				w, h = mod.Fit(72, 30)
				mod.SetSize(w, h)
				golden.RequireEqual(t, mod.View())
			})
		}
	}
}

// TestAuthModalErrorMark checks that the modal marks why the token
// couldn't be read with the icon set's glyph, and joins the hint as the
// set does, as the app's other error lines are.
func TestAuthModalErrorMark(t *testing.T) {
	p, err := config.Default().Palette(true)
	if err != nil {
		t.Fatal(err)
	}
	for _, set := range []string{config.IconsNerd, config.IconsUnicode, config.IconsASCII} {
		cfg, err := config.Default().Set("ui.icons", set)
		if err != nil {
			t.Fatal(err)
		}
		m := New(t.Context(), cfg, Layout{}, WithRepo(testRepo), WithAccess(newFakeAccess(core.Access{})))
		m.theme = ui.NewTheme(p, true)
		_ = m.authCommand("")
		mod, ok := m.modal.(*authModal)
		if !ok {
			t.Fatalf("%s: the modal open is %T, want the auth modal", set, m.modal)
		}
		w, h := mod.Fit(72, 30)
		mod.SetSize(w, h)
		mod.checked(core.Access{}, access.Plan{}, fmt.Errorf("read the token: %w", core.ErrOffline))
		ic := ui.NewIcons(set)
		want := ic.Error + " Can't reach GitHub"
		if v := ansi.Strip(mod.View()); !strings.Contains(v, want) {
			t.Errorf("%s: view lacks %q:\n%s", set, want, v)
		}
	}
}

// TestProgramTellsWhatTheTokenLacks runs the program with a token that
// lacks repo, and checks that it says so once GitHub said.
func TestProgramTellsWhatTheTokenLacks(t *testing.T) {
	_, fakes := newTestApp(t)
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: fakes[3]}
	acc := newFakeAccess(core.Access{Kind: core.TokenClassic})
	app := New(t.Context(), config.Default(), layout, WithRepo(testRepo), WithAccess(acc))
	tm := teatest.NewTestModel(t, app, teatest.WithInitialTermSize(120, 24))
	a := uitest.Classic("notifications", "public_repo")
	acc.mu.Lock()
	acc.A = a
	acc.mu.Unlock()
	acc.changes <- a
	teatest.WaitFor(t, tm.Output(), func(out []byte) bool {
		return drawn(out, "cancelling need the repo scope")
	}, teatest.WithDuration(5*time.Second))
	tm.Send(ctrlC)
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

func TestAuthPromptNamesTheProgram(t *testing.T) {
	argv := []string{"/usr/bin/gh", "auth", "refresh", "--hostname=github.com"}
	if got := commandLine(argv, "/usr/bin/gh"); got != "gh auth refresh --hostname=github.com" {
		t.Errorf("the gh on the PATH shows as %q", got)
	}
	argv[0] = "/opt/gh/bin/gh"
	if got := commandLine(argv, "/usr/bin/gh"); got != "/opt/gh/bin/gh auth refresh --hostname=github.com" {
		t.Errorf("a gh GH_PATH names shows as %q, want its path", got)
	}
	if got := commandLine(argv, ""); !strings.HasPrefix(got, "/opt/gh/bin/gh ") {
		t.Errorf("without gh on the PATH, it shows as %q", got)
	}
}

// fakeInbox serves the notifications of the program, and counts reads.
type fakeInbox struct {
	mu    sync.Mutex
	lists int
}

func (f *fakeInbox) CachedList(notifsvc.ListQuery) (core.Page[core.Notification], bool) {
	return core.Page[core.Notification]{}, false
}

func (f *fakeInbox) List(context.Context, notifsvc.ListQuery) (core.Page[core.Notification], error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists++
	r := core.RepoRef{Owner: "eggzec", Name: "gh-tui"}
	return core.Page[core.Notification]{Items: []core.Notification{{
		ID: "1", Repo: r, Unread: true, UpdatedAt: time.Now(),
		Subject: core.Subject{Title: "Badge is off by one", Type: core.SubjectIssue, Number: 1, WebURL: "https://github.com/eggzec/gh-tui/issues/1"},
	}}}, nil
}

func (f *fakeInbox) MarkRead(string) *optimistic.Op {
	return optimistic.New(func(context.Context) error { return nil })
}
func (f *fakeInbox) MarkDone(string) *optimistic.Op {
	return optimistic.New(func(context.Context) error { return nil })
}
func (f *fakeInbox) MarkAllRead(time.Time) *optimistic.Op {
	return optimistic.New(func(context.Context) error { return nil })
}
func (f *fakeInbox) Invalidate() {}

// TestChecksOff runs the app with auth.check off and a token that lacks
// every scope: nothing tells the user of it, the notifications are read,
// the marks are offered, and :auth says why.
func TestChecksOff(t *testing.T) {
	cfg := config.Default()
	cfg.Auth.Check = false
	acc := newFakeAccess(uitest.Classic("gist"))
	// The access service checks nothing while checks are off.
	acc.off = true
	m, notes, in := newTokenApp(t, cfg, acc)
	run(m, m.updateAccess(accessChangedMsg{access: acc.A}))
	if got := toasted(m); got != "" {
		t.Errorf("toasts %q with checks off, want none", got)
	}
	if in.lists == 0 || !strings.Contains(onScreen(m), "Badge is off by one") {
		t.Errorf("after %d reads the screen shows %q, want the notifications", in.lists, onScreen(m))
	}
	if got := uitest.Enabled(notes.KeyLayers()); !slices.Contains(got, "read") {
		t.Errorf("the notifications offer %v, want the mark", got)
	}
	runCommand(t, m, "auth")
	if got := onScreen(m); !strings.Contains(got, "auth.check is off") {
		t.Errorf(":auth shows %q, want it to say checks are off", got)
	}
}

// newTokenApp returns an app with the notifications section, whose token
// acc knows of, started on the notifications, and what the section reads.
func newTokenApp(t *testing.T, cfg config.Config, acc *fakeAccess) (*Model, *notifications.Section, *fakeInbox) {
	t.Helper()
	v := ui.NewVoice(cfg.Keys, "")
	v.Token = ui.NewToken(acc, cfg.Keys)
	in := &fakeInbox{}
	notes := notifications.New(t.Context(), in, cfg.Keys, notifications.WithVoice(v))
	fakes := []*fakeSection{{title: "Files"}, {title: "Pull requests"}, {title: "Issues"}}
	layout := Layout{Files: fakes[0], Pulls: fakes[1], Issues: fakes[2], Notifications: notes}
	m := New(t.Context(), cfg, layout, WithAccess(acc), WithVoice(v))
	m.toast.SetDuration(0)
	// run waits for every command, and the changes never end.
	m.accessChanges = nil
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 24})
	run(m, m.Init())
	return m, notes, in
}

// helpStatus returns how the open help lists the binding described as
// desc, or -1 when it doesn't.
func helpStatus(m *Model, desc string) keyhelp.Status {
	for _, r := range m.keyhelp.Rows() {
		if r.Binding.Help().Desc == desc {
			return r.Status
		}
	}
	return -1
}

// TestHelpShowsWhatTheTokenRefuses checks that the help lists the marks
// the token may not make as disabled, and the keys of :auth.
func TestHelpShowsWhatTheTokenRefuses(t *testing.T) {
	acc := newFakeAccess(uitest.Classic("gist"))
	acc.plan = refreshPlan
	m, _, in := newTokenApp(t, config.Default(), acc)
	if in.lists != 0 {
		t.Errorf("the notifications were read %d times", in.lists)
	}
	run(m, m.key(press("?")))
	for _, desc := range []string{"read", "done"} {
		if got := helpStatus(m, desc); got != keyhelp.Disabled {
			t.Errorf("the help lists %q as %v, want disabled", desc, got)
		}
	}
	run(m, m.key(press("?")))
	runCommand(t, m, "auth")
	run(m, m.key(press("?")))
	if !m.helpOpen() || !strings.HasSuffix(m.keyhelp.Title(), "Token") {
		t.Fatalf("? over :auth opened the help %v titled %q", m.helpOpen(), m.keyhelp.Title())
	}
	for _, desc := range []string{"yes", "no"} {
		if got := helpStatus(m, desc); got == -1 || got == keyhelp.Disabled {
			t.Errorf("the help over :auth lists %q as %v, want it offered", desc, got)
		}
	}
}
