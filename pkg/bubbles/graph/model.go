// Package graph provides a lazily loaded commit graph, like git log --graph.
//
// A graph pulls commits newest first, in chunks, from a [Fetch] function, and
// draws each one on a row with the lanes of its history to the left: ● for
// the commit, │ for the lanes passing by, and ╮ ╯ ├ ─ where branches fork
// and merge. The lanes are laid out as chunks arrive, so a new chunk only
// continues the layout, and only the rows that fit in the window are drawn.
//
// Unlike feed, a graph keeps every chunk it loaded: the layout of a chunk
// depends on all the chunks before it.
package graph

import (
	"context"
	"slices"
	"strings"
	"sync/atomic"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Commit is one row of the graph.
type Commit struct {
	// ID identifies the commit, such as its full SHA. It must be unique and
	// not empty.
	ID string
	// Parents are the IDs of the parents, the first parent first. Parents
	// that are not loaded yet keep their lane open until they are.
	Parents []string
	// Short is shown dimmed before the title, such as the short SHA.
	Short string
	// Title is the main text of the row, such as the subject.
	Title string
	// Detail is shown dimmed after the title, such as the author.
	Detail string
	// Right is shown dimmed at the right edge of the row, such as the age.
	// It is dropped first when the row is narrow.
	Right string
	// Link is an address that Short and Title link to, such as the
	// commit's page on the web, which a terminal that knows links (OSC 8)
	// opens on a click. It is optional, and only a plain https address
	// links.
	Link string
	// Value carries whatever the producer wants back with the commit.
	Value any
}

// Fetch returns the chunk of commits after cursor, newest first. An empty
// cursor asks for the first chunk, and an empty next cursor means there are
// no more chunks. Commits with an empty or repeated ID are ignored.
type Fetch func(ctx context.Context, cursor string) (commits []Commit, next string, err error)

var lastID atomic.Int64

func nextID() int {
	return int(lastID.Add(1))
}

// row is a loaded commit, laid out and rendered.
type row struct {
	commit Commit
	cells  []cell
	// text is the short SHA, title and detail in their styles, and right
	// the right column; the widths are in cells.
	text   string
	textW  int
	right  string
	rightW int
}

// Model is a commit graph. Create one with [New].
//
// The set of seen IDs lives in a map shared by copies of the model, as the
// Elm pattern hands each Update the latest copy and drops the old one.
type Model struct {
	settings

	id    int
	fetch Fetch
	// gen counts Resets; ctx is cancelled when it changes.
	gen    int
	ctx    context.Context
	cancel context.CancelFunc

	rows   []row
	seen   map[string]struct{}
	layout layout
	// cursor fetches the next chunk.
	cursor   string
	done     bool
	fetching bool
	err      error

	sel int
	top int
	// announced is the ID of the commit the last SelectMsg was about.
	announced string
	// resized asks the next Update to fetch what a new size shows.
	resized bool

	spin     spinner.Model
	spinning bool

	// Rendered once in SetStyles and SetKeyMap, so View only copies them.
	// frags[g][c] is glyph g in the color of lane c, and hline[c] a
	// horizontal line in the gap after a slot.
	frags         [glyphCount][]string
	hline         []string
	gutterFocused string
	gutterBlurred string
	gutterNone    string
	loadingText   string
	emptyLine     string
	errLine       string
	errHint       string
}

// New returns a graph that loads commits with fetch. Call Init to fetch the
// first chunk.
func New(fetch Fetch, opts ...Option) Model {
	m := Model{
		settings: defaultSettings(),
		id:       nextID(),
		fetch:    fetch,
		spin:     spinner.New(spinner.WithSpinner(spinner.Dot)),
	}
	for _, opt := range opts {
		opt(&m.settings)
	}
	m.ctx, m.cancel = context.WithCancel(m.parent)
	m.clear()
	// Init fetches the first chunk, and it cannot record that itself.
	m.fetching = true
	m.spinning = true
	m.SetKeyMap(m.keyMap)
	m.SetStyles(m.styles)
	return m
}

// Init fetches the first chunk and starts the spinner.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.fetchCmd(), m.spin.Tick)
}

// Reset forgets every commit and loads the history from fetch, for example
// for another branch. Fetches in flight are cancelled, and the first commit
// sends a [SelectMsg] again once it loads.
func (m *Model) Reset(fetch Fetch) tea.Cmd {
	m.cancel()
	m.ctx, m.cancel = context.WithCancel(m.parent)
	m.gen++
	m.fetch = fetch
	m.clear()
	return m.startFetch()
}

// clear forgets every commit.
func (m *Model) clear() {
	m.rows = nil
	m.seen = map[string]struct{}{}
	m.layout = newLayout(m.maxLanes)
	m.cursor, m.done, m.err = "", false, nil
	m.sel, m.top = 0, 0
	m.announced = ""
}

// ID returns the unique ID of the graph.
func (m Model) ID() int {
	return m.id
}

// Selected returns the commit under the cursor, or false if there is none.
func (m Model) Selected() (Commit, bool) {
	if m.sel >= len(m.rows) {
		return Commit{}, false
	}
	return m.rows[m.sel].commit, true
}

// At returns the loaded commit at index i, newest first, or false if i is
// out of range. It lets the parent look around the cursor, such as to read
// ahead what the commits next to it show.
func (m Model) At(i int) (Commit, bool) {
	if i < 0 || i >= len(m.rows) {
		return Commit{}, false
	}
	return m.rows[i].commit, true
}

// Index returns the index of the commit under the cursor.
func (m Model) Index() int {
	return m.sel
}

// Len returns the number of commits loaded.
func (m Model) Len() int {
	return len(m.rows)
}

// Done reports whether the last chunk has been fetched.
func (m Model) Done() bool {
	return m.done
}

// Err returns the error of a failed fetch that has not been retried.
func (m Model) Err() error {
	return m.err
}

// SetSize sets the width and height of the graph. Rows a larger window shows
// are fetched on the next Update.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.scroll()
	m.resized = true
}

// Width returns the width of the graph.
func (m Model) Width() int {
	return m.width
}

// Height returns the height of the graph.
func (m Model) Height() int {
	return m.height
}

// Focus makes the graph react to keys.
func (m *Model) Focus() {
	m.focused = true
}

// Blur makes the graph ignore keys.
func (m *Model) Blur() {
	m.focused = false
}

// Focused reports whether the graph reacts to keys.
func (m Model) Focused() bool {
	return m.focused
}

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keyMap = k
	m.refreshError()
}

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap {
	return m.keyMap
}

// SetStyles sets the styles and renders the fragments and rows that depend
// on them.
func (m *Model) SetStyles(s Styles) {
	s.Lanes = append([]lipgloss.Style(nil), s.Lanes...)
	if len(s.Lanes) == 0 {
		s.Lanes = []lipgloss.Style{{}}
	}
	m.styles = s
	m.spin.Style = s.Spinner
	for g := range glyphCount {
		m.frags[g] = make([]string, len(s.Lanes))
		for c := range s.Lanes {
			switch g {
			case glyphSpace, glyphLine:
				m.frags[g][c] = " "
			case glyphOverflow:
				m.frags[g][c] = s.Overflow.Render(glyphs[g])
			default:
				m.frags[g][c] = s.Lanes[c].Render(glyphs[g])
			}
		}
	}
	m.hline = make([]string, len(s.Lanes))
	for c := range s.Lanes {
		m.hline[c] = s.Lanes[c].Render("─")
	}
	m.gutterFocused = s.Cursor.Render(cursorGlyph) + " "
	m.gutterBlurred = s.BlurredCursor.Render(cursorGlyph) + " "
	m.gutterNone = "  "
	m.loadingText = s.Loading.Render("Loading…")
	m.emptyLine = s.Empty.Render(m.emptyText)
	// Copies of the model share the rows, so render into new ones.
	m.rows = slices.Clone(m.rows)
	for i := range m.rows {
		m.render(&m.rows[i])
	}
	m.refreshError()
}

// Styles returns the styles.
func (m Model) Styles() Styles {
	s := m.styles
	s.Lanes = append([]lipgloss.Style(nil), s.Lanes...)
	return s
}

// SetEmptyText sets the text shown when the branch has no commits.
func (m *Model) SetEmptyText(text string) {
	m.emptyText = text
	m.emptyLine = m.styles.Empty.Render(text)
}

// render renders the text of r in the current styles.
func (m Model) render(r *row) {
	c := r.commit
	var b strings.Builder
	w := 0
	part := func(s string, st lipgloss.Style) {
		if s == "" {
			return
		}
		if w > 0 {
			b.WriteByte(' ')
			w++
		}
		b.WriteString(st.Render(s))
		w += ansi.StringWidth(s)
	}
	part(c.Short, m.styles.Short)
	part(c.Title, m.styles.Title)
	if c.Link != "" {
		linked := termtext.Link(c.Link, b.String())
		b.Reset()
		b.WriteString(linked)
	}
	part(c.Detail, m.styles.Detail)
	r.text, r.textW = b.String(), w
	r.right, r.rightW = "", 0
	if c.Right != "" {
		r.right, r.rightW = m.styles.Right.Render(c.Right), ansi.StringWidth(c.Right)
	}
}

// startFetch marks the next chunk as in flight and returns the command that
// fetches it.
func (m *Model) startFetch() tea.Cmd {
	m.fetching = true
	m.err = nil
	m.refreshError()
	cmd := m.fetchCmd()
	if !m.spinning {
		m.spinning = true
		return tea.Batch(cmd, m.spin.Tick)
	}
	return cmd
}

func (m Model) fetchCmd() tea.Cmd {
	fetch, ctx, id, gen, cursor := m.fetch, m.ctx, m.id, m.gen, m.cursor
	return func() tea.Msg {
		commits, next, err := fetch(ctx, cursor)
		return chunkMsg{id: id, gen: gen, cursor: cursor, commits: commits, next: next, err: err}
	}
}

// refreshError renders the error row, which depends on the error, the
// styles and the retry key.
func (m *Model) refreshError() {
	m.keyMap.Retry.SetEnabled(m.err != nil)
	if m.err == nil {
		m.errLine, m.errHint = "", ""
		return
	}
	msg, _, _ := strings.Cut(m.err.Error(), "\n")
	m.errLine = m.styles.Error.Render("✗ Couldn't load: " + msg)
	m.errHint = ""
	if h := m.keyMap.Retry.Help(); h.Key != "" {
		m.errHint = m.styles.Hint.Render(" · " + h.Key + " to " + h.Desc)
	}
}
