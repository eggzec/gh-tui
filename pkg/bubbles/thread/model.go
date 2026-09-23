// Package thread is a scrollable document, such as the header and body of an
// issue, followed by its comments. The body is markdown rendered with glamour.
// Comments load lazily in chunks, oldest first, as the reader nears the end.
package thread

import (
	"context"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2/ansi"
)

// Fetch returns the chunk of comments after cursor, oldest first. The first
// chunk has an empty cursor, and an empty next cursor means there are no
// more. The service decides the chunk size.
type Fetch[T any] func(ctx context.Context, cursor string) (items []T, next string, err error)

// Render renders one comment as a block of lines at most width cells wide.
// Its height may vary with the content.
type Render[T any] func(item T, width int) string

var lastID atomic.Int64

// Model is a thread of type T comments. Create it with [New].
type Model[T any] struct {
	id     int64
	gen    int
	fetch  Fetch[T]
	render Render[T]

	width, height int
	focused       bool
	keys          KeyMap
	styles        Styles
	markdown      *ansi.StyleConfig
	maxChunks     int

	parent context.Context
	ctx    context.Context
	cancel context.CancelFunc

	vp   viewport.Model
	spin spinner.Model

	hasDoc   bool
	header   string
	body     string
	doc      []string // header and body lines, rendered at docWidth
	docWidth int
	mdRuns   int // glamour renders, so tests can prove View doesn't render

	chunks  []chunk[T]
	started bool // the first chunk was requested
	tail    tail

	// lines is the whole thread, one entry per screen line, each exactly
	// width cells wide, so View only slices it.
	lines     []string
	starts    []int // line of each chunk in lines
	statusIdx int   // line of the status, or -1
	status    string
	blank     string

	text texts
}

// chunk is one page of comments, rendered once per width. An evicted chunk
// drops its items and lines but keeps its cursor, to fetch it again, and its
// height, so the lines after it don't move.
type chunk[T any] struct {
	cursor string
	next   string
	items  []T
	loaded bool
	lines  []string
	starts []int // line of each item in lines
	height int

	loading bool
	seq     int
	err     error
}

// tail tracks the request for the chunk after the last one.
type tail struct {
	loading bool
	seq     int
	err     error
}

// texts are the fixed status fragments, styled once in SetStyles.
type texts struct {
	loadingDoc, loadingComments, empty, errPrefix, retry string
}

// New returns a thread that loads comments with fetch and draws each one
// with render.
func New[T any](fetch Fetch[T], render Render[T], opts ...Option) Model[T] {
	s := settings{
		keys:      DefaultKeyMap(),
		styles:    DefaultStyles(true),
		ctx:       context.Background(),
		maxChunks: DefaultMaxChunks,
	}
	for _, opt := range opts {
		opt(&s)
	}
	m := Model[T]{
		id:        lastID.Add(1),
		fetch:     fetch,
		render:    render,
		focused:   s.focused,
		keys:      s.keys,
		markdown:  s.markdown,
		maxChunks: s.maxChunks,
		parent:    s.ctx,
		vp:        viewport.New(),
		spin:      spinner.New(spinner.WithSpinner(spinner.Dot)),
		docWidth:  -1,
		statusIdx: -1,
	}
	m.ctx, m.cancel = context.WithCancel(m.parent)
	m.width, m.height = max(s.width, 0), max(s.height, 0)
	m.vp.SetWidth(m.width)
	m.vp.SetHeight(m.height)
	m.blank = strings.Repeat(" ", m.width)
	m.SetStyles(s.styles)
	return m
}

// ID returns the instance ID that scopes the thread's messages.
func (m Model[T]) ID() int64 { return m.id }

// Init starts the spinner shown until the document is set.
func (m Model[T]) Init() tea.Cmd {
	if m.hasDoc {
		return nil
	}
	return m.spin.Tick
}

// SetDocument sets the header, already styled by the caller, and the
// markdown body. It returns the command that loads the first chunk of
// comments. Setting the document again, for example after the body was
// edited, keeps the comments and the reading position.
func (m *Model[T]) SetDocument(header, markdown string) tea.Cmd {
	a := m.anchor()
	m.hasDoc = true
	m.header, m.body = header, markdown
	m.docWidth = -1
	m.renderDoc()
	m.layout(a)
	return m.manage()
}

// Reload fetches the loaded chunks again, for example after a sync event or
// when a comment was posted, and keeps the reading position. Evicted chunks
// are fetched fresh anyway when the screen nears them. If the last chunk
// gained a next cursor, the chunks after it load as usual.
func (m *Model[T]) Reload() tea.Cmd {
	if !m.hasDoc {
		return nil
	}
	var cmd tea.Cmd
	relayout := false
	for i := range m.chunks {
		c := &m.chunks[i]
		if c.loaded || c.err != nil {
			relayout = relayout || c.err != nil
			cmd = batch(cmd, m.loadChunk(i))
		}
	}
	if relayout {
		m.layout(m.anchor())
	}
	if m.tail.err != nil {
		cmd = batch(cmd, m.loadTail())
	}
	return batch(cmd, m.manage())
}

// Reset clears the document and the comments and cancels the fetches in
// flight, to show a new document. It returns the command that starts the
// loading spinner, which shows until [Model.SetDocument].
func (m *Model[T]) Reset() tea.Cmd {
	m.cancel()
	m.ctx, m.cancel = context.WithCancel(m.parent)
	m.gen++
	m.hasDoc, m.header, m.body = false, "", ""
	m.doc, m.docWidth = nil, -1
	m.chunks, m.started, m.tail = nil, false, tail{}
	m.vp.SetYOffset(0)
	m.layout(top)
	return m.spin.Tick
}

// SetSize sets the width and height. A new width renders the document and
// the loaded comments again and keeps the comment at the top of the screen
// in place.
func (m *Model[T]) SetSize(width, height int) {
	width, height = max(width, 0), max(height, 0)
	if width == m.width && height == m.height {
		return
	}
	a := m.anchor()
	m.vp.SetHeight(height)
	m.height = height
	if width != m.width {
		m.width = width
		m.vp.SetWidth(width)
		m.blank = strings.Repeat(" ", width)
		m.renderDoc()
		for i := range m.chunks {
			if m.chunks[i].loaded {
				m.renderChunk(&m.chunks[i])
			}
		}
	}
	m.layout(a)
}

// Width returns the width.
func (m Model[T]) Width() int { return m.width }

// Height returns the height.
func (m Model[T]) Height() int { return m.height }

// SetMaxChunks sets how many comment chunks stay in memory, as in
// [WithMaxChunks].
func (m *Model[T]) SetMaxChunks(n int) { m.maxChunks = n }

// MaxChunks returns how many comment chunks stay in memory.
func (m Model[T]) MaxChunks() int { return m.maxChunks }

// SetStyles sets the styles and renders what depends on them again.
func (m *Model[T]) SetStyles(s Styles) {
	a := m.anchor()
	m.styles = s
	m.spin.Style = s.Spinner
	m.text = texts{
		loadingDoc:      s.Loading.Render("Loading…"),
		loadingComments: s.Loading.Render("Loading comments…"),
		empty:           s.Empty.Render("No comments yet."),
		errPrefix:       s.Error.Render("Couldn't load comments."),
	}
	m.styleRetry()
	m.docWidth = -1
	m.renderDoc()
	m.layout(a)
}

// Styles returns the styles.
func (m Model[T]) Styles() Styles { return m.styles }

// SetKeyMap sets the key bindings.
func (m *Model[T]) SetKeyMap(k KeyMap) {
	m.keys = k
	m.styleRetry()
	m.layout(m.anchor())
}

// KeyMap returns the key bindings.
func (m Model[T]) KeyMap() KeyMap { return m.keys }

// ShortHelp implements help.KeyMap. It offers retry only after an error.
func (m Model[T]) ShortHelp() []key.Binding {
	k := m.keys
	k.Retry.SetEnabled(k.Retry.Enabled() && m.failed())
	return k.ShortHelp()
}

// FullHelp implements help.KeyMap. It offers retry only after an error.
func (m Model[T]) FullHelp() [][]key.Binding {
	k := m.keys
	k.Retry.SetEnabled(k.Retry.Enabled() && m.failed())
	return k.FullHelp()
}

// Focus focuses the thread so it reacts to keys.
func (m *Model[T]) Focus() { m.focused = true }

// Blur blurs the thread so it ignores keys.
func (m *Model[T]) Blur() { m.focused = false }

// Focused reports whether the thread is focused.
func (m Model[T]) Focused() bool { return m.focused }

// ScrollPercent returns how far the thread is scrolled, from 0 to 1, over
// what is laid out so far.
func (m Model[T]) ScrollPercent() float64 { return m.vp.ScrollPercent() }

// AtBottom reports whether the last laid out line is on screen. More
// comments may still load.
func (m Model[T]) AtBottom() bool { return m.vp.AtBottom() }

// YOffset returns the first line on screen.
func (m Model[T]) YOffset() int { return m.vp.YOffset() }

// TotalLines returns the number of lines laid out.
func (m Model[T]) TotalLines() int { return len(m.lines) }

func (m *Model[T]) styleRetry() {
	s := m.styles
	m.text.retry = s.Hint.Render(" Press ") +
		s.Key.Render(m.keys.Retry.Help().Key) +
		s.Hint.Render(" to retry.")
}

func (m Model[T]) failed() bool {
	if m.tail.err != nil {
		return true
	}
	for i := range m.chunks {
		if m.chunks[i].err != nil {
			return true
		}
	}
	return false
}

func (m Model[T]) markdownStyle() ansi.StyleConfig {
	if m.markdown != nil {
		return *m.markdown
	}
	return m.styles.Markdown
}
