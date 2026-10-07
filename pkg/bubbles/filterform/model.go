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

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
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
	// chip is the chip under the cursor of a Multi, or its length for
	// "+ add".
	chip int
}

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
	// editing is whether the row's editor is open; before is the value it
	// opened with, which esc puts back, and toggled whether space chose
	// something since.
	editing bool
	before  Value
	toggled bool
	picking bool
	pick    picker.Model

	text  textinput.Model
	query textinput.Model
	spin  spinner.Model
	// spinning is whether a spinner tick is on its way.
	spinning bool
	help     help.Model
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
		help:     help.New(),
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
	m.resetChips()
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
// are left empty. It closes an open editor.
func (m *Model) SetQuery(q string) {
	m.closeEditor(false)
	m.state = parse(&m.spec, q)
	m.fields = slices.Clone(m.fields)
	m.resetChips()
	m.syncQuery()
	m.render()
}

// Reset puts every field and the sort back to their defaults and drops the
// free text, on both tabs. It closes an open editor.
func (m *Model) Reset() {
	m.closeEditor(false)
	m.state = defaults(&m.spec)
	m.fields = slices.Clone(m.fields)
	m.resetChips()
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

// Capturing reports whether the form takes every key, which it does while
// an editor or the query line has focus, since letters are typed there. A
// parent asks so it knows not to act on its own bindings meanwhile.
func (m Model) Capturing() bool {
	return m.focused && (m.editing || m.row == m.queryRow())
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
	var cmd tea.Cmd
	if m.row == m.queryRow() {
		cmd = m.query.Focus()
	}
	m.render()
	return cmd
}

// Blur blurs the form so it ignores keys. It closes an open editor,
// keeping what was chosen, and cancels the loads in flight; they start
// again when their fields are opened.
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
		m.pick.SetKeyMap(k.Picker)
	}
	m.render()
}

// ShortHelp implements help.KeyMap. It lists the keys that act on the row
// in focus.
func (m Model) ShortHelp() []key.Binding { return m.shortHelp() }

func (m *Model) shortHelp() []key.Binding {
	k := m.keys
	switch {
	case m.editing && m.picking && m.kind() == Multi:
		return []key.Binding{relabel(k.Toggle, "choose"), relabel(k.Edit, "done"), relabel(k.Cancel, "back")}
	case m.editing && !m.picking && m.kind() != Text:
		if m.fields[m.row].state == failed {
			return []key.Binding{relabel(k.Edit, "retry"), relabel(k.Cancel, "back")}
		}
		return []key.Binding{relabel(k.Cancel, "back")}
	case m.editing:
		return []key.Binding{relabel(k.Edit, "done"), relabel(k.Cancel, "back")}
	case m.row == m.queryRow():
		return []key.Binding{k.Down, k.Apply, relabel(k.Cancel, "cancel")}
	}
	// The key to the other tab comes last, since the tabs show already.
	var out []key.Binding
	switch m.kind() {
	case Multi, Person, Text:
		out = []key.Binding{k.Down, k.Edit, k.Clear, k.Cancel, m.tabHelp()}
	default:
		right := k.Right
		right.SetHelp(right.Help().Key+"/"+k.Toggle.Help().Key, right.Help().Desc)
		out = []key.Binding{k.Down, right, k.Apply, k.Cancel, m.tabHelp()}
	}
	if !m.tabbed() {
		out = out[:len(out)-1]
	}
	return out
}

// tabHelp returns the key to the other tab, named after it.
func (m *Model) tabHelp() key.Binding {
	return relabel(m.keys.NextTab, tabHelpDescs[(m.tab+1)%numTabs])
}

// FullHelp implements help.KeyMap. It enables the keys that act on the
// row in focus: an open picker takes Edit, Toggle, Cancel and its own
// moves, another editor Edit and Cancel, and the query line the moves,
// Apply and Cancel; the rest are typed. A row takes enter to open its
// editor if it has one, and to apply if not. The form's Edit and Cancel
// stand in for the picker's Choose and Cancel.
func (m Model) FullHelp() [][]key.Binding {
	k := m.keys
	if m.picking {
		k.Picker = m.pick.KeyMap()
	}
	form := []*key.Binding{&k.NextTab, &k.PrevTab, &k.Up, &k.Down, &k.Left, &k.Right, &k.Toggle, &k.Edit, &k.Apply, &k.Clear, &k.Cancel}
	pick := []*key.Binding{&k.Picker.Up, &k.Picker.Down, &k.Picker.PageUp, &k.Picker.PageDown, &k.Picker.NextScope, &k.Picker.PrevScope}
	var on []*key.Binding
	switch {
	case m.picking:
		on = append([]*key.Binding{&k.Edit, &k.Toggle, &k.Cancel}, pick...)
	case m.editing && (m.kind() == Text || m.fields[m.row].state == failed):
		on = []*key.Binding{&k.Edit, &k.Cancel}
	case m.editing:
		on = []*key.Binding{&k.Cancel}
	case m.row == m.queryRow():
		on = []*key.Binding{&k.Up, &k.Down, &k.Apply, &k.Cancel}
	default:
		on = slices.DeleteFunc(slices.Clone(form), func(b *key.Binding) bool {
			if b == &k.NextTab || b == &k.PrevTab {
				return !m.tabbed()
			}
			return b == &k.Edit && !m.hasEditor() || b == &k.Apply && m.hasEditor()
		})
	}
	if !m.picking && !m.editing && !m.canClear() {
		k.Clear.SetEnabled(false)
	}
	for _, b := range slices.Concat(form, pick, []*key.Binding{&k.Picker.Choose, &k.Picker.Cancel}) {
		if !slices.Contains(on, b) {
			b.SetEnabled(false)
		}
	}
	return k.FullHelp()
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

// resetChips puts each Multi's chip cursor on "+ add".
func (m *Model) resetChips() {
	for i := range m.fields {
		m.fields[i].chip = len(m.state.values[i].list)
	}
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
	m.fields[i].chip = min(m.fields[i].chip, len(v.list))
	m.syncQuery()
}

func (m Model) applied() AppliedMsg {
	return AppliedMsg{ID: m.id, Values: m.Values(), Sort: m.state.sort, Query: m.Query()}
}
