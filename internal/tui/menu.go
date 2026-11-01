package tui

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// menuWidth is the width of the menu, and menuAround the rows it needs
// besides its entries: the frame and the filter line.
const (
	menuWidth  = 44
	menuAround = 4
)

// menuEntry is one action of the menu: its name, the key it has now, and the
// action that choosing it presses.
type menuEntry struct {
	title, key, action string
}

// menuEntries returns the actions that layers offer on the selected item
// and that are enabled for it, in the order the layers list them.
func menuEntries(layers []keyhelp.Layer) []menuEntry {
	var out []menuEntry
	for i := range layers {
		for _, b := range layers[i].MenuItems() {
			if !b.Enabled() {
				continue
			}
			acts := keymap.Actions(b)
			if len(acts) == 0 {
				continue
			}
			out = append(out, menuEntry{title: b.Help().Desc, key: menuKey(b), action: acts[0]})
		}
	}
	return out
}

// menuKey returns the key b has now for the menu, or "" for an action that
// has none.
func menuKey(b key.Binding) string {
	if len(b.Keys()) == 0 {
		return ""
	}
	return b.Help().Key
}

// menuRunMsg asks the app to press the action that the menu closed on, if
// the selection is still the item it opened on.
type menuRunMsg struct {
	action string
	// sel is the selection the menu opened on, and had whether there was one.
	sel ui.Selection
	had bool
}

// menuModal lists the actions of the selected item, each with its key.
type menuModal struct {
	pick    picker.Model
	icons   ui.Icons
	rows    int
	sel     ui.Selection
	had     bool
	entries []menuEntry
}

func newMenuModal(entries []menuEntry, sel ui.Selection, had bool, keys picker.KeyMap, icons ui.Icons) *menuModal {
	items := make([]picker.Item, len(entries))
	for i, e := range entries {
		items[i] = picker.Item{Title: e.title, Detail: e.key, Value: e.action}
	}
	m := &menuModal{icons: icons, rows: len(entries), sel: sel, had: had, entries: entries}
	m.pick = picker.New(nil,
		picker.WithItems(items),
		picker.WithGroupHeaders(false),
		picker.WithModes(true),
		picker.WithFilterLine(false),
		picker.WithKeyMap(keys),
	)
	_ = m.pick.Focus()
	return m
}

// Title implements ui.Modal.
func (m *menuModal) Title() string { return "Menu" }

// Update implements ui.Modal. The menu closes before the action runs, so
// that the action meets the screen as the key would.
func (m *menuModal) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case picker.ChosenMsg:
		if msg.ID != m.pick.ID() {
			return nil
		}
		action, _ := msg.Item.Value.(string)
		run := menuRunMsg{action: action, sel: m.sel, had: m.had}
		// In order: a batch may deliver the run while the menu is still on
		// top, where the pressed action would be lost.
		return tea.Sequence(ui.LeaveModal(m), func() tea.Msg { return run })
	case picker.CancelMsg:
		if msg.ID != m.pick.ID() {
			return nil
		}
		return ui.LeaveModal(m)
	}
	var cmd tea.Cmd
	m.pick, cmd = m.pick.Update(msg)
	return cmd
}

// View implements ui.Modal.
func (m *menuModal) View() string { return m.pick.View() }

// SetSize implements ui.Modal.
func (m *menuModal) SetSize(width, height int) { m.pick.SetSize(width, height) }

// Fit implements ui.Fitter.
func (m *menuModal) Fit(maxWidth, maxHeight int) (width, height int) {
	return min(maxWidth, menuWidth), min(maxHeight, m.rows+menuAround)
}

// SetTheme implements ui.Modal.
func (m *menuModal) SetTheme(t ui.Theme) { m.pick.SetStyles(t.Picker(m.icons)) }

// Commands implements ui.Commanded: the menu takes only the commands that act on the app.
func (m *menuModal) Commands() []string { return nil }

// KeyLayers implements ui.Keyed.
func (m *menuModal) KeyLayers() []keyhelp.Layer {
	return []keyhelp.Layer{ui.ContextHelp("picker_normal", m.pick, m.pick.Capturing())}
}

// openMenu opens the menu of the actions the focused item has, or says that
// it has none.
func (m *Model) openMenu() tea.Cmd {
	layers, _ := m.innerLayers()
	entries := menuEntries(layers)
	if len(entries) == 0 {
		// With no item selected, there is no list to open a menu on; x is
		// then nothing but a key.
		if _, had := m.selection(); !had {
			return nil
		}
		return m.toast.Push(toast.Info, "Nothing to do with the selection.")
	}
	sel, had := m.selection()
	return m.open(newMenuModal(entries, sel, had, picker.NewKeyMap(ui.Lookup(m.cfg.Keys, "picker")), m.icons), nil)
}

// runMenu presses the action the menu chose, unless the selection moved
// to another item meanwhile.
func (m *Model) runMenu(msg menuRunMsg) tea.Cmd {
	if sel, had := m.selection(); sel != msg.sel || had != msg.had {
		return m.toast.Push(toast.Warning, "The selection changed; open the menu again.")
	}
	return m.pressAction(msg.action)
}
