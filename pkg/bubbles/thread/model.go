// Package thread is a scrollable document, such as the header and body of an
// issue, followed by its comments. The body is markdown, rendered with
// package markdown, which the comments can render with too.
// Comments load lazily in chunks, oldest first, as the reader nears the end.
package thread

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2/ansi"

	"github.com/eggzec/gh-tui/pkg/markdown"
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
	emptyText     string
	errorText     func(error) (text, hint string)

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
	// md renders the body and, through Markdown, the comments, and keeps
	// what it rendered. Copies of the model share it.
	md *markdown.Renderer
	// folds keeps which diagrams the reader opened. Copies of the model
	// share it too, since a comment renderer calls Markdown on its own.
	folds *folds
	// docHeads are the heads of the diagrams in doc, and heads those in
	// lines, in order.
	docHeads []head
	heads    []head

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
	starts []int  // line of each item in lines
	heads  []head // the diagrams in lines
	height int

	loading bool
	seq     int
	err     error
	// said is what the thread says of err, worded once as it is set.
	said said
}

// tail tracks the request for the chunk after the last one.
type tail struct {
	loading bool
	seq     int
	err     error
	said    said
}

// said is what the thread says of a failed fetch: the words, and the
// hint after them, such as "r to retry".
type said struct {
	text, hint string
}

// texts are the fixed status fragments, styled once in SetStyles.
type texts struct {
	loadingDoc, loadingComments, empty string
	// pointer marks the head of the diagram that the toggle key opens.
	pointer string
}

// New returns a thread that loads comments with fetch and draws each one
// with render.
func New[T any](fetch Fetch[T], render Render[T], opts ...Option) Model[T] {
	s := settings{
		keys:      DefaultKeyMap(),
		styles:    DefaultStyles(true),
		ctx:       context.Background(),
		maxChunks: DefaultMaxChunks,
		emptyText: "No comments yet.",
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
		emptyText: s.emptyText,
		errorText: s.errorText,
		parent:    s.ctx,
		vp:        viewport.New(),
		spin:      spinner.New(spinner.WithSpinner(spinner.Dot)),
		docWidth:  -1,
		statusIdx: -1,
		md:        markdown.New(s.styles.Markdown),
		folds:     &folds{},
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
func (m *Model[T]) SetDocument(header, body string) tea.Cmd {
	a := m.anchor()
	m.hasDoc = true
	m.header, m.body = header, body
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

// SetFirst shows items as the first chunk of comments, with next the cursor
// of the chunk after them, so that comments the caller already has, such as
// from a cache, show without waiting for a fetch. It does nothing once the
// first chunk was requested. [Model.Reload] fetches the chunk again.
func (m *Model[T]) SetFirst(items []T, next string) {
	if m.started {
		return
	}
	a := m.anchor()
	c := chunk[T]{next: next, items: slices.Clone(items)}
	m.renderChunk(&c)
	m.chunks, m.started = append(m.chunks, c), true
	m.layout(a)
}

// Reset clears the document and the comments and cancels the fetches in
// flight, to show a new document. It returns the command that starts the
// loading spinner, which shows until [Model.SetDocument].
func (m *Model[T]) Reset() tea.Cmd {
	m.cancel()
	m.ctx, m.cancel = context.WithCancel(m.parent)
	m.gen++
	m.hasDoc, m.header, m.body = false, "", ""
	m.doc, m.docWidth, m.docHeads = nil, -1, nil
	m.folds.open = nil
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

// SetStyles sets the styles and renders what depends on them again: the
// document and the loaded comments, whose renderer may use the styles of
// the caller, which change with them.
func (m *Model[T]) SetStyles(s Styles) {
	a := m.anchor()
	m.styles = s
	m.md.SetStyle(m.markdownStyle())
	m.spin.Style = s.Spinner
	m.text = texts{
		loadingDoc:      s.Loading.Render("Loading…"),
		loadingComments: s.Loading.Render("Loading comments…"),
		empty:           s.Empty.Render(m.emptyText),
		pointer:         s.Key.Render("›"),
	}
	m.rerender(a)
}

// SetCutHint sets what a body or comment too long to show in full offers
// in the note that ends it, such as the key that opens it on GitHub, and
// renders them again.
func (m *Model[T]) SetCutHint(hint string) {
	m.md.SetHint(hint)
	m.rerender(m.anchor())
}

// rerender renders the document and the loaded comments again and
// scrolls back to a.
func (m *Model[T]) rerender(a anchor) {
	m.docWidth = -1
	m.renderDoc()
	for i := range m.chunks {
		if m.chunks[i].loaded {
			m.renderChunk(&m.chunks[i])
		}
	}
	m.layout(a)
}

// Markdown returns src rendered as markdown in the style of the body, as
// lines at most width cells wide, not padded, with the collapsible blocks
// whose index is in open shown in full, as [markdown.Renderer.Render]
// does, and those the reader opened too. A comment renderer uses it so
// the comments look like the body, and so the reader can open their
// diagrams, if it keeps the lines it returns whole and in order. What it
// renders is kept, so rendering a comment again, as a reload does, costs
// nothing.
func (m Model[T]) Markdown(src string, width int, open ...int) string {
	out, heads := m.md.RenderHeads(src, width, m.folds.with(src, open)...)
	if len(heads) > 0 {
		m.folds.seen = append(m.folds.seen, seen{src: src, text: out, heads: heads})
	}
	return out
}

// MarkdownRenders returns how many times the thread rendered markdown
// rather than reading what it rendered before, so the tests of a caller
// can tell that scrolling renders nothing.
func (m Model[T]) MarkdownRenders() int { return m.md.Renders() }

// Styles returns the styles.
func (m Model[T]) Styles() Styles { return m.styles }

// SetKeyMap sets the key bindings.
func (m *Model[T]) SetKeyMap(k KeyMap) {
	m.keys = k
	m.reword()
}

// SetErrorText sets how the thread reads a failed fetch, as
// [WithErrorText] does.
func (m *Model[T]) SetErrorText(say func(error) (text, hint string)) {
	m.errorText = say
	m.reword()
}

// KeyMap returns the key bindings.
func (m Model[T]) KeyMap() KeyMap { return m.keys }

// ShortHelp implements help.KeyMap. It offers retry only after an error,
// and to toggle a diagram only while one is on screen.
func (m Model[T]) ShortHelp() []key.Binding {
	return m.activeKeys().ShortHelp()
}

// FullHelp implements help.KeyMap, offering what ShortHelp does.
func (m Model[T]) FullHelp() [][]key.Binding {
	return m.activeKeys().FullHelp()
}

// activeKeys returns the key bindings with those that do nothing now
// disabled.
func (m Model[T]) activeKeys() KeyMap {
	k := m.keys
	k.Retry.SetEnabled(k.Retry.Enabled() && m.failed())
	k.Toggle.SetEnabled(k.Toggle.Enabled() && m.OnDiagram())
	return k
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

// word returns what the thread says of err, a failed fetch.
func (m *Model[T]) word(err error) said {
	retry := ""
	if k := m.keys.Retry.Help().Key; k != "" {
		retry = k + " to retry"
	}
	if m.errorText == nil {
		msg, _, _ := strings.Cut(err.Error(), "\n")
		return said{text: "Couldn't load comments: " + msg, hint: retry}
	}
	text, hint := m.errorText(err)
	if text == "" {
		// A failure not worth telling, such as a canceled fetch, still
		// leaves comments to read again, which the retry key does.
		return said{hint: retry}
	}
	return said{text: text, hint: hint}
}

// reword words the failed fetches again, after the keys or the error text
// changed, and lays the thread out with them.
func (m *Model[T]) reword() {
	for i := range m.chunks {
		if c := &m.chunks[i]; c.err != nil {
			c.said = m.word(c.err)
		}
	}
	if m.tail.err != nil {
		m.tail.said = m.word(m.tail.err)
	}
	m.layout(m.anchor())
}

// Err returns the error of a fetch that failed, the next chunk's first,
// or nil if none did.
func (m Model[T]) Err() error {
	if m.tail.err != nil {
		return m.tail.err
	}
	for i := range m.chunks {
		if err := m.chunks[i].err; err != nil {
			return err
		}
	}
	return nil
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
