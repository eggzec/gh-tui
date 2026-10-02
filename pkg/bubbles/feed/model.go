// Package feed provides a lazily loaded, windowed list.
//
// A feed pulls items in chunks from a [Fetch] function, in whatever size the
// producer returns, and renders only the rows that fit in its window. Unlike
// bubbles/list it never needs every item in memory.
package feed

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Fetch returns the chunk of items after cursor. An empty cursor asks for the
// first chunk, and an empty next cursor means there are no more chunks.
//
// A Fetch that has only items that may be out of date, such as items kept
// from an earlier session, returns them with [ErrStale], so that they are
// shown at once while the chunk is fetched again.
type Fetch[T any] func(ctx context.Context, cursor string) (items []T, next string, err error)

// ErrStale is returned by a Fetch together with items that may be out of
// date. The feed shows them as if the fetch had succeeded, and fetches the
// chunk again at once, keeping them on screen until the new ones arrive, the
// way Reload does. The Fetch must not return ErrStale for the same chunk
// every time.
var ErrStale = errors.New("feed: stale items")

// ErrKept is returned by a Fetch together with items kept from an earlier
// fetch, because the source can't give new ones now, such as while it
// can't be reached. The feed shows them as if the fetch had succeeded, and
// [Model.RetryKept] fetches the chunk again once the source can.
var ErrKept = errors.New("feed: kept items")

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
	// n is the number of items in the chunk. It survives eviction, so the
	// positions of later items stay stable.
	n int
	// loaded is false once the chunk's items have been evicted.
	loaded bool
	// kept reports that the chunk's items came with ErrKept.
	kept     bool
	err      error
	fetching bool
}

// Model is a feed of items of type T. Create one with [New].
type Model[T any] struct {
	settings

	id     int
	fetch  Fetch[T]
	render Render[T]
	key    func(T) string

	// gen counts Resets and Reloads; ctx is cancelled when it changes.
	gen    int
	ctx    context.Context
	cancel context.CancelFunc

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
	// resized asks the next Update to fetch what a new size shows.
	resized bool
	// err is the first failed fetch, shown in the error row.
	err error

	// anchor is the key of the item selected when Reload was called, and
	// anchorRow its row in the window. It holds until the user moves.
	anchor    string
	anchorRow int
	anchored  bool

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
	placeholder   string
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
	m.key, _ = m.settings.key.(func(T) string)
	m.ctx, m.cancel = context.WithCancel(m.parent)
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

// Reset forgets every item and fetches the first chunk again, for example
// for a new query. Results of earlier fetches are dropped.
func (m *Model[T]) Reset() tea.Cmd {
	m.newGeneration()
	clear(m.chunks)
	m.chunks = m.chunks[:0]
	m.reindex()
	m.tail = chunk[T]{}
	m.done = false
	m.sel, m.top = 0, 0
	m.anchored = false
	return m.startFetch(0)
}

// Reload fetches the loaded chunks again, for example after a sync event or
// an optimistic update, and shows the old items until the new ones arrive.
// With a key set, the selection follows the selected item; otherwise it
// keeps its index. Results of earlier fetches are dropped.
func (m *Model[T]) Reload() tea.Cmd {
	m.anchorSelection()
	m.newGeneration()

	var cmd tea.Cmd
	for i := range m.chunks {
		c := &m.chunks[i]
		c.fetching = false
		if c.loaded || c.err != nil {
			cmd = tea.Batch(cmd, m.startFetch(i))
		}
	}
	m.tail.fetching = false
	if len(m.chunks) == 0 || m.tail.err != nil {
		cmd = tea.Batch(cmd, m.startFetch(len(m.chunks)))
	}
	return cmd
}

// anchorSelection makes the selection follow the selected item, if the feed
// has a key, until the user moves.
func (m *Model[T]) anchorSelection() {
	m.anchored = false
	if it, ok := m.Selected(); ok && m.key != nil {
		m.anchor, m.anchorRow, m.anchored = m.key(it), m.sel-m.top, true
	}
}

// newGeneration cancels the fetches in flight and makes their results stale.
func (m *Model[T]) newGeneration() {
	if m.cancel != nil {
		m.cancel()
	}
	m.ctx, m.cancel = context.WithCancel(m.parent)
	m.gen++
}

// SetKey sets how to identify an item. See [WithKey].
func (m *Model[T]) SetKey(key func(T) string) {
	m.key = key
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

// Item returns item i, or false if there is none or it is not loaded.
func (m Model[T]) Item(i int) (T, bool) {
	return m.item(i)
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

// Settled reports whether the first chunk is loaded and not being fetched
// again, such as after it came back stale, so that work the user isn't
// waiting for can start without slowing down what the feed shows first.
func (m Model[T]) Settled() bool {
	return len(m.chunks) > 0 && m.chunks[0].loaded && !m.chunks[0].fetching
}

// Err returns the error of a failed fetch that has not been retried.
func (m Model[T]) Err() error {
	return m.err
}

// SetSize sets the width and height of the feed. It only moves the window;
// chunks keep their size. Rows the new window shows that were evicted are
// fetched again on the next Update.
func (m *Model[T]) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.scroll()
	m.resized = true
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
	m.spin.Spinner = spinner.Dot
	if len(s.SpinnerFrames.Frames) > 0 {
		m.spin.Spinner = s.SpinnerFrames
	}
	cursor := termtext.Cells(s.CursorGlyph, 1)
	m.gutterFocused = s.Cursor.Render(cursor) + " "
	m.gutterBlurred = s.BlurredCursor.Render(cursor) + " "
	m.gutterNone = "  "
	m.loadingText = s.Loading.Render("Loading" + s.Ellipsis)
	m.emptyLine = s.Empty.Render(m.emptyText)
	m.placeholder = s.Placeholder.Render(s.Ellipsis)
	m.refreshError()
}

// SetEmptyText sets the text shown when the feed has no items.
func (m *Model[T]) SetEmptyText(text string) {
	m.emptyText = text
	m.emptyLine = m.styles.Empty.Render(text)
}

// SetErrorText sets how the error row reads a failed fetch, as
// [WithErrorText] does.
func (m *Model[T]) SetErrorText(say func(error) (text, hint string)) {
	m.errorText = say
	m.refreshError()
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
	if !m.chunks[c].loaded {
		return zero, false
	}
	return m.chunks[c].items[i-m.starts[c]], true
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
	fetch, ctx, id, gen := m.fetch, m.ctx, m.id, m.gen
	return func() tea.Msg {
		items, next, err := fetch(ctx, cursor)
		return chunkMsg[T]{id: id, gen: gen, index: i, cursor: cursor, items: items, next: next, err: err}
	}
}

// find returns the index of the loaded item with the given key.
func (m Model[T]) find(key string) (int, bool) {
	for i, c := range m.chunks {
		if !c.loaded {
			continue
		}
		for j, it := range c.items {
			if m.key(it) == key {
				return m.starts[i] + j, true
			}
		}
	}
	return 0, false
}

// refreshError renders the error row, which depends on the error, the
// styles, the retry key and the error text.
func (m *Model[T]) refreshError() {
	m.err = m.tail.err
	for _, c := range m.chunks {
		if c.err != nil {
			m.err = c.err
			break
		}
	}
	m.keyMap.Retry.SetEnabled(m.err != nil)
	m.errLine, m.errHint = "", ""
	if m.err == nil {
		return
	}
	text, hint := m.errorWords(m.err)
	if text == "" {
		// A failure not worth telling, such as a canceled fetch, still
		// leaves rows to read again, which the retry key does.
		if k := m.keyMap.Retry.Help().Key; k != "" {
			m.errHint = m.styles.Hint.Render(k + " to retry")
		}
		return
	}
	m.errLine = m.styles.Error.Render(m.styles.ErrorGlyph + " " + text)
	if hint != "" {
		m.errHint = m.styles.Hint.Render(m.styles.ErrorSeparator + hint)
	}
}

// errorWords returns what the error row says of err, and the hint after it.
func (m *Model[T]) errorWords(err error) (text, hint string) {
	if m.errorText != nil {
		return m.errorText(err)
	}
	msg, _, _ := strings.Cut(err.Error(), "\n")
	if k := m.keyMap.Retry.Help().Key; k != "" {
		hint = k + " to retry"
	}
	return "Couldn't load: " + msg, hint
}
