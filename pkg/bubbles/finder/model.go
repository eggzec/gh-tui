// Package finder is a fuzzy finder over a list of paths, in the manner of
// telescope's find_files or the file finder of github.com: a text input
// over the paths that match what is typed, best first, with the matched
// characters marked.
//
// The paths come from a [Load] function that the finder calls in a command
// when it starts. Matching ranks a match in the file name above one in the
// directories, a match at the start of a word or run of consecutive
// characters above scattered ones, and shorter paths above longer ones.
// A query that extends the last one looks only through its matches, and a
// large list is matched in a command, so typing never blocks. The user
// picks a path with enter, which sends a [ChosenMsg], or closes the finder
// with esc, which sends a [CancelMsg].
package finder

import (
	"context"
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
)

// Item is one path the finder searches.
type Item struct {
	// Path is what the query matches, such as the path of a file from the
	// root of a repository. Directories are separated by slashes.
	Path string
	// Detail is shown dimmed at the right edge of the row, such as the
	// size of a file. It is dropped when the row is too narrow.
	Detail string
	// Value is the parent's own data, such as the file the path stands
	// for. The finder hands it back in ChosenMsg and Selected.
	Value any
}

// Listing is what a Load returns.
type Listing struct {
	// Items are the paths to search. The finder keeps the slice, so the
	// producer must not change it afterwards.
	Items []Item
	// Note is shown in the status line, such as that the list is
	// incomplete.
	Note string
}

// Load returns the paths to search. The finder calls it once, in a
// command, and cancels ctx if it is closed for good before Load returns,
// so it may block.
type Load func(ctx context.Context) (Listing, error)

var lastID atomic.Int64

// Model is a finder. Create one with [New]. It starts blurred, and the
// parent focuses it when it opens.
type Model struct {
	settings

	id     int64
	load   Load
	ctx    context.Context
	cancel context.CancelFunc

	input textinput.Model
	spin  spinner.Model
	// spinning is whether a spinner tick is on its way.
	spinning bool

	loading bool
	err     error
	note    string
	corpus  *corpus
	// recent holds the rank of each recent item, 0 for the most recent.
	recent map[int32]int

	// res is the result shown, of the query it names. seq numbers the
	// queries; a match in a command carries the seq it was started for,
	// and is dropped once seq moved on. stop cancels it then.
	res      *result
	seq      int
	matching bool
	stop     context.CancelFunc

	sel, top int

	// Rendered once in SetStyles, so rows only copy them.
	esc esc
	// rows caches the rendered rows of res that aren't selected, by item,
	// at the current width and styles. It is replaced, not cleared, when
	// any of them changes, since copies of the model share it.
	rows map[int32]string
	// view is rendered whenever the state changes, so View is free.
	view string
}

// New returns a blurred finder over the paths that load returns. Return
// Init from the parent's Init, or where it opens the finder, to load them.
func New(load Load, opts ...Option) Model {
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
		load:     load,
		input:    input,
		spin:     spinner.New(spinner.WithSpinner(spinner.Dot)),
		loading:  true,
		// Init sends the first tick, and it cannot record that itself.
		spinning: true,
		stop:     func() {},
	}
	m.ctx, m.cancel = context.WithCancel(s.parent)
	m.SetStyles(m.styles)
	m.layout()
	return m
}

// Init loads the paths and starts the spinner.
func (m Model) Init() tea.Cmd {
	if !m.loading {
		return nil
	}
	return tea.Batch(m.loadCmd(), m.spin.Tick)
}

// Retry loads the paths again once their load failed, such as once the
// network is back. It returns nil if it didn't fail.
func (m *Model) Retry() tea.Cmd {
	if m.err == nil || m.loading {
		return nil
	}
	m.loading, m.err = true, nil
	m.render()
	return tea.Batch(m.loadCmd(), m.tick())
}

func (m Model) loadCmd() tea.Cmd {
	load, ctx, id := m.load, m.ctx, m.id
	return func() tea.Msg {
		l, err := load(ctx)
		if err != nil {
			return loadedMsg{id: id, err: err}
		}
		c, err := newCorpus(ctx, l.Items)
		if err != nil {
			return loadedMsg{id: id, err: err}
		}
		return loadedMsg{id: id, corpus: c, note: l.Note}
	}
}

// Close cancels the load and any match in flight, for a finder that is
// closed for good.
func (m *Model) Close() {
	m.cancel()
	m.stop()
}

// ID returns the instance ID that scopes the finder's messages.
func (m Model) ID() int64 { return m.id }

// Query returns what the user typed.
func (m Model) Query() string { return m.input.Value() }

// SetQuery replaces what the user typed and matches it.
func (m *Model) SetQuery(q string) tea.Cmd {
	m.input.SetValue(q)
	m.input.CursorEnd()
	return m.match()
}

// Reset clears the query and ranks the recent paths first, for a finder
// that opens again. The paths are those of WithRecent, most recent first.
func (m *Model) Reset(recent []string) tea.Cmd {
	m.recentPaths = recent[:min(len(recent), maxRecent):min(len(recent), maxRecent)]
	if m.corpus != nil {
		m.recent = m.corpus.ranks(m.recentPaths)
	}
	m.input.SetValue("")
	// The recent paths changed, so no earlier result stands.
	m.res = nil
	return m.match()
}

// Loading reports whether the paths are still loading.
func (m Model) Loading() bool { return m.loading }

// Err returns the error of the load, if it failed.
func (m Model) Err() error { return m.err }

// Total returns the number of paths searched.
func (m Model) Total() int {
	if m.corpus == nil {
		return 0
	}
	return m.corpus.len()
}

// Matches returns the number of paths that match the query shown.
func (m Model) Matches() int {
	if m.res == nil {
		return 0
	}
	return len(m.res.items)
}

// Matching reports whether a match of the query runs in a command.
func (m Model) Matching() bool { return m.matching }

// Selected returns the selected item, or false if there is none.
func (m Model) Selected() (Item, bool) {
	if m.res == nil || m.sel >= len(m.res.items) {
		return Item{}, false
	}
	return m.corpus.items[m.res.items[m.sel]], true
}

// Index returns the row of the selection among the matches.
func (m Model) Index() int { return m.sel }

// SetSize sets the width and height.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.layout()
}

// Width returns the width.
func (m Model) Width() int { return m.width }

// Height returns the height.
func (m Model) Height() int { return m.height }

// Focus makes the finder react to keys.
func (m *Model) Focus() {
	m.focused = true
	m.input.Focus()
	m.render()
}

// Blur makes the finder ignore keys.
func (m *Model) Blur() {
	m.focused = false
	m.input.Blur()
	m.render()
}

// Focused reports whether the finder reacts to keys.
func (m Model) Focused() bool { return m.focused }

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) { m.keys = k }

// ShortHelp implements help.KeyMap.
func (m Model) ShortHelp() []key.Binding { return m.keys.ShortHelp() }

// FullHelp implements help.KeyMap.
func (m Model) FullHelp() [][]key.Binding { return m.keys.FullHelp() }

// ranks returns the rank of each of the paths that c holds, by its index.
func (c *corpus) ranks(paths []string) map[int32]int {
	if len(paths) == 0 {
		return nil
	}
	r := make(map[int32]int, len(paths))
	for rank, p := range paths {
		if i, ok := c.index[p]; ok {
			if _, dup := r[i]; !dup {
				r[i] = rank
			}
		}
	}
	return r
}
