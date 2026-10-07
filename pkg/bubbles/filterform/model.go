// Package filterform is a form for the filters and sort of a list, kept in
// sync with the GitHub query they stand for.
//
// The parent declares the fields in a [Spec]. Each field writes and claims
// search qualifiers, so the form shows the equivalent query on its last
// line: editing a field rewrites the query, and editing the query sets the
// fields. Words no field claims are kept as free text. A spec with a sort
// shows it on a tab of its own, after the filters; ] and [ switch tabs.
// The user applies the form, both tabs at once, with enter, which sends an
// [AppliedMsg], or closes it with esc, which sends a [CancelMsg].
package filterform

import (
	"context"
	"slices"
	"sync/atomic"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

var lastID atomic.Int64

// loadState is where the options of a remote field are.
type loadState int

const (
	notLoaded loadState = iota
	loading
	loaded
	failed
)

// field is what the form keeps of a field besides its value.
type field struct {
	state loadState
	err   error
	seq   int
	// items are the options Load returned.
	items []Item
}

// mode is what the form takes keys for.
type mode int

const (
	// rowsMode moves between the rows and changes their values. Letters
	// are keys.
	rowsMode mode = iota
	// insertMode types in a Text field or the query line.
	insertMode
	// listMode has a dropdown open on the row in focus, or waits for the
	// options it lists, or shows why they failed to load.
	listMode
)

// Model is a filter form. Create one with [New]. It starts blurred, and
// the parent focuses it when it opens.
type Model struct {
	settings

	id     int64
	spec   Spec
	state  state
	fields []field

	// tab is the tab on view, and row the row in focus on it: a field, or
	// on the Sort tab what is sorted by and the order, then the query
	// line.
	tab Tab
	row int
	// mode is what keys do. before is the value a Text field had when it
	// was opened for typing, which leaving it puts back when it doesn't
	// keep the text. picking is whether the dropdown's picker is open,
	// rather than waiting for its options.
	// dropItems and dropWidth are the number of options it opened with and
	// the width they want, and toggled the value of the checklist item
	// that space or enter acted on last, so that enter doesn't undo it.
	mode      mode
	before    Value
	picking   bool
	pick      picker.Model
	dropItems int
	dropWidth int
	toggled   string

	text  textinput.Model
	query textinput.Model
	spin  spinner.Model
	// spinning is whether a spinner tick is on its way.
	spinning bool
	focused  bool

	ctx    context.Context
	cancel context.CancelFunc
	seq    int

	// top is the first line of the rows shown, kept between renders so the
	// rows don't jump.
	top    int
	glyphs glyphs
	cache  renderCache
	// lw caches the label column's width for a width of the form.
	lw struct {
		label, width int
		set          bool
	}
	// view is rendered whenever the state changes, so View is free.
	view string
}

// New returns a blurred form for spec, at its defaults or the query given
// with WithQuery. The form keeps a copy of spec.
func New(spec Spec, opts ...Option) Model {
	s := defaultSettings()
	for _, opt := range opts {
		opt(&s)
	}
	m := Model{
		settings: s,
		id:       lastID.Add(1),
		spec:     spec.clone(),
		text:     newInput(),
		query:    newInput(),
		spin:     spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	m.query.Placeholder = "Type a query, or choose above"
	m.fields = make([]field, len(m.spec.Fields))
	if m.tabbed() && s.tab == SortTab {
		m.tab = SortTab
	}
	m.ctx, m.cancel = context.WithCancel(m.parent)
	if s.hasQuery {
		m.state = parse(&m.spec, s.query)
	} else {
		m.state = defaults(&m.spec)
	}
	m.syncQuery()
	m.SetStyles(m.styles)
	return m
}

func newInput() textinput.Model {
	in := textinput.New()
	in.Prompt = ""
	return in
}

// Init does nothing: a remote field loads its options when it is opened.
func (m Model) Init() tea.Cmd { return nil }

// ID returns the instance ID that scopes the form's messages.
func (m Model) ID() int64 { return m.id }

// Query returns the GitHub query the fields make, followed by the free
// text.
func (m Model) Query() string { return m.state.query(&m.spec) }

// SetQuery sets the fields from q: each token goes to the first field that
// claims it, and the rest is kept as free text. Fields q doesn't mention
// are left empty. It leaves insert mode and closes an open dropdown.
func (m *Model) SetQuery(q string) {
	m.closeEditor(false)
	m.state = parse(&m.spec, q)
	m.fields = slices.Clone(m.fields)
	m.syncQuery()
	m.render()
}

// Reset puts every field and the sort back to their defaults and drops the
// free text, on both tabs. It leaves insert mode and closes an open
// dropdown.
func (m *Model) Reset() {
	m.closeEditor(false)
	m.state = defaults(&m.spec)
	m.fields = slices.Clone(m.fields)
	m.syncQuery()
	m.render()
}

// Values returns each field's value by key.
func (m Model) Values() map[string]Value {
	out := make(map[string]Value, len(m.spec.Fields))
	for i := range m.spec.Fields {
		out[m.spec.Fields[i].Key] = m.state.values[i].clone()
	}
	return out
}

// Value returns the value of the field with the key k, or false if there
// is none.
func (m Model) Value(k string) (Value, bool) {
	i := m.fieldIndex(k)
	if i < 0 {
		return Value{}, false
	}
	return m.state.values[i].clone(), true
}

// Sort returns the sort, or the zero Sort for a form without one.
func (m Model) Sort() Sort { return m.state.sort }

// FreeText returns the words of the query that no field claims.
func (m Model) FreeText() []string { return slices.Clone(m.state.free) }

func (m Model) fieldIndex(k string) int {
	return slices.IndexFunc(m.spec.Fields, func(f Field) bool { return f.Key == k })
}

// Capturing reports whether the form takes every key, which it does in
// insert mode and while a dropdown's filter is typed in, since letters are
// typed there. A dropdown in its normal mode takes only its keys. A parent
// asks so it knows not to act on its own bindings meanwhile.
func (m Model) Capturing() bool {
	switch {
	case !m.focused:
		return false
	case m.mode == insertMode:
		return true
	}
	return m.mode == listMode && m.picking && m.pick.Typing()
}

// Loading reports whether any field is loading its options.
func (m Model) Loading() bool {
	return slices.ContainsFunc(m.fields, func(f field) bool { return f.state == loading })
}

// SetSize sets the width and height.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.render()
}

// Width returns the width.
func (m Model) Width() int { return m.width }

// Height returns the height.
func (m Model) Height() int { return m.height }

// Focus focuses the form so it takes keys.
func (m *Model) Focus() tea.Cmd {
	m.focused = true
	m.render()
	return nil
}

// Blur blurs the form so it ignores keys. It leaves insert mode and closes
// an open dropdown, keeping what was typed or chosen, and cancels the loads
// in flight; they start again when their fields are opened.
func (m *Model) Blur() {
	m.focused = false
	m.closeEditor(true)
	m.query.Blur()
	m.cancel()
	m.ctx, m.cancel = context.WithCancel(m.parent)
	m.fields = slices.Clone(m.fields)
	for i := range m.fields {
		if m.fields[i].state == loading {
			m.fields[i].state = notLoaded
		}
	}
	m.syncQuery()
	m.render()
}

// Focused reports whether the form takes keys.
func (m Model) Focused() bool { return m.focused }

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keys = k
	if m.picking {
		m.pick.SetKeyMap(m.listKeyMap())
	}
	m.render()
}

// queryRow returns the row of the query line, the last one of the tab.
func (m *Model) queryRow() int {
	if m.tab == SortTab {
		return sortRows
	}
	return len(m.spec.Fields)
}

// kind returns the kind of the field in focus, or -1 on the Sort tab and
// the query line.
func (m *Model) kind() Kind {
	if m.tab == FiltersTab && m.row < len(m.spec.Fields) {
		return m.spec.Fields[m.row].Kind
	}
	return -1
}

// items returns the options of field i: its own, then those it loaded.
func (m *Model) items(i int) []Item {
	f := &m.spec.Fields[i]
	if len(m.fields[i].items) == 0 {
		return f.Options
	}
	return append(slices.Clip(f.Options), m.fields[i].items...)
}

// syncQuery shows the query in the query line, unless the user is typing
// there: the line follows the fields, but not while it sets them.
func (m *Model) syncQuery() {
	if m.query.Focused() {
		return
	}
	m.query.SetValue(m.Query())
	m.query.CursorEnd()
}

// setValue sets field i's value, for a key or an editor.
func (m *Model) setValue(i int, v Value) {
	m.state.values = slices.Clone(m.state.values)
	m.state.values[i] = v
	m.syncQuery()
}

func (m Model) applied() AppliedMsg {
	return AppliedMsg{ID: m.id, Values: m.Values(), Sort: m.state.sort, Query: m.Query()}
}
