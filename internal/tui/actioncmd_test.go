package tui

import (
	"slices"
	"strings"
	"testing"
	"testing/synctest"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
)

// refusals are what the app says in place of running a command.
var refusals = []string{"works in", "Unknown command", "nothing to", "Nothing here", "Close "}

// runReaches runs run, which does what a key or a command does, and says
// whether it reached something the services here don't serve, which fails
// loudly: such a run did reach its action.
func runReaches(run func() tea.Cmd) (cmd tea.Cmd, reached bool) {
	defer func() {
		if recover() != nil {
			reached = true
		}
	}()
	return run(), false
}

// effect reports whether run does something in m: it sends a message,
// changes what is on view or the keys that work, or reaches what the
// services don't serve.
func effect(m *Model, run func() tea.Cmd) bool {
	before, was := ansi.Strip(m.View().Content), layerNames(m.keyLayers())
	cmd, reached := runReaches(run)
	after, is := ansi.Strip(m.View().Content), layerNames(m.keyLayers())
	return reached || sends(cmd) || before != after || was != is
}

// TestActionCommandsWorkUnbound checks that every action that help lists
// with a key, and that is a command, does there with the command what the
// key does, when the config unbinds the action: a command needs no key.
func TestActionCommandsWorkUnbound(t *testing.T) {
	// Each action is checked in the first context that its key does
	// something in, which keeps the test short.
	checked := map[string]bool{}
	for _, c := range keyContexts() {
		if c.context == "" || !pendingActions(c, checked) {
			continue
		}
		t.Run(strings.NewReplacer(" ", "-", ":", "").Replace(c.name), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				_, layers := c.reach(t)
				for _, l := range layers {
					if l.Context == "" || l.Typing {
						continue
					}
					for _, b := range l.Bindings {
						if !b.Enabled() || b.Help().Desc == "" {
							continue
						}
						for _, action := range keymap.Actions(b) {
							// A pane lists the keys of the contexts it
							// shares a viewer with, which its chain lacks,
							// and a step that presses an action can't
							// reach the context without its key.
							ctx, _, _ := strings.Cut(action, ".")
							if !config.Commandable(action) || checked[action] || !slices.Contains(config.Chain(c.context), ctx) ||
								slices.Contains(c.steps, action) || slices.Contains(c.after, action) {
								continue
							}
							if _, idle := idleRows[l.Context+" "+b.Help().Desc]; idle || action == config.ActionZoom && strings.HasPrefix(c.context, "actions") {
								// The Actions modal zooms with no change in
								// the view but the hint of the key, which
								// an unbound action has none of.
								continue
							}
							checkUnboundCommand(t, c, action, checked)
						}
					}
				}
			})
		})
	}
}

// pendingActions reports whether the context c has a command, but those
// of the global context, that checked doesn't have yet, so that a context
// whose commands are all checked isn't reached again.
func pendingActions(c keyContext, checked map[string]bool) bool {
	keys := config.Default().Keys
	for _, ctx := range config.Chain(c.context) {
		if ctx == config.ContextGlobal {
			continue
		}
		for _, action := range keys.Actions() {
			if name, _, _ := strings.Cut(action, "."); name == ctx && config.Commandable(action) && !checked[action] {
				return true
			}
		}
	}
	return false
}

// checkUnboundCommand checks, in the context c, that the key of action
// does something there, and that the command of the same name does
// something with the key unbound, and neither is refused, and records it
// in checked.
func checkUnboundCommand(t *testing.T, c keyContext, action string, checked map[string]bool) {
	t.Helper()
	ctx, name, _ := strings.Cut(action, ".")
	var press tea.KeyPressMsg
	for _, k := range config.Default().Keys.Of(action) {
		if msg, ok := keyPress(k); ok {
			press = msg
			break
		}
	}
	if press.String() == "" {
		return
	}
	m, _ := c.reach(t)
	if !effect(m, func() tea.Cmd { return m.key(press) }) {
		// TestHelpRowsWork answers for a key that does nothing here.
		return
	}
	checked[action] = true
	// A key that asks to confirm is a mutation, which its command asks
	// for too, and not only sends.
	asks := strings.Contains(layerNames(m.keyLayers()), "confirm")
	m, _ = c.reachWith(t, func(cfg *config.Config) { cfg.Keys.Set(action, []string{}) })
	if !effect(m, func() tea.Cmd { return m.runLine(name, nil) }) {
		t.Errorf("%s: :%s with %s unbound does nothing, as its key does", c.name, name, ctx)
	}
	if asks && !strings.Contains(layerNames(m.keyLayers()), "confirm") {
		t.Errorf("%s: :%s with %s unbound does without asking, as its key asks", c.name, name, ctx)
	}
	for _, r := range refusals {
		if strings.Contains(toasted(m), r) {
			t.Errorf("%s: :%s with %s unbound says %q", c.name, name, ctx, toasted(m))
		}
	}
}

// TestActionCommandsAskLikeKeys checks that a mutation reached by its
// command asks, as its key does, with the key unbound.
func TestActionCommandsAskLikeKeys(t *testing.T) {
	for _, tt := range []struct {
		name   string
		action string
		steps  []string
		// asks is what the question says, if it is checked.
		asks string
	}{
		{"merge a pull request", "pulls.merge", []string{"global.pane_2"}, ""},
		{"close a pull request", "pulls.close", []string{"global.pane_2"}, ""},
		{"mark read", "notifications.read", []string{"global.notifications"}, "Mark eggzec/gh-tui#1 as read?"},
		{"mark done", "notifications.done", []string{"global.notifications"}, "Mark eggzec/gh-tui#1 as done?"},
		{"mark all read", "notifications.read", []string{"global.notifications"}, "Mark all notifications as read?"},
		{"star the repository", "repo.star", nil, "Star eggzec/gh-tui?"},
		{"close an issue", "issues.close", []string{"global.pane_3"}, ""},
		{"close the issue", "issue_modal.close", []string{"global.pane_3", "global.select"}, ""},
		{"merge the pull request", "pull_modal.merge", []string{"global.pane_2", "global.select"}, ""},
		{"close the pull request", "pull_modal.close", []string{"global.pane_2", "global.select"}, ""},
		{"draft the pull request", "pull_modal.draft", []string{"global.pane_2", "global.select"}, ""},
		{"re-run the failed jobs", "actions.rerun_failed", []string{"repo.actions"}, ""},
		{"re-run a run", "actions.rerun", []string{"repo.actions"}, ""},
		{"close the marked pull request", "pulls.close", []string{"global.pane_2", "pulls.mark"}, "Close 1 pull request?"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				_, name, _ := strings.Cut(tt.action, ".")
				if tt.name == "mark all read" {
					// Marking all read is an argument of read.
					name = "read all"
				}
				m := keysAfter(t, true, tt.steps...)
				if got := layerNames(m.keyLayers()); got == "always, confirm" {
					t.Fatalf("the app asks before the command: %s", got)
				}
				if tt.name == "close the marked pull request" {
					// The row is marked, so that the question is the bulk one.
					unmarked := keysAfter(t, true, tt.steps[:len(tt.steps)-1]...)
					if ansi.Strip(m.View().Content) == ansi.Strip(unmarked.View().Content) {
						t.Fatal("the mark changed nothing on view")
					}
				}
				unbound := newKeysAppWith(t, true, func(c *config.Config) { c.Keys.Set(tt.action, []string{}) })
				c := keyContext{name: tt.name, repo: true}
				for _, s := range tt.steps {
					for _, k := range c.press(t, config.Default().Keys, s) {
						msg, _ := keyPress(k)
						driveKeys(t, unbound, unbound.key(msg))
					}
				}
				runCommand(t, unbound, name)
				if got := layerNames(unbound.keyLayers()); got != "always, confirm" {
					t.Errorf(":%s with %s unbound reached %q, want the question its key asks: toasts %s", name, tt.action, got, toasted(unbound))
				}
				if view := ansi.Strip(unbound.View().Content); !strings.Contains(view, tt.asks) {
					t.Errorf(":%s with %s unbound asks something else than %q:\n%s", name, tt.action, tt.asks, view)
				}
			})
		})
	}
}

// TestActionCommandsNameWhereTheyWork checks that a command that the focus
// doesn't have is refused with the places it works in, from the screens,
// and over a modal that doesn't have it, with the modal naming itself.
func TestActionCommandsNameWhereTheyWork(t *testing.T) {
	for _, tt := range []struct {
		name  string
		repo  bool
		steps []string
		line  string
		want  string
	}{
		{"on the dashboard", false, nil, "merge", "merge works in Pull requests (Repository) and Pull request modal."},
		{"on the files", true, nil, "merge", "merge works in Pull requests (Repository) and Pull request modal."},
		{"over history", true, []string{"repo.history"}, "merge", "Close History first to use merge."},
		{"over the issue", true, []string{"global.pane_3", "global.select"}, "merge", "Close the issue first to use merge."},
		{"history on the dashboard", false, nil, "history", "history works in Repository screen."},
		{"rerun over history", true, []string{"repo.history"}, "rerun", "Close History first to use rerun."},
		{"a name many have", true, nil, "filter", "filter works in Repositories (Dashboard), Pull requests (Repository), Issues (Repository), Notifications screen and 4 more."},
		{"a navigation key", true, nil, "up", "Unknown command: up."},
		{"quit", true, nil, "quit", "Unknown command: quit."},
		{"no such command", true, nil, "nosuch", "Unknown command: nosuch."},
		{"an argument", true, []string{"global.pane_2"}, "close now", "The close command takes no argument."},
		{"read all on the files", true, nil, "read all", "read all works in Notifications screen."},
		{"read all on the dashboard", false, nil, "read all", "read all works in Notifications screen."},
		{"read all over the pull request", true, []string{"global.pane_2", "global.select"}, "read all", "Close the pull request first to use read all."},
		{"read on the files", true, nil, "read", "read works in Notifications screen."},
		{"read with another word", false, []string{"global.notifications"}, "read some", "The read command takes no argument but all."},
		{"rerun on the files", true, nil, "rerun", "rerun works in Actions modal."},
		{"rerun over the pull request", true, []string{"global.pane_2", "global.select"}, "rerun", "Close the pull request first to use rerun."},
		{"star on the dashboard", false, nil, "star", "star works in Repository screen."},
		{"star over history", true, []string{"repo.history"}, "star", "Close History first to use star."},
		{"labels on the files", true, nil, "labels", "labels works in Issues (Repository) and Issue modal."},
		{"labels on the pull requests", true, []string{"global.pane_2"}, "labels", "labels works in Issues (Repository) and Issue modal."},
		{"labels over history", true, []string{"repo.history"}, "labels", "Close History first to use labels."},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, tt.repo, tt.steps...)
				open := m.modal
				runCommand(t, m, tt.line)
				if !hasToast(m, tt.want) {
					t.Errorf(":%s toasts %s, want %q", tt.line, toasted(m), tt.want)
				}
				if m.modal != open {
					t.Error("the refusal changed the modal")
				}
			})
		})
	}
}

// TestActionCommandsResolveInTheFocusedChain checks that a name that
// several contexts have is the innermost one's.
func TestActionCommandsResolveInTheFocusedChain(t *testing.T) {
	for _, tt := range []struct {
		name  string
		steps []string
		line  string
		want  string
	}{
		{"pull requests", []string{"global.pane_2"}, "close", "pulls.close"},
		{"issues", []string{"global.pane_3"}, "close", "issues.close"},
		{"the pull request", []string{"global.pane_2", "global.select"}, "close", "pull_modal.close"},
		{"checks fall through", []string{"global.pane_2", "global.select"}, "merge", "pull_modal.merge"},
		{"global", []string{"global.pane_2"}, "refresh", "global.refresh"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, tt.steps...)
				got, ok := m.resolveAction(tt.line)
				if !ok || got != tt.want {
					t.Errorf("%s resolves to %q, %v, want %q", tt.line, got, ok, tt.want)
				}
			})
		})
	}
}

// TestActionsNeverShadowBuiltInCommands checks that no command that is an
// action takes the name of a built-in command, except the few that share
// it with an action on purpose, whose built-in command runs first.
func TestActionsNeverShadowBuiltInCommands(t *testing.T) {
	shared := map[string]bool{"open": true, "search": true, "references": true}
	for _, a := range config.Default().Keys.Actions() {
		_, name, _ := strings.Cut(a, ".")
		if _, builtin := findCommand(name); builtin && config.Commandable(a) && !shared[name] {
			t.Errorf("the action %s is a command named like the built-in %s", a, name)
		}
	}
	for _, name := range config.Navigation() {
		if _, builtin := findCommand(name); builtin {
			t.Errorf("%s is a built-in command and an action with no command", name)
		}
	}
}

// TestBuiltInCommandsRunFirst checks that the built-in commands that an
// action shares its name with do what they do, and not the action.
func TestBuiltInCommandsRunFirst(t *testing.T) {
	b := &browser{}
	m, _ := newGotoApp(t, newGotoRepos(), WithBrowser(b.open), WithRepo(testRepo))
	runCommand(t, m, "open cli/cli")
	if !slices.Contains(b.urls, "https://github.com/cli/cli") {
		t.Errorf("open cli/cli opened %q, want the repository", b.urls)
	}
}

// TestCompletionListsTheFocusedChainFirst checks that the line completes
// the actions of the focused pane, then those of its screen or modal, then
// the built-in commands and the global actions by name.
func TestCompletionListsTheFocusedChainFirst(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2")
		got := texts(m.complete("c", 1))
		want := []string{"checks", "clear_filter", "close", "config ", "copy "}
		if !slices.Equal(got, want) {
			t.Errorf("on the pull requests, c completes %q, want %q", got, want)
		}
		for _, cand := range m.complete("c", 1)[:3] {
			if cand.Detail == "" {
				t.Errorf("%s is completed without what it does", cand.Label)
			}
		}
		m = keysAfter(t, true, "global.pane_3")
		if got := texts(m.complete("c", 1)); !slices.Equal(got, []string{"clear_filter", "close", "comment", "config ", "copy "}) {
			t.Errorf("on the issues, c completes %q", got)
		}
		m = keysAfter(t, true)
		if got := texts(m.complete("c", 1)); !slices.Equal(got, []string{"collapse", "config ", "copy "}) {
			t.Errorf("on the files, c completes %q", got)
		}
	})
}

// TestActionCommandsSayWhenNothingHappens checks that a command that
// resolves, and whose key does nothing where it is, says there is nothing
// to do instead of leaving the line silent.
func TestActionCommandsSayWhenNothingHappens(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		steps []string
		line  string
	}{
		{"maximize with no modal", nil, "maximize"},
		{"reopen what is open", []string{"global.pane_3"}, "reopen"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, tt.steps...)
				open := m.modal
				runCommand(t, m, tt.line)
				if want := "There is nothing to " + tt.line + " here."; !hasToast(m, want) {
					t.Errorf(":%s toasts %s, want %q", tt.line, toasted(m), want)
				}
				if m.modal != open {
					t.Error("the refusal changed the modal")
				}
			})
		})
	}
}

// TestModalRefusesWhatItDoesNotOwn checks that a command that works
// somewhere else, over a modal that doesn't have it, makes the modal name
// itself, even where the command works in a modal of the kind.
func TestModalRefusesWhatItDoesNotOwn(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2", "global.select", "pull_modal.references")
		if got := layerNames(m.keyLayers()); !strings.Contains(got, "references") {
			t.Skipf("the pull request has no linked items to show here: %s", got)
		}
		runCommand(t, m, "merge")
		if want := "Close " + m.modalName(m.topModal()) + " first to use merge."; !hasToast(m, want) {
			t.Errorf(":merge toasts %s, want %q", toasted(m), want)
		}
	})
}

// TestInnermostContextResolvesFirst checks that a name that a pane and its
// modal both have is the pane's, which has the focus, and the modal's with
// the pane's key unbound from the config.
func TestInnermostContextResolvesFirst(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2", "global.select", "global.next_tab", "global.pane_1")
		chain := m.focusChain()
		pane := slices.IndexFunc(chain, func(ctx string) bool { return ctx == "pull_files" })
		modal := slices.Index(chain, "pull_modal")
		if pane < 0 || modal < 0 {
			t.Fatalf("the focus chain is %v, want the files and the pull request in it", chain)
		}
		if pane > modal {
			t.Fatalf("the focus chain is %v, want the pane before its modal", chain)
		}
		m.cfg.Keys.Set("pull_files.merge", []string{"ctrl+g"})
		if got, ok := m.resolveAction("merge"); !ok || got != "pull_files.merge" {
			t.Errorf("merge resolves to %q, %v, want the pane's", got, ok)
		}
		m.cfg.Keys.Set("pull_files.merge", nil)
		delete(m.cfg.Keys["pull_files"], "merge")
		if got, ok := m.resolveAction("merge"); !ok || got != "pull_modal.merge" {
			t.Errorf("merge resolves to %q, %v, want the modal's", got, ok)
		}
	})
}

// TestActionCommandsWaitForAWidgetThatTakesKeys checks that a command is
// refused, naming the modal to close, while the modal takes every key.
func TestActionCommandsWaitForAWidgetThatTakesKeys(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2", "pulls.filter")
		open := m.modal
		m.actionCommand("merge", "")
		if want := "Close the filter first to use merge."; !hasToast(m, want) {
			t.Errorf(":merge toasts %s, want %q", toasted(m), want)
		}
		if m.modal != open {
			t.Error("the refusal changed the modal")
		}
	})
}

// TestActionCommandsThatOnlySendDontWarn checks that a command whose
// effect is the command it returns, which doesn't change the view until
// it runs, is not taken for one that does nothing.
func TestActionCommandsThatOnlySendDontWarn(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		steps []string
		line  string
	}{
		{"refresh the pull requests", []string{"global.pane_2"}, "refresh"},
		{"refresh the files", nil, "refresh"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, tt.steps...)
				cmd := m.actionCommand(tt.line, "")
				if cmd == nil {
					t.Fatalf(":%s returned no command", tt.line)
				}
				if hasToast(m, "nothing to") {
					t.Errorf(":%s toasts %s, want no warning", tt.line, toasted(m))
				}
			})
		})
	}
}

// TestActionCommandsWaitForACapturingSection checks that a command is
// refused, as the key is, while the focused section takes every key.
func TestActionCommandsWaitForACapturingSection(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.pane_2", "pulls.quick_filter")
		if c, ok := m.focused().section.(ui.Capturer); !ok || !c.Capturing() {
			t.Fatal("the quick filter doesn't capture keys")
		}
		m.actionCommand("merge", "")
		if want := "Finish typing first to use merge."; !hasToast(m, want) {
			t.Errorf(":merge toasts %s, want %q", toasted(m), want)
		}
	})
}

// TestReadAllIsTheReadCommandWithAll checks that read all, and not a key,
// marks every notification read, and asks first, and that read alone is
// the action of the key.
func TestReadAllIsTheReadCommandWithAll(t *testing.T) {
	if _, ok := config.Default().Keys["notifications"]["read_all"]; ok {
		t.Fatal("default.yaml still has notifications.read_all")
	}
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.notifications")
		runCommand(t, m, "read all")
		if got := layerNames(m.keyLayers()); got != "always, confirm" {
			t.Fatalf(":read all reached %q, want the question: toasts %s", got, toasted(m))
		}
		if view := ansi.Strip(m.View().Content); !strings.Contains(view, "Mark all notifications as read?") {
			t.Errorf(":read all asks something else:\n%s", view)
		}
		// A key that was M asks nothing now.
		m = keysAfter(t, true, "global.notifications")
		tap(t, m, "M")
		if got := layerNames(m.keyLayers()); got == "always, confirm" {
			t.Error("M still asks to mark all read")
		}
		// Read alone marks the selected one.
		m = keysAfter(t, true, "global.notifications")
		runCommand(t, m, "read")
		if view := ansi.Strip(m.View().Content); strings.Contains(view, "all notifications") || layerNames(m.keyLayers()) != "always, confirm" {
			t.Errorf(":read asks about all, or nothing:\n%s", view)
		}
	})
}

// TestReadAllCompletes checks that the line completes read where the
// notifications are, and all after it.
func TestReadAllCompletes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m := keysAfter(t, true, "global.notifications")
		if got := texts(m.complete("rea", 3)); !slices.Equal(got, []string{"read"}) {
			t.Errorf("rea completes %q, want read", got)
		}
		if got := texts(m.complete("read a", 6)); !slices.Equal(got, []string{"all"}) {
			t.Errorf("read a completes %q, want all", got)
		}
		if got := texts(m.complete("read x", 6)); len(got) != 0 {
			t.Errorf("read x completes %q", got)
		}
		m = keysAfter(t, true)
		if got := texts(m.complete("read a", 6)); len(got) != 0 {
			t.Errorf("read a completes %q on the files", got)
		}
	})
}

// TestMutationCommandsCompleteWhereTheyWork checks that rerun, star and
// labels are completed in the chain where they work, and only there.
func TestMutationCommandsCompleteWhereTheyWork(t *testing.T) {
	for _, tt := range []struct {
		name  string
		steps []string
		line  string
		want  bool
	}{
		{"star on the files", nil, "star", true},
		{"star on the dashboard", []string{"global.dashboard"}, "star", false},
		{"rerun in the actions", []string{"repo.actions"}, "rerun", true},
		{"rerun on the files", nil, "rerun", false},
		{"labels in the issue", []string{"global.pane_3", "global.select"}, "labels", true},
		{"labels on the pull requests", []string{"global.pane_2"}, "labels", false},
		{"read in the notifications", []string{"global.notifications"}, "read", true},
		{"read on the files", nil, "read", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := keysAfter(t, true, tt.steps...)
				if got := slices.Contains(texts(m.complete(tt.line, len(tt.line))), tt.line); got != tt.want {
					t.Errorf("%s completes: %v, want %v", tt.line, got, tt.want)
				}
			})
		})
	}
}

// TestLabelsCommandOpensTheEditor checks that labels, with the key unbound,
// opens the label editor of the issue, whose submit asks before it sends.
func TestLabelsCommandOpensTheEditor(t *testing.T) {
	for _, tt := range []struct {
		name   string
		action string
		steps  []string
	}{
		{"from the issue", "issue_modal.labels", []string{"global.pane_3", "global.select"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				m := newKeysAppWith(t, true, func(c *config.Config) { c.Keys.Set(tt.action, []string{}) })
				c := keyContext{name: tt.name, repo: true}
				for _, s := range tt.steps {
					for _, k := range c.press(t, config.Default().Keys, s) {
						msg, _ := keyPress(k)
						driveKeys(t, m, m.key(msg))
					}
				}
				runCommand(t, m, "labels")
				if got := layerNames(m.keyLayers()); got != "always, prompt (types)" {
					t.Fatalf(":labels reached %q, want the editor: toasts %s", got, toasted(m))
				}
				typeKeys(m, ", extra")
				driveKeys(t, m, m.key(enter))
				if got := layerNames(m.keyLayers()); got != "always, confirm" {
					t.Errorf("submitting the labels reached %q, want the question", got)
				}
			})
		})
	}
}

// TestMutationCommandsKeepBuiltInNames checks that the commands that
// change something don't take the names of the built-in ones.
func TestMutationCommandsKeepBuiltInNames(t *testing.T) {
	for _, name := range []string{"read", "rerun", "star", "labels"} {
		if _, builtin := findCommand(name); builtin {
			t.Errorf("%s is a built-in command, which would run in place of its action", name)
		}
		if config.ActionContexts(config.Default().Keys, name) == nil {
			t.Errorf("%s is no action of any context", name)
		}
	}
}
