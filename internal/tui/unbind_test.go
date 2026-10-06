package tui

import (
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/actions"
	"github.com/eggzec/gh-tui/internal/tui/checks"
	"github.com/eggzec/gh-tui/internal/tui/dashboard"
	"github.com/eggzec/gh-tui/internal/tui/files"
	"github.com/eggzec/gh-tui/internal/tui/history"
	"github.com/eggzec/gh-tui/internal/tui/issues"
	"github.com/eggzec/gh-tui/internal/tui/notifications"
	"github.com/eggzec/gh-tui/internal/tui/owner"
	"github.com/eggzec/gh-tui/internal/tui/pulls"
	"github.com/eggzec/gh-tui/internal/tui/releases"
	searchpage "github.com/eggzec/gh-tui/internal/tui/search"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/internal/tui/ui/uitest"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// unbindPlace is somewhere in the app that TestUnbindEachAction looks
// at: it opens on testRepo if repo is set, or on the dashboard, takes
// msg if set, and then each step: an action, pressed by its first key,
// or else a key.
type unbindPlace struct {
	name  string
	repo  bool
	msg   tea.Msg
	steps []string
	// want names the layers of keys there, as layerNames does, so that
	// a place the steps no longer reach fails rather than passes.
	want string
}

// unbindPlaces are the screens, panes and modals of the app, each with
// its keys and hints.
func unbindPlaces() []unbindPlace {
	octocat := ui.OwnerMsg{Login: "octocat"}
	return []unbindPlace{
		{name: "dashboard", want: "app, global, dashboard, dashboard_repos"},
		{name: "dashboard pinned", steps: []string{config.ActionPane1}, want: "app, global, dashboard, dashboard_pinned"},
		{name: "dashboard work", steps: []string{config.ActionPane3}, want: "app, global, dashboard, dashboard_work"},
		{name: "dashboard calendar", steps: []string{config.ActionPane4}, want: "app, global, dashboard, dashboard_calendar"},
		{name: "dashboard inbox", steps: []string{config.ActionPane5}, want: "app, global, dashboard, dashboard_inbox"},
		{name: "dashboard zoomed", steps: []string{config.ActionZoom}, want: "app, global, dashboard, dashboard_repos"},
		{name: "dashboard filter", steps: []string{"dashboard_repos.filter"}, want: "global, filter"},
		{name: "notifications", steps: []string{config.ActionNotifications}, want: "app, global, notifications"},
		{name: "notifications filter", steps: []string{config.ActionNotifications, "notifications.filter"}, want: "global, filter"},
		{name: "notifications mark read", steps: []string{config.ActionNotifications, "notifications.read"}, want: "app, confirm"},
		{name: "search", steps: []string{config.ActionSearch}, want: "app, app, search_query (types)"},
		{name: "search kinds", steps: []string{config.ActionSearch, "up"}, want: "app, global, search, search_kinds"},
		// Typing on the code kind asks for a code search of the new text.
		{name: "search code", steps: []string{config.ActionSearch, "k", "e", "y", "up", "down", "down", "down", config.ActionPrevPane, "s"}, want: "app, app, search_query (types)"},
		{name: "search results", steps: []string{config.ActionSearch, "k", "e", "y", config.ActionSelect}, want: "app, global, search, search_results"},
		{name: "owner", msg: octocat, want: "app, global, owner, owner_list"},
		{name: "owner tab", msg: octocat, steps: []string{config.ActionNextTab}, want: "app, global, owner, owner_list"},
		{name: "owner readme", msg: octocat, steps: []string{config.ActionPane3}, want: "app, global, owner, owner_readme"},
		{name: "owner filter", msg: octocat, steps: []string{"owner_list.filter"}, want: "global, filter"},
		{name: "files", repo: true, want: "app, global, repo, files"},
		{name: "files zoomed", repo: true, steps: []string{config.ActionZoom}, want: "app, global, repo, files"},
		{name: "files preview", repo: true, steps: []string{"down", config.ActionSelect}, want: "global, preview"},
		{name: "finder", repo: true, steps: []string{config.ActionFindFile}, want: "app, finder (types)"},
		{name: "pull requests", repo: true, steps: []string{config.ActionPane2}, want: "app, global, repo, pulls"},
		{name: "pull requests filter", repo: true, steps: []string{config.ActionPane2, "pulls.filter"}, want: "global, filter"},
		{name: "pull requests merge", repo: true, steps: []string{config.ActionPane2, "pulls.merge"}, want: "app, confirm"},
		{name: "pull request", repo: true, steps: []string{config.ActionPane2, config.ActionSelect}, want: "global, pull_modal, pull_conversation"},
		{name: "checks", repo: true, steps: []string{config.ActionPane2, "pulls.checks"}, want: "global, pull_modal, pull_checks"},
		{name: "checks job", repo: true, steps: []string{config.ActionPane2, "pulls.checks", config.ActionSelect}, want: "global, pull_modal, pull_check_log"},
		{name: "checks detail", repo: true, steps: []string{config.ActionPane2, "pulls.checks", "down", config.ActionSelect}, want: "global, pull_modal, pull_check_detail"},
		{name: "issues", repo: true, steps: []string{config.ActionPane3}, want: "app, global, repo, issues"},
		{name: "issue", repo: true, steps: []string{config.ActionPane3, config.ActionSelect}, want: "global, issue_modal"},
		{name: "issue comment", repo: true, steps: []string{config.ActionPane3, config.ActionSelect, "issue_modal.comment"}, want: "app, prompt (types)"},
		{name: "history", repo: true, steps: []string{config.ActionHistory}, want: "global, history, history_graph"},
		{name: "history branches", repo: true, steps: []string{config.ActionHistory, config.ActionPrevPane}, want: "global, history, history_branches"},
		{name: "history files", repo: true, steps: []string{config.ActionHistory, config.ActionSelect}, want: "global, history, history_files"},
		{name: "history patch", repo: true, steps: []string{config.ActionHistory, config.ActionSelect, config.ActionSelect}, want: "global, history, history_patch"},
		{name: "commit", repo: true, msg: ui.OpenCommitMsg{Repo: testRepo, SHA: keyCommit.SHA}, want: "global, history, history_files"},
		{name: "release", repo: true, msg: ui.OpenReleaseMsg{Repo: testRepo, ID: keyRelease.ID, URL: keyRelease.URL}, want: "global, release_modal"},
		{name: "actions", repo: true, steps: []string{config.ActionActions}, want: "global, actions, actions_runs"},
		{name: "actions jobs", repo: true, steps: []string{config.ActionActions, config.ActionNextPane}, want: "global, actions, actions_jobs"},
		{name: "actions log", repo: true, steps: []string{config.ActionActions, config.ActionNextPane, config.ActionNextPane}, want: "global, actions, actions_log"},
		{name: "actions rerun", repo: true, steps: []string{config.ActionActions, "actions.rerun_failed"}, want: "app, confirm"},
		{name: "command line", repo: true, steps: []string{config.ActionCommand}, want: "command_line (types)"},
	}
}

// TestUnbindEachAction unbinds each action in turn, as keys.<context>.X: [] in the
// config does, and checks that the app still builds its key maps, and
// shows the dashboard and the repository screen and their help.
func TestUnbindEachAction(t *testing.T) {
	for _, action := range config.Default().Keys.Actions() {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			cfg.Keys.Set(action, []string{})
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate() = %v, want %s unbound", err, action)
			}
			for _, p := range unbindPlaces() {
				// The screens the app opens on need no step.
				if len(p.steps) == 0 && p.msg == nil {
					synctest.Test(t, func(t *testing.T) { checkUnbound(t, cfg, p) })
				}
			}
		})
	}
}

// TestUnbindAllElse goes to each place of the app with every action
// unbound but those the way there takes, and checks that it shows the
// place and its help, and names no key it doesn't have. Every action is
// unbound in most places, so the key maps that modals build as they open
// are checked without each key too.
func TestUnbindAllElse(t *testing.T) {
	for _, p := range unbindPlaces() {
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			cfg := config.Default()
			for _, action := range cfg.Keys.Actions() {
				if !slices.Contains(p.steps, action) {
					cfg.Keys.Set(action, []string{})
				}
			}
			synctest.Test(t, func(t *testing.T) { checkUnbound(t, cfg, p) })
		})
	}
}

// TestUnboundCommands checks that a command that presses the key of an
// action the config unbinds says so, rather than doing something else.
func TestUnboundCommands(t *testing.T) {
	for command, action := range map[string]string{
		"help": config.ActionHelp, "refresh": config.ActionRefresh, "open": config.ActionOpen,
	} {
		t.Run(command, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				cfg := config.Default()
				cfg.Keys.Set(action, []string{})
				m := newUnboundApp(t, cfg, true)
				for _, k := range append([]string{":"}, strings.Split(command, "")...) {
					msg, _ := keyPress(k)
					driveKeys(t, m, m.key(msg))
				}
				driveKeys(t, m, m.key(enter))
				if want := "No key is bound to keys." + action; !hasToast(m, want) {
					t.Errorf(":%s shows %q, want %q", command, toasted(m), want)
				}
			})
		})
	}
}

// checkUnbound goes to p in the app built with cfg, and checks what it
// shows there and in its help.
func checkUnbound(t *testing.T, cfg config.Config, p unbindPlace) {
	t.Helper()
	m := newUnboundApp(t, cfg, p.repo)
	if p.msg != nil {
		driveKeys(t, m, func() tea.Msg { return p.msg })
	}
	for _, s := range p.steps {
		name := s
		if keys := cfg.Keys.Of(s); len(keys) > 0 {
			name = keys[0]
		}
		msg, ok := keyPress(name)
		if !ok {
			t.Fatalf("%s: can't press %q", p.name, name)
		}
		driveKeys(t, m, m.key(msg))
	}
	if got := layerNames(m.keyLayers()); got != p.want {
		t.Fatalf("%s: the keys reach %q, want %q", p.name, got, p.want)
	}
	checkShown(t, p.name, m.View().Content)
	checkRows(t, p.name, m.keyLayers())
	if !m.helpOpen() {
		driveKeys(t, m, m.openHelp())
	}
	checkShown(t, p.name+" help", m.View().Content)
}

// checkShown fails on a hint that names an empty key.
func checkShown(t *testing.T, place, view string) {
	t.Helper()
	for _, bad := range []string{"Press  ", "Press .", " to open on GitHub", " to retry"} {
		if i := strings.Index(view, bad); i >= 0 && (bad[0] == 'P' || i == 0 || view[i-1] == ' ') {
			t.Errorf("%s: shows %q with no key", place, bad)
		}
	}
}

// checkRows fails on a row of the help that says nothing: a binding with
// neither keys nor a description.
func checkRows(t *testing.T, place string, layers []keyhelp.Layer) {
	t.Helper()
	for _, r := range keyhelp.Analyze(layers) {
		if len(r.Binding.Keys()) == 0 && r.Binding.Help().Desc == "" {
			t.Errorf("%s: the help of %s has a blank row", place, r.Source)
		}
	}
}

// newUnboundApp returns the app with every section and modal it has, as
// newKeysApp does, but with the keys of cfg.
func newUnboundApp(t *testing.T, cfg config.Config, repo bool) *Model {
	t.Helper()
	ctx := t.Context()
	acc := newFakeAccess(uitest.Classic("repo", "workflow", "notifications", "read:org", "gist"))
	v := ui.NewVoice(cfg.Keys, "")
	v.Token = ui.NewToken(acc, cfg.Keys)
	layout := Layout{
		Files: files.New(ctx, keyFiles{}, cfg.Keys, files.WithVoice(v)),
		Pulls: pulls.New(ctx, keyPulls{}, cfg.Keys, pulls.WithVoice(v),
			pulls.WithChecks(keyActions{}, checks.WithVoice(v))),
		Issues:        issues.New(ctx, keyIssues{}, cfg.Keys, issues.WithVoice(v)),
		Notifications: notifications.New(ctx, keyInbox{}, cfg.Keys, notifications.WithVoice(v)),
		Search:        searchpage.New(ctx, keySearch{}, cfg.Keys, searchpage.WithVoice(v)),
		Dashboard: dashboard.New(ctx, keyDash{}, cfg.Keys, dashboard.WithVoice(v),
			dashboard.WithInbox(keyInbox{}), dashboard.WithHere(testRepo, nil)),
		Owner: owner.New(ctx, keyOwners{}, cfg.Keys, owner.WithVoice(v)),
	}
	opts := []Option{
		WithVoice(v), WithAccess(acc),
		WithHistory(history.Opener(keyHistory{}, cfg.Keys, history.WithVoice(v))),
		WithCommit(history.CommitOpener(keyHistory{}, cfg.Keys, history.WithVoice(v))),
		WithRelease(releases.Opener(keyReleases{}, cfg.Keys, releases.WithVoice(v))),
		WithActions(actions.Opener(keyActions{}, cfg.Keys, actions.WithVoice(v))),
	}
	if repo {
		opts = append(opts, WithRepo(testRepo))
	}
	m := New(ctx, cfg, layout, opts...)
	m.toast.SetDuration(0)
	m.toast.SetErrorDuration(0)
	m.accessChanges = nil
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	driveKeys(t, m, m.Init())
	return m
}
