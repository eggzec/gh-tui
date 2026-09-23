// Package feed provides a lazily loaded, windowed list.
//
// A feed pulls items in chunks from a [Fetch] function, in whatever size the
// producer returns, and renders only the rows that fit in its window. Unlike
// bubbles/list it never needs every item in memory.
package feed

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
)

// Fetch returns the chunk of items after cursor. An empty cursor asks for the
// first chunk, and an empty next cursor means there are no more chunks.
type Fetch[T any] func(ctx context.Context, cursor string) (items []T, next string, err error)

// Render renders one item in at most width cells. With an item height above
// one, lines are separated by "\n"; extra lines are dropped.
type Render[T any] func(item T, selected bool, width int) string

var lastID atomic.Int64

func nextID() int {
	return int(lastID.Add(1))
}

// chunk is one fetched page of items.
type chunk[T any] struct {
	// cursor fetched this chunk and next is the cursor it returned.
	cursor string
	next   string
	items  []T
	// n is the number of items in the chunk.
	n        int
	err      error
	fetching bool
}

// Model is a feed of items of type T. Create one with [New].
type Model[T any] struct {
	settings

	id     int
	fetch  Fetch[T]
	render Render[T]

	chunks []chunk[T]
	// starts[i] is the index of the first item of chunks[i].
	starts []int
	total  int
	// tail is the chunk after the last one: its cursor, and whether it is
	// being fetched or failed.
	tail chunk[T]
	done bool

	sel int
	top int

	spin     spinner.Model
	spinning bool

	// Rendered once in SetStyles and SetKeyMap, so View only copies them.
	gutterFocused string
	gutterBlurred string
	gutterNone    string
	loadingText   string
	emptyLine     string
	errLine       string
	errHint       string
}

// New returns a feed that loads items with fetch and draws them with render.
// Call Init to fetch the first chunk.
func New[T any](fetch Fetch[T], render Render[T], opts ...Option) Model[T] {
	m := Model[T]{
		settings: defaultSettings(),
		id:       nextID(),
		fetch:    fetch,
		render:   render,
		spin:     spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	for _, opt := range opts {
		opt(&m.settings)
	}
	// Init fetches the first chunk, and it cannot record that itself.
	m.tail.fetching = true
	m.spinning = true
	m.SetKeyMap(m.keyMap)
	m.SetStyles(m.styles)
	return m
}

// Init fetches the first chunk and starts the spinner.
func (m Model[T]) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(0, m.tail.cursor), m.spin.Tick)
}

// ID returns the unique ID of the feed.
func (m Model[T]) ID() int {
	return m.id
}

// Selected returns the selected item, or false if there is none or it is not
// loaded.
func (m Model[T]) Selected() (T, bool) {
	return m.item(m.sel)
}

// Index returns the index of the selected item.
func (m Model[T]) Index() int {
	return m.sel
}

// Len returns the number of items known so far, loaded or not.
func (m Model[T]) Len() int {
	return m.total
}

// Done reports whether the last chunk has been fetched.
func (m Model[T]) Done() bool {
	return m.done
}

// Err returns the error of the last failed fetch, if it has not been retried.
func (m Model[T]) Err() error {
	return m.tail.err
}

// SetSize sets the width and height of the feed. It only moves the window;
// it never changes what is fetched.
func (m *Model[T]) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.scroll()
}

// SetWidth sets the width of the feed.
func (m *Model[T]) SetWidth(width int) {
	m.SetSize(width, m.height)
}

// SetHeight sets the height of the feed.
func (m *Model[T]) SetHeight(height int) {
	m.SetSize(m.width, height)
}

// Width returns the width of the feed.
func (m Model[T]) Width() int {
	return m.width
}

// Height returns the height of the feed.
func (m Model[T]) Height() int {
	return m.height
}

// Focus makes the feed react to keys.
func (m *Model[T]) Focus() {
	m.focused = true
}

// Blur makes the feed ignore keys.
func (m *Model[T]) Blur() {
	m.focused = false
}

// Focused reports whether the feed reacts to keys.
func (m Model[T]) Focused() bool {
	return m.focused
}

// SetKeyMap sets the key bindings.
func (m *Model[T]) SetKeyMap(k KeyMap) {
	m.keyMap = k
	m.refreshError()
}

// KeyMap returns the key bindings.
func (m Model[T]) KeyMap() KeyMap {
	return m.keyMap
}

// SetStyles sets the styles and renders the fragments that depend on them.
func (m *Model[T]) SetStyles(s Styles) {
	m.styles = s
	m.spin.Style = s.Spinner
	m.gutterFocused = s.Cursor.Render(cursorGlyph) + " "
	m.gutterBlurred = s.BlurredCursor.Render(cursorGlyph) + " "
	m.gutterNone = "  "
	m.loadingText = s.Loading.Render("Loading…")
	m.emptyLine = s.Empty.Render(m.emptyText)
	m.refreshError()
}

// SetEmptyText sets the text shown when the feed has no items.
func (m *Model[T]) SetEmptyText(text string) {
	m.emptyText = text
	m.emptyLine = m.styles.Empty.Render(text)
}

// EmptyText returns the text shown when the feed has no items.
func (m Model[T]) EmptyText() string {
	return m.emptyText
}

// Styles returns the styles.
func (m Model[T]) Styles() Styles {
	return m.styles
}

// item returns the item at index i if it is loaded.
func (m Model[T]) item(i int) (T, bool) {
	var zero T
	if i < 0 || i >= m.total {
		return zero, false
	}
	c := m.chunkAt(i)
	items := m.chunks[c].items
	if items == nil {
		return zero, false
	}
	return items[i-m.starts[c]], true
}

// chunkAt returns the index of the chunk that holds item i.
func (m Model[T]) chunkAt(i int) int {
	c, found := slices.BinarySearch(m.starts, i)
	if found {
		// Skip empty chunks that start at the same index.
		for c+1 < len(m.starts) && m.starts[c+1] == i {
			c++
		}
		return c
	}
	return c - 1
}

// reindex recomputes where each chunk starts after lengths changed.
func (m *Model[T]) reindex() {
	m.starts = m.starts[:0]
	m.total = 0
	for _, c := range m.chunks {
		m.starts = append(m.starts, m.total)
		m.total += c.n
	}
}

// chunk returns the chunk at index i, where len(m.chunks) is the tail.
func (m *Model[T]) chunk(i int) *chunk[T] {
	if i == len(m.chunks) {
		return &m.tail
	}
	return &m.chunks[i]
}

// startFetch marks chunk i as in flight and returns the command that fetches
// it.
func (m *Model[T]) startFetch(i int) tea.Cmd {
	c := m.chunk(i)
	c.fetching = true
	c.err = nil
	m.refreshError()
	cmd := m.fetchCmd(i, c.cursor)
	if i == len(m.chunks) && !m.spinning {
		m.spinning = true
		return tea.Batch(cmd, m.spin.Tick)
	}
	return cmd
}

func (m Model[T]) fetchCmd(i int, cursor string) tea.Cmd {
	fetch, ctx, id := m.fetch, m.ctx, m.id
	return func() tea.Msg {
		items, next, err := fetch(ctx, cursor)
		return chunkMsg[T]{id: id, index: i, cursor: cursor, items: items, next: next, err: err}
	}
}

// refreshError renders the error row, which depends on the error, the
// styles and the retry key.
func (m *Model[T]) refreshError() {
	failed := m.tail.err != nil
	m.keyMap.Retry.SetEnabled(failed)
	if !failed {
		m.errLine, m.errHint = "", ""
		return
	}
	msg, _, _ := strings.Cut(m.tail.err.Error(), "\n")
	m.errLine = m.styles.Error.Render("✗ Couldn't load: " + msg)
	m.errHint = ""
	if h := m.keyMap.Retry.Help(); h.Key != "" {
		m.errHint = m.styles.Hint.Render(" · " + h.Key + " to " + h.Desc)
	}
}
