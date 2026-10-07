// Package picker is a search popup in the manner of a command palette: a
// text input over a list of results that can be grouped by kind.
//
// Results come from a [Search] function that the picker calls in a command,
// a short while after the user stops typing, or from a fixed list of items
// that it filters itself with fuzzy matching. The user picks a result with
// enter, which sends a [ChosenMsg], or closes the picker with esc, which
// sends a [CancelMsg].
package picker

import (
	"context"
	"slices"
	"sync/atomic"
	"time"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Item is one result.
type Item struct {
	// Kind names the group the item is listed in, such as "Repositories".
	// Items of one kind are listed together under a header, and the kinds
	// in the order they first appear. It is also what a scope selects.
	Kind string
	// Title is the main text of the row, where the query's matches are
	// highlighted.
	Title string
	// Detail is shown after the title in a quieter style, and cut first
	// when the row is too narrow.
	Detail string
	// Value is the parent's own data, such as the repository the item
	// stands for. The picker hands it back in ChosenMsg.
	Value any
}

// Query is what the user asked for.
type Query struct {
	// Text is what the user typed.
	Text string
	// Scope is the kind the user limited the search to, one of those given
	// to WithScopes, or "" for every kind.
	Scope string
}

// Search returns the items that match q. The picker calls it in a command
// and cancels ctx when the query changes before it returns, so it may block.
// It is called for an empty query too, which lets the producer offer
// something to start from.
type Search func(ctx context.Context, q Query) ([]Item, error)

// DefaultDebounce is how long the picker waits after the last key before it
// searches.
const DefaultDebounce = 250 * time.Millisecond

var lastID atomic.Int64

// Model is a picker. Create one with [New]. It starts blurred, and the parent
// focuses it when it opens.
type Model struct {
	settings

	id     int64
	search Search
	// pool holds the fixed items, cleaned once for filtering.
	pool []result

	input textinput.Model
	spin  spinner.Model
	// spinning is whether a spinner tick is on its way.
	spinning bool

	// seq numbers queries. Results and debounce ticks carry the seq they
	// were started for and are dropped once it moved on; ctx is cancelled
	// then too.
	seq    int
	ctx    context.Context
	cancel context.CancelFunc

	// typing is whether the input has focus, in a picker with modes.
	typing bool
	// marks are the values the marked rows stand for.
	marks []any

	loading bool
	err     error
	// listed is what was found, and results is that with the typed item
	// after it, if there is one.
	listed  []result
	results []result
	typedAt bool
	rows    []row
	// itemRow[i] is the row of results[i].
	itemRow []int
	// scope is 0 for every kind, or 1 + the index of the kind in scopes.
	scope int
	sel   int
	top   int

	// Rendered once in SetStyles, so rows only copy them.
	gutterOn string
	prompt   string
	// frame is the frame's sides at the current width, rendered in layout.
	frame frame
	// lines caches the rendered rows, unselected, at the current width and
	// styles. Copies of the model share it, so it is replaced rather than
	// cleared when either changes.
	lines []string
	// view is rendered whenever the state changes, so View is free.
	view string
}

// New returns a blurred picker that searches with search. Pass a nil search
// and WithItems to filter a fixed list instead. With both, the items are
// listed while the query is empty, and search is called for any other.
// Return Init from the parent's Init, or the command of Reset when the
// picker opens again, to list the first results.
func New(search Search, opts ...Option) Model {
	s := defaultSettings()
	for _, opt := range opts {
		opt(&s)
	}
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = s.placeholder

	m := Model{
		settings: s,
		id:       lastID.Add(1),
		search:   search,
		input:    input,
		spin:     spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	m.pool, m.items = results(m.items), nil
	m.keys = m.withState(m.keys)
	m.ctx, m.cancel = context.WithCancel(m.parent)
	m.SetStyles(m.styles)
	// Init cannot change the model, so the first search is set up here.
	if m.wantsSearch(m.Query()) {
		m.loading, m.spinning = true, true
	} else {
		m.show(m.filter(m.Query()))
	}
	m.render()
	return m
}

// Init starts the first search, if the picker has a Search function.
func (m Model) Init() tea.Cmd {
	if !m.loading {
		return nil
	}
	return tea.Batch(m.searchCmd(), m.spin.Tick)
}

// Reset clears the query and the scope and lists the first results again,
// for example when the parent opens the picker again.
func (m *Model) Reset() tea.Cmd {
	m.input.SetValue("")
	m.scope = 0
	return m.refresh(false)
}

// ID returns the instance ID that scopes the picker's messages.
func (m Model) ID() int64 { return m.id }

// Query returns what the user asked for.
func (m Model) Query() Query {
	q := Query{Text: m.input.Value()}
	if m.scope > 0 {
		q.Scope = m.scopes[m.scope-1]
	}
	return q
}

// SetItems replaces the fixed list of items, and lists them again if they
// are what the picker shows. The picker keeps a copy.
func (m *Model) SetItems(items []Item) {
	m.pool, m.local = results(items), true
	if q := m.Query(); !m.wantsSearch(q) {
		m.show(m.filter(q))
		m.render()
	}
}

// Selected returns the selected item, or false if there is none.
func (m Model) Selected() (Item, bool) {
	if m.sel < 0 || m.sel >= len(m.results) {
		return Item{}, false
	}
	return m.results[m.sel].Item, true
}

// Len returns the number of results listed.
func (m Model) Len() int { return len(m.results) }

// Loading reports whether a search is waiting or running.
func (m Model) Loading() bool { return m.loading }

// Err returns the error of the last search, or nil.
func (m Model) Err() error { return m.err }

// Capturing reports whether the picker takes every key while it is open:
// letters go to the input. A picker with modes does so only while it types.
// A parent asks so it knows not to act on its own bindings meanwhile.
func (m Model) Capturing() bool { return m.typingMode() }

// Typing reports whether the picker is focused and its keys type. In a
// picker with modes that is insert mode, not normal mode.
func (m Model) Typing() bool { return m.focused && m.typingMode() }

// typingMode reports whether the picker types when it is focused.
func (m Model) typingMode() bool {
	if m.modes {
		return m.typing
	}
	return !m.noFilterLine
}

// SetMarked sets the values of the rows that are marked, which need not be
// in the list. The selection and the scroll stay.
func (m *Model) SetMarked(values []any) {
	m.marks = slices.Clone(values)
	m.lines = make([]string, len(m.rows))
	m.render()
}

// SetSize sets the width and height, frame included.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.layout()
}

// Width returns the width.
func (m Model) Width() int { return m.width }

// Height returns the height.
func (m Model) Height() int { return m.height }

// Focus focuses the picker so it takes keys.
// A picker with modes is in normal mode then.
func (m *Model) Focus() tea.Cmd {
	m.focused = true
	if m.modes {
		m.leaveTyping()
		return nil
	}
	cmd := m.input.Focus()
	m.render()
	return cmd
}

// Blur blurs the picker so it ignores keys. Searches in flight still land.
func (m *Model) Blur() {
	m.focused = false
	m.input.Blur()
	m.typing = false
	m.retype()
	m.render()
}

// Focused reports whether the picker takes keys.
func (m Model) Focused() bool { return m.focused }

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings. The scope keys are disabled when the
// picker has no scopes.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keys = m.withState(k)
}

// withState disables the bindings the picker has no use for: the scope
// keys without scopes, and the keys that focus the input without a filter
// line.
func (m Model) withState(k KeyMap) KeyMap {
	k.NextScope.SetEnabled(len(m.scopes) > 0)
	k.PrevScope.SetEnabled(len(m.scopes) > 0)
	if m.noFilterLine {
		k.Normal.Insert.SetEnabled(false)
		k.Normal.Append.SetEnabled(false)
	}
	return k
}

// ShortHelp implements help.KeyMap. In normal mode it lists the keys of
// that mode.
func (m Model) ShortHelp() []key.Binding {
	if m.modes && !m.typing {
		k := m.keys
		return []key.Binding{k.Normal.Up, k.Normal.Down, k.Normal.Insert, k.Choose, k.Cancel}
	}
	return m.keys.ShortHelp()
}

// FullHelp implements help.KeyMap. The keys of the mode the picker is in are
// enabled, and those of the other are not, so no two share a key.
func (m Model) FullHelp() [][]key.Binding {
	k := m.keys
	switch {
	case m.modes && !m.typing:
		for _, b := range []*key.Binding{&k.Up, &k.Down, &k.PageUp, &k.PageDown, &k.NextScope, &k.PrevScope} {
			b.SetEnabled(false)
		}
	default:
		for _, b := range k.Normal.Bindings() {
			b.SetEnabled(false)
		}
	}
	return k.fullHelp()
}

// wantsSearch reports whether q goes to the Search function rather than the
// fixed items.
func (m Model) wantsSearch(q Query) bool {
	return m.search != nil && (q.Text != "" || !m.local)
}

// refresh starts over for the current query: it drops what earlier queries
// still have in flight, then filters the items or searches, right away or
// after the debounce.
func (m *Model) refresh(debounce bool) tea.Cmd {
	m.newGeneration()
	q := m.Query()
	if !m.wantsSearch(q) {
		m.loading, m.err = false, nil
		m.show(m.filter(q))
		m.render()
		return nil
	}
	m.loading = true
	m.retype()
	cmd := m.searchCmd()
	if debounce && m.debounce > 0 && q.Text != "" {
		cmd = m.debounceCmd()
	}
	if !m.spinning {
		m.spinning = true
		cmd = tea.Batch(cmd, m.spin.Tick)
	}
	m.render()
	return cmd
}

// stop drops the search in flight, if any, and keeps what is listed.
func (m *Model) stop() {
	m.newGeneration()
	m.loading = false
	m.retype()
}

func (m *Model) newGeneration() {
	m.cancel()
	m.ctx, m.cancel = context.WithCancel(m.parent)
	m.seq++
}

func (m Model) searchCmd() tea.Cmd {
	search, ctx, q, id, seq := m.search, m.ctx, m.Query(), m.id, m.seq
	return func() tea.Msg {
		items, err := search(ctx, q)
		return resultMsg{id: id, seq: seq, text: q.Text, items: items, err: err}
	}
}

func (m Model) debounceCmd() tea.Cmd {
	id, seq := m.id, m.seq
	return tea.Tick(m.debounce, func(time.Time) tea.Msg {
		return debounceMsg{id: id, seq: seq}
	})
}
