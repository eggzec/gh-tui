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
	// Command opens the command line in place of the help line.
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
	// jump stands for Panes in the help.
	jump key.Binding
}

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
		k.jump = key.NewBinding(key.WithKeys(labels...), key.WithHelp(strings.Join(labels, "/"), "focus pane"))
	} else {
		k.jump = key.NewBinding(key.WithDisabled())
	}
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
