package tui

import (
	"slices"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// KeyMap holds the keys the app handles itself. Sections have their own.
type KeyMap struct {
	Quit   key.Binding
	Help   key.Binding
	Search key.Binding
	// History opens the history of the repository on the repository
	// screen.
	History key.Binding
	// Actions opens the workflow runs of the repository on the repository
	// screen.
	Actions key.Binding
	// FindFile opens the file finder of the section that has one, on the
	// repository screen.
	FindFile key.Binding
	// Command opens the command line in place of the status bar.
	Command key.Binding
	// form holds the keys of the filter modal's form.
	form filterform.KeyMap
	// Notifications switches between the screen on view and the
	// notifications, and Dashboard between it and the dashboard.
	Notifications key.Binding
	Dashboard     key.Binding
	// Next and Prev cycle the focus through the panes.
	Next key.Binding
	Prev key.Binding
	// Panes focus pane 1, 2 and 3.
	Panes []key.Binding
	// Zoom shows the focused pane of the repository screen alone, or
	// every pane again.
	Zoom key.Binding
	// Back goes back through the owner pages and the screens shown.
	Back key.Binding
	// Maximize toggles the open modal between its size and the whole
	// screen.
	Maximize key.Binding
	// Jump holds the keys of Panes, which it stands for in help.
	Jump key.Binding
	// Owner shows the page of the person or organization behind the
	// selection of the focused section, such as the author of a pull
	// request.
	Owner key.Binding
	// Repo shows the repository behind the selection, such as that of a
	// search result or of a row of the dashboard.
	Repo key.Binding
	// Dismiss dismisses an error toast, and stops a goto that waits for
	// GitHub. The app enables it while one shows or waits; what else it
	// does, clearing and closing, is up to the section or modal.
	Dismiss key.Binding
}

// forceQuit quits from anywhere, even while a modal or a section takes
// every key, so that nothing can trap the user.
var forceQuit = key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("^c", "quit"))

func newKeyMap(keys config.Keymap) KeyMap {
	k := KeyMap{
		Quit:          withForceQuit(ui.Binding(keys, config.ActionQuit, "quit")),
		Help:          ui.Binding(keys, config.ActionHelp, "help"),
		Search:        ui.Binding(keys, config.ActionSearch, "search"),
		History:       ui.Binding(keys, config.ActionHistory, "history"),
		Actions:       ui.Binding(keys, config.ActionActions, "actions"),
		FindFile:      ui.Binding(keys, config.ActionFindFile, "find file"),
		Command:       ui.Binding(keys, config.ActionCommand, "command"),
		form:          ui.FilterFormKeys(keys, "filter"),
		Notifications: ui.Binding(keys, config.ActionNotifications, "notifications"),
		Dashboard:     ui.Binding(keys, config.ActionDashboard, "dashboard"),
		Owner:         ui.Binding(keys, config.ActionOwner, "owner page"),
		Repo:          ui.Binding(keys, config.ActionRepo, "this repo"),
		Next:          ui.Binding(keys, config.ActionNextPane, "next pane"),
		Prev:          ui.Binding(keys, config.ActionPrevPane, "previous pane"),
		Zoom:          ui.Binding(keys, config.ActionZoom, "zoom"),
		Back:          ui.Binding(keys, config.ActionBack, "back"),
		Maximize:      ui.Binding(keys, config.ActionMaximize, "maximize"),
		Dismiss:       ui.Binding(keys, config.ActionDismiss, "dismiss"),
		Panes: []key.Binding{
			ui.Binding(keys, config.ActionPane1, "files"),
			ui.Binding(keys, config.ActionPane2, "pull requests"),
			ui.Binding(keys, config.ActionPane3, "issues"),
		},
	}
	k.Jump = ui.Jump(k.Panes...)
	return k
}

// withForceQuit returns b with ctrl+c, which always quits and which the
// config can't bind, so that the keys of quit, or of what cancels the
// command line, are all the keys that do so.
func withForceQuit(b key.Binding) key.Binding {
	help := b.Help().Key
	if !b.Enabled() {
		help = forceQuit.Help().Key
	}
	return keymap.Derive(key.NewBinding(key.WithKeys(append(slices.Clone(b.Keys()), forceQuit.Keys()...)...), key.WithHelp(help, b.Help().Desc)), b)
}

// ShortHelp implements help.KeyMap.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Dismiss, k.Back, k.Search, k.Command, k.FindFile, k.History, k.Actions, k.Notifications, k.Dashboard, k.Help, k.Quit,
	}
}

// FullHelp implements help.KeyMap: every key the app handles, in the
// order it matches them.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{append(k.globalKeys(), k.History, k.Actions)}
}

// globalKeys returns the keys that work on every screen, in the order the
// app matches them, and repoKeys those of the repository screen only.
func (k KeyMap) globalKeys() []key.Binding {
	return []key.Binding{
		k.Command, k.Quit, k.Help, k.Search, k.FindFile,
		k.Zoom, k.Maximize, k.Dismiss, k.Back, k.Owner, k.Repo, k.Notifications, k.Dashboard,
		k.Next, k.Prev, k.Jump,
	}
}

func (k KeyMap) repoKeys() []key.Binding { return []key.Binding{k.History, k.Actions} }

// layers returns the keys of the app as layers of keys, with the keys that
// work everywhere, and on the repository screen those of its own.
func (k KeyMap) layers(m *Model) []keyhelp.Layer {
	global := ui.ContextLayer(config.ContextGlobal, k.globalKeys(),
		[]key.Binding{k.Dismiss, k.Back, k.Search, k.Command, k.FindFile, k.Notifications, k.Dashboard, k.Help, k.Quit})
	if m.screen != repoScreen {
		return []keyhelp.Layer{global}
	}
	return []keyhelp.Layer{global, ui.ContextLayer("repo", k.repoKeys(), k.repoKeys())}
}

// state returns k as the app takes it now: the keys that open what the
// screen has, named for what they do there, and those of the panes where
// the app moves between them.
func (k KeyMap) state(m *Model) KeyMap {
	k.History.SetEnabled(k.History.Enabled() && m.canOpenHistory())
	k.Actions.SetEnabled(k.Actions.Enabled() && m.canOpenActions())
	k.FindFile.SetEnabled(k.FindFile.Enabled() && m.fileFinder() != nil)
	k.Owner.SetEnabled(k.Owner.Enabled() && m.selectedOwner() != "")
	k.Repo.SetEnabled(k.Repo.Enabled() && m.selectedRepo() != core.RepoRef{})
	k.Zoom.SetEnabled(k.Zoom.Enabled() && m.canZoom() && m.width >= narrowWidth)
	if m.canZoom() && m.zoomed() {
		k.Zoom.SetHelp(k.Zoom.Help().Key, "unzoom")
	}
	k.Back.SetEnabled(k.Back.Enabled() && m.canGoBack())
	k.Maximize.SetEnabled(k.Maximize.Enabled() && m.modal != nil)
	k.Dismiss.SetEnabled(k.Dismiss.Enabled() && (m.toast.Has(toast.Error) || m.going != nil))
	if m.going != nil && !m.toast.Has(toast.Error) {
		k.Dismiss.SetHelp(k.Dismiss.Help().Key, "cancel")
	}
	if m.screen != repoScreen {
		// Only the repository screen has panes to cycle through. On the
		// other screens the app leaves these keys to the section, which
		// does what it defines for them: the dashboard and the owner page
		// move between their panes, the search page between its query,
		// kinds and results, and the notifications have no use for them.
		k.Next.SetEnabled(false)
		k.Prev.SetEnabled(false)
	}
	// Showing what is on view does nothing, so help doesn't offer it.
	k.Dashboard.SetEnabled(k.Dashboard.Enabled() && m.dash != nil && m.screen != dashScreen)
	k.Notifications.SetEnabled(k.Notifications.Enabled() && m.screen != notifScreen)
	// Every other screen and modal focuses its own panes, and lists the
	// keys for them.
	k.Jump.SetEnabled(k.Jump.Enabled() && m.screen == repoScreen && len(m.panes) > 0)
	return k
}

// lineKeys returns the keys of the command line: those of its context.
// ctrl+c is not among them: it quits from the line as from everywhere.
func lineKeys(keys config.Keymap) cmdline.KeyMap {
	return cmdline.NewKeyMap(ui.Lookup(keys, "command_line"))
}

// pane returns the index of the pane that msg focuses, or -1.
func (k KeyMap) pane(msg tea.KeyPressMsg) int {
	for i, b := range k.Panes {
		if key.Matches(msg, b) {
			return i
		}
	}
	return -1
}
