package keyhelp

import (
	"slices"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

var lastID atomic.Int64

// Model is the content of a help overlay: a title, a query line, and every
// binding of its layers, grouped by layer, with its state. Create one with
// [New]. It starts blurred, and the parent focuses it when it opens.
type Model struct {
	settings

	id    int64
	rows  []Row
	input textinput.Model
	// hay holds the text of each row that the query matches.
	hay []string
	// actions holds the actions of each row, such as "pulls.merge".
	actions [][]string
	// capturing is whether the next key filters by key, and key the key
	// that does.
	capturing bool
	key       string
	// shown holds the indexes of the rows that match the query and key.
	shown []int
	vp    viewport.Model
	// prompt is rendered once in SetStyles.
	prompt  string
	focused bool
	// view is rendered whenever the state changes, so View is free.
	view string
	// drawn holds the lines of each row, the row and what it lost,
	// rendered at drawnWidth; a nil entry isn't rendered yet. A row is
	// rendered once per width, layers and styles, however the query
	// changes what is shown. Copies share it, which is safe: a change of
	// any of those gives a model a new one, and an entry is the same
	// whichever copy renders it.
	drawn      [][]string
	drawnWidth int
	// stale is set when a change came while the help was blurred: nobody
	// sees a closed help, so it lists the rows again only once focused or
	// viewed.
	stale bool
}

// New returns a blurred help.
func New(opts ...Option) Model {
	s := defaultSettings()
	for _, opt := range opts {
		opt(&s)
	}
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = s.placeholder
	m := Model{settings: s, id: lastID.Add(1), input: input, vp: viewport.New()}
	m.analyze()
	m.SetStyles(m.styles)
	m.refilter()
	return m
}

// Init implements tea.Model. The help needs no command to start.
func (m Model) Init() tea.Cmd { return nil }

// ID returns the instance ID that scopes the help's messages.
func (m Model) ID() int64 { return m.id }

// Rows returns every row, whether the query matches it or not.
func (m Model) Rows() []Row { return slices.Clone(m.rows) }

// Shown returns the rows that match the query and the captured key, in
// the order they are listed.
func (m Model) Shown() []Row {
	out := make([]Row, len(m.shown))
	for i, r := range m.shown {
		out[i] = m.rows[r]
	}
	return out
}

// SetLayers sets the layers to list, and keeps the query. The help keeps a
// copy.
func (m *Model) SetLayers(layers []Layer) {
	m.layers = slices.Clone(layers)
	m.analyze()
	m.refilter()
}

// Title returns the title.
func (m Model) Title() string { return m.title }

// SetTitle sets the title.
func (m *Model) SetTitle(title string) {
	m.title = title
	m.render()
}

// Query returns what the user typed.
func (m Model) Query() string { return m.input.Value() }

// SetQuery sets the query as if the user had typed it.
func (m *Model) SetQuery(q string) {
	m.input.SetValue(q)
	m.refilter()
}

// Key returns the key that filters the list, or "".
func (m Model) Key() string { return m.key }

// Capturing reports whether the next key filters by key rather than acts.
func (m Model) Capturing() bool { return m.capturing }

// Reset clears the query and the captured key and scrolls to the top, for
// example when the parent opens the help again.
func (m *Model) Reset() {
	m.input.Reset()
	m.capturing, m.key = false, ""
	m.refilter()
}

// SetSize sets the width and height.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.list()
}

// Width returns the width.
func (m Model) Width() int { return m.width }

// Height returns the height.
func (m Model) Height() int { return m.height }

// Focus focuses the help so it takes keys.
func (m *Model) Focus() tea.Cmd {
	m.focused = true
	cmd := m.input.Focus()
	if m.stale {
		m.relist()
	} else {
		m.render()
	}
	return cmd
}

// Blur blurs the help so it ignores keys.
func (m *Model) Blur() {
	m.focused = false
	m.input.Blur()
	m.render()
}

// Focused reports whether the help takes keys.
func (m Model) Focused() bool { return m.focused }

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) { m.keys = k }

// ShortHelp implements help.KeyMap.
func (m Model) ShortHelp() []key.Binding { return m.keysNow().ShortHelp() }

// FullHelp implements help.KeyMap.
func (m Model) FullHelp() [][]key.Binding { return m.keysNow().FullHelp() }

// keysNow returns the key bindings with the state applied: while a key is
// captured only Capture acts, and Close is typed into a query.
func (m Model) keysNow() KeyMap {
	k := m.keys
	if m.capturing {
		for _, b := range []*key.Binding{&k.Up, &k.Down, &k.PageUp, &k.PageDown, &k.Home, &k.End, &k.Back, &k.Close} {
			b.SetEnabled(false)
		}
	}
	if m.filtered() {
		k.Close.SetEnabled(false)
	}
	return k
}

// filtered reports whether a query or a key filters the list.
func (m Model) filtered() bool { return m.input.Value() != "" || m.key != "" }

// analyze finds the rows of the layers and the text the query matches in
// each.
func (m *Model) analyze() {
	m.rows = Analyze(m.layers)
	m.hay = make([]string, len(m.rows))
	m.actions = m.actions[:0]
	for _, l := range m.layers {
		for i := range l.Bindings {
			var a []string
			if i < len(l.Actions) {
				a = l.Actions[i]
			}
			m.actions = append(m.actions, a)
		}
	}
	for i, r := range m.rows {
		h := r.Binding.Help()
		m.hay[i] = strings.Join([]string{h.Desc, r.Source, strings.Join(r.Binding.Keys(), " "), h.Key}, " ")
	}
	m.drawn = nil
}
