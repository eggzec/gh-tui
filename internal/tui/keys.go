package tui

import (
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
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
	// Filter opens the filter modal of the focused pane on its Filters
	// tab, if it has one, and Sort opens it on its Sort tab, if the pane
	// can be sorted.
	Filter key.Binding
	Sort   key.Binding
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
	// every pane again, and Back shows them again too.
	Zoom key.Binding
	Back key.Binding
	// Jump holds the keys of Panes, which it stands for in help.
	Jump key.Binding
	// Dismiss closes the newest toast. It is the toasts' own key, which
	// the app matches, and which they enable while they show.
	Dismiss key.Binding
}

// forceQuit quits from anywhere, even while a modal or a section takes
// every key, so that nothing can trap the user.
var forceQuit = key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("^c", "quit"))

func newKeyMap(keys map[string][]string) KeyMap {
	k := KeyMap{
		Quit:          ui.Binding(keys, config.ActionQuit, "quit"),
		Help:          ui.Binding(keys, config.ActionHelp, "help"),
		Search:        ui.Binding(keys, config.ActionSearch, "search"),
		History:       ui.Binding(keys, config.ActionHistory, "history"),
		Actions:       ui.Binding(keys, config.ActionActions, "actions"),
		FindFile:      ui.Binding(keys, config.ActionFindFile, "find file"),
		Command:       ui.Binding(keys, config.ActionCommand, "command"),
		Filter:        ui.Binding(keys, config.ActionFilter, "filter"),
		Sort:          ui.Binding(keys, config.ActionSort, "sort"),
		form:          ui.FilterFormKeys(keys),
		Notifications: ui.Binding(keys, config.ActionNotifications, "notifications"),
		Dashboard:     ui.Binding(keys, config.ActionDashboard, "dashboard"),
		Next:          ui.Binding(keys, config.ActionNextTab, "next pane"),
		Prev:          ui.Binding(keys, config.ActionPrevTab, "previous pane"),
		Zoom:          ui.Binding(keys, config.ActionZoom, "zoom"),
		Back:          ui.Binding(keys, config.ActionBack, "unzoom"),
		Dismiss:       toast.DefaultKeyMap().Dismiss,
		Panes: []key.Binding{
			ui.Binding(keys, config.ActionPane1, "files"),
			ui.Binding(keys, config.ActionPane2, "pull requests"),
			ui.Binding(keys, config.ActionPane3, "issues"),
		},
	}
	labels := make([]string, 0, len(k.Panes))
	for _, b := range k.Panes {
		if b.Enabled() {
			labels = append(labels, b.Help().Key)
		}
	}
	if len(labels) > 0 {
		k.Jump = key.NewBinding(key.WithKeys(labels...), key.WithHelp(strings.Join(labels, "/"), "focus pane"))
	} else {
		k.Jump = key.NewBinding(key.WithDisabled())
	}
	return k
}

// ShortHelp implements help.KeyMap. The way out of a zoom comes first.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{
		k.Back, k.Search, k.Command, k.FindFile, k.History, k.Actions, k.Notifications, k.Dashboard, k.Help, k.Quit,
	}
}

// FullHelp implements help.KeyMap: every key the app handles, in the
// order it matches them.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{
			k.Command, k.Quit, k.Help, k.Search, k.History, k.Actions, k.FindFile, k.Filter, k.Sort,
			k.Zoom, k.Back, k.Dismiss, k.Notifications, k.Dashboard,
		},
		{k.Next, k.Prev, k.Jump},
	}
}

// state returns k as the app takes it now: the keys that open what the
// screen has, named for what they do there, and those of the panes where
// the app moves between them.
func (k KeyMap) state(m *Model) KeyMap {
	p := m.focused()
	k.History.SetEnabled(k.History.Enabled() && m.canOpenHistory())
	k.Actions.SetEnabled(k.Actions.Enabled() && m.canOpenActions())
	k.FindFile.SetEnabled(k.FindFile.Enabled() && m.fileFinder() != nil)
	filters, sorts := false, false
	if p != nil {
		_, _, filters = filterOf(p.section, filterform.FiltersTab)
		_, _, sorts = filterOf(p.section, filterform.SortTab)
	}
	k.Filter.SetEnabled(k.Filter.Enabled() && filters)
	k.Sort.SetEnabled(k.Sort.Enabled() && sorts)
	k.Zoom.SetEnabled(k.Zoom.Enabled() && m.canZoom() && m.width >= narrowWidth)
	k.Back.SetEnabled(k.Back.Enabled() && m.canZoom() && m.zoomed())
	k.Dismiss = m.toast.KeyMap().Dismiss
	switch m.screen {
	case notifScreen:
		k.Notifications.SetHelp(k.Notifications.Help().Key, "back")
	case dashScreen:
		k.Dashboard.SetHelp(k.Dashboard.Help().Key, "back")
		k.Dashboard.SetEnabled(k.Dashboard.Enabled() && m.back != dashScreen)
		// The dashboard moves between its own panes.
		k.Jump.SetEnabled(false)
	case repoScreen, searchScreen:
	}
	if m.screen != repoScreen {
		// Only the repository screen has panes to cycle through, so the
		// help offers these keys to the section. The app still takes them
		// on the notifications and search screens, where they do nothing:
		// passing them on would change what tab, [ and ] do on the search
		// page.
		k.Next.SetEnabled(false)
		k.Prev.SetEnabled(false)
	}
	k.Dashboard.SetEnabled(k.Dashboard.Enabled() && m.dash != nil)
	k.Jump.SetEnabled(k.Jump.Enabled() && len(m.panes) > 0)
	return k
}

// lineKeys returns the keys of the command line. Enter runs the line and
// esc and ctrl+c cancel it, whatever the config says, and the keys of the
// config's select and back actions do too, unless the line needs them
// otherwise: to type, or to edit, move, complete or recall. So a letter
// bound to back still types itself, and backspace still deletes.
func lineKeys(keys map[string][]string) cmdline.KeyMap {
	k := cmdline.DefaultKeyMap()
	taken := lineOwnKeys(k)
	k.Submit = withKeys(k.Submit, keys[config.ActionSelect], taken)
	k.Cancel = withKeys(k.Cancel, keys[config.ActionBack], taken)
	return k
}

// lineOwnKeys returns the keys the command line uses for itself: those of
// its own key map and those of the text input it edits with.
func lineOwnKeys(k cmdline.KeyMap) []string {
	ti := textinput.DefaultKeyMap()
	var out []string
	for _, b := range []key.Binding{
		k.Submit, k.Cancel, k.CancelEmpty, k.Next, k.Prev, k.Older, k.Newer,
		ti.CharacterForward, ti.CharacterBackward, ti.WordForward, ti.WordBackward,
		ti.DeleteWordBackward, ti.DeleteWordForward, ti.DeleteAfterCursor, ti.DeleteBeforeCursor,
		ti.DeleteCharacterBackward, ti.DeleteCharacterForward, ti.LineStart, ti.LineEnd,
		ti.Paste, ti.AcceptSuggestion, ti.NextSuggestion, ti.PrevSuggestion,
	} {
		out = append(out, b.Keys()...)
	}
	return out
}

// withKeys returns b with the keys of more that type nothing and aren't
// taken.
func withKeys(b key.Binding, more, taken []string) key.Binding {
	keys := b.Keys()
	for _, k := range more {
		if k == "space" || utf8.RuneCountInString(k) == 1 || slices.Contains(taken, k) || slices.Contains(keys, k) {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == len(b.Keys()) {
		return b
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(b.Help().Key, b.Help().Desc))
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
