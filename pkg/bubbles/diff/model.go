package diff

import (
	"context"
	"sync/atomic"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Fetch returns the page of files after cursor. An empty cursor asks for the
// first page, and an empty next cursor means there are no more pages.
type Fetch func(ctx context.Context, cursor string) (files []File, next string, err error)

var lastID atomic.Int64

// paging is where the fetching of pages stands.
type paging struct {
	// next is the cursor of the page to fetch, done says there is none,
	// and err is why the last fetch failed.
	next     string
	done     bool
	fetching bool
	err      error
	// seek is a position asked for before its file was fetched; file says
	// that only the file's header was asked for.
	seek *pendingSeek
}

type pendingSeek struct {
	pos  Pos
	file bool
}

// Model is a view of the diff of many files, with a line cursor. Create one
// with [New].
//
// The model reads a [Layout], which it shares with its copies: only the
// latest copy of a model should be updated, as Bubble Tea does.
type Model struct {
	settings

	id     int
	fetch  Fetch
	layout *Layout

	// pg is the state of the paging, shared by the copies of the model as
	// the layout is, so that they cannot disagree on what was fetched.
	pg *paging

	// hl holds the tokens of the files, shared as well.
	hl *highlights

	// cursor is the row of the cursor, top the first row of the window
	// (or, when it is inside a file, the row the file's header stands in
	// for), and left the cells scrolled sideways.
	cursor int
	top    int
	left   int
	// dirty asks the next Update to fetch what a new size or a seek shows.
	dirty bool

	// Rendered once in SetStyles, so View only copies them.
	gutterFocused string
	gutterBlurred string
	wrap          wraps
}

// New returns a view that loads files with fetch. Call Init to fetch the
// first page.
func New(fetch Fetch, opts ...Option) Model {
	m := Model{
		settings: defaultSettings(),
		id:       int(lastID.Add(1)),
		fetch:    fetch,
		pg:       &paging{},
		hl:       newHighlights(),
	}
	for _, o := range opts {
		o(&m.settings)
	}
	m.layout = NewLayout(nil, opts...)
	// Init fetches the first page, and it cannot record that itself.
	m.pg.fetching = true
	m.SetKeyMap(m.keyMap)
	m.SetStyles(m.styles)
	return m
}

// Init fetches the first page.
func (m Model) Init() tea.Cmd { return m.fetchCmd() }

func (m Model) fetchCmd() tea.Cmd {
	fetch, ctx, id, cursor := m.fetch, m.parent, m.id, m.pg.next
	return func() tea.Msg {
		files, next, err := fetch(ctx, cursor)
		return pageMsg{id: id, cursor: cursor, files: files, next: next, err: err}
	}
}

// ID returns the unique ID of the view, which its messages carry.
func (m Model) ID() int { return m.id }

// Len returns the number of rows.
func (m Model) Len() int { return m.layout.Len() }

// Files returns the number of files fetched so far.
func (m Model) Files() int { return m.layout.Files() }

// Done reports whether the last page has been fetched.
func (m Model) Done() bool { return m.pg.done }

// Err returns the error of a failed fetch that has not been retried.
func (m Model) Err() error { return m.pg.err }

// Cursor returns the row of the cursor.
func (m Model) Cursor() int { return m.cursor }

// Position returns the line of a file the cursor is on; false when the
// cursor is on a header, a marker or a note, which stand for no line.
func (m Model) Position() (Pos, bool) { return m.layout.PosAt(m.cursor) }

// CurrentFile returns the file the cursor is in, and false when it is in
// none, such as with no files.
func (m Model) CurrentFile() (File, bool) {
	i, ok := m.layout.FileAt(m.cursor)
	if !ok {
		return File{}, false
	}
	return m.layout.File(i)
}

// SeekFile puts the cursor on the header of the file at path, and reports
// whether the file has been fetched. If it has not, and pages remain, the
// view keeps fetching pages until the file arrives, or the last page, and
// then puts the cursor there; a key press cancels that.
func (m *Model) SeekFile(path string) bool {
	return m.seek(Pos{Path: path}, true)
}

// Seek puts the cursor on the line p names, unfolding its file if it is
// folded. It reports whether the line is shown; when it is not, the cursor
// goes to the header of the file. If the file has not been fetched and
// pages remain, the view keeps fetching them until it arrives, or the last
// page, and then does the same; a key press cancels that.
func (m *Model) Seek(p Pos) bool {
	return m.seek(p, false)
}

func (m *Model) seek(p Pos, file bool) bool {
	m.pg.seek = nil
	i := m.layout.FileIndex(p.Path)
	if i < 0 {
		if !m.pg.done {
			m.pg.seek = &pendingSeek{pos: p, file: file}
			m.dirty = true
		}
		return false
	}
	if file {
		row, _ := m.layout.FileRow(i)
		m.moveTo(row)
		return true
	}
	m.layout.SetCollapsed(i, false)
	row, found := m.layout.Find(p)
	m.moveTo(row)
	return found
}

// moveTo puts the cursor on row, and the window around it if it is not in
// view, so that a jump shows what is around the row.
func (m *Model) moveTo(row int) {
	m.cursor = row
	if h := m.bodyHeight(); row < m.top || row >= m.top+h {
		m.top = row - h/2
	}
	m.left = 0
	m.dirty = true
	m.scroll()
}

// SetSize sets the width and height of the view. It only moves the window.
// Pages that the new window shows are fetched on the next Update.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = max(width, 0), max(height, 0)
	m.scroll()
	m.dirty = true
}

// SetWidth sets the width of the view.
func (m *Model) SetWidth(width int) { m.SetSize(width, m.height) }

// SetHeight sets the height of the view.
func (m *Model) SetHeight(height int) { m.SetSize(m.width, height) }

// Width returns the width of the view.
func (m Model) Width() int { return m.width }

// Height returns the height of the view.
func (m Model) Height() int { return m.height }

// Focus makes the view react to keys.
func (m *Model) Focus() { m.focused = true }

// Blur makes the view ignore keys.
func (m *Model) Blur() { m.focused = false }

// Focused reports whether the view reacts to keys.
func (m Model) Focused() bool { return m.focused }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keyMap = k
	m.syncKeys()
}

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keyMap }

// ShortHelp returns the bindings for the short help view.
func (m Model) ShortHelp() []key.Binding { return m.keyMap.ShortHelp() }

// FullHelp returns every binding with the state of the view applied: the
// retry key is enabled only after a failed fetch.
func (m Model) FullHelp() [][]key.Binding { return m.keyMap.FullHelp() }

// syncKeys enables the retry key while a fetch has failed.
func (m *Model) syncKeys() { keymap.Enable(&m.keyMap.Retry, m.pg.err != nil) }

// SetStyles sets the styles and renders the fragments that depend on them.
func (m *Model) SetStyles(s Styles) {
	m.styles = s
	cursor := termtext.Cells(s.CursorGlyph, 1)
	m.gutterFocused = s.Cursor.Render(cursor) + " "
	m.gutterBlurred = s.BlurredCursor.Render(cursor) + " "
	m.wrap = newWraps(s)
}

// Styles returns the styles.
func (m Model) Styles() Styles { return m.styles }
