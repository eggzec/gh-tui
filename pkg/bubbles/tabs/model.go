// Package tabs provides a one-line tab bar for switching between top-level
// sections. The active tab is marked with an accent bar in a thin rule that
// spans the full width.
package tabs

import (
	"slices"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"
)

var lastID atomic.Int64

func nextID() int { return int(lastID.Add(1)) }

// Model is the tab bar. Use [New] to create one.
type Model struct {
	id      int
	tabs    []string
	active  int
	width   int
	focused bool
	keys    KeyMap
	styles  Styles

	// labels holds each whole title rendered in both states, so switching
	// tabs only reassembles strings.
	labels []label
	// view is the rendered bar. It is rebuilt whenever something it depends
	// on changes, so View does no work.
	view string
}

// New returns a tab bar with the given options applied. It starts blurred,
// with the first tab active.
func New(opts ...Option) Model {
	m := Model{
		id:     nextID(),
		keys:   DefaultKeyMap(),
		styles: DefaultStyles(true),
	}
	for _, opt := range opts {
		opt(&m)
	}
	m.active = m.clamp(m.active)
	m.prepare()
	m.render()
	return m
}

// ID returns the instance ID carried by this tab bar's messages.
func (m Model) ID() int { return m.id }

// Tabs returns the tab titles.
func (m Model) Tabs() []string { return m.tabs }

// SetTabs replaces the tab titles. The active tab is kept if it still
// exists, and otherwise moves to the last tab.
func (m *Model) SetTabs(titles ...string) {
	m.tabs = slices.Clone(titles)
	m.active = m.clamp(m.active)
	m.prepare()
	m.render()
}

// Active returns the index of the active tab.
func (m Model) Active() int { return m.active }

// SetActive makes tab i active and returns a command that emits a
// [ChangeMsg]. It returns nil if i is out of range or already active.
func (m *Model) SetActive(i int) tea.Cmd {
	if i < 0 || i >= len(m.tabs) || i == m.active {
		return nil
	}
	m.active = i
	m.render()
	id := m.id
	return func() tea.Msg { return ChangeMsg{ID: id, Index: i} }
}

// Width returns the width the bar is rendered at.
func (m Model) Width() int { return m.width }

// SetWidth sets the width of the bar. With a width of zero or less the bar
// takes its natural width.
func (m *Model) SetWidth(width int) {
	if width == m.width {
		return
	}
	m.width = width
	m.render()
}

// Focus makes the tab bar react to keys.
func (m *Model) Focus() { m.focused = true }

// Blur stops the tab bar from reacting to keys.
func (m *Model) Blur() { m.focused = false }

// Focused reports whether the tab bar reacts to keys.
func (m Model) Focused() bool { return m.focused }

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap replaces the key bindings.
func (m *Model) SetKeyMap(k KeyMap) { m.keys = k }

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }

// SetStyles replaces the styles.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	m.prepare()
	m.render()
}

func (m Model) clamp(i int) int {
	return max(0, min(i, len(m.tabs)-1))
}
