package tui

import (
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/internal/imgcaps"
	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// The stages of a cell query, after none.
const (
	// cellAskCell asks the size of a cell, XTWINOPS 16, with DA1 after it.
	cellAskCell = 1 + iota
	// cellAskWindow asks the size of the window in pixels, XTWINOPS 14,
	// with DA1 after it.
	cellAskWindow
)

// How the size of a cell was found, for the log.
const (
	cellFromCell     = "cell"
	cellFromWindow   = "window"
	cellFromTmux     = "tmux"
	cellFromFallback = "fallback"
)

// cellTimeoutMsg ends the wait for the answers to the cell query of id.
type cellTimeoutMsg struct{ id uint32 }

// cellSizedMsg carries the size of a cell, and how it was found. again
// asks once more, as a resize during the query wants.
type cellSizedMsg struct {
	cell  imgcaps.Cell
	from  string
	again bool
}

// sized returns the message of cell, found from, or of the fallback when
// cell is no size.
func sized(cell imgcaps.Cell, from string) cellSizedMsg {
	if !cell.Valid() {
		return cellSizedMsg{cell: imgcaps.FallbackCell, from: cellFromFallback}
	}
	return cellSizedMsg{cell: cell, from: from}
}

// cellQuery finds out the size of a cell in pixels from the terminal,
// outside tmux. It asks for the size of a cell, and failing that for the
// size of the window in pixels, which it divides by the columns and rows,
// and failing that settles on imgcaps.FallbackCell. DA1, which every
// terminal answers, follows each question, so that a question the
// terminal ignores costs one round trip, not a timeout.
//
// Inside tmux the terminal is never asked: tmux hands its answers to the
// active pane, which needn't be the app's. tmux says the size of its
// client's cells itself (imageProbe.askClient).
type cellQuery struct {
	// cols and rows are the size of the window in cells, and askCols and
	// askRows what they were when the query was asked, which the window's
	// pixels are divided by.
	cols, rows       int
	askCols, askRows int
	// id tells the timer of each query from those before it.
	id    uint32
	stage int
	cell  imgcaps.Cell
	from  string
	// stale counts the DA1s that queries which timed out still wait for.
	// A terminal answers in order, so until the last of them comes, what
	// it answers is theirs and not the query's: a late answer can't be
	// taken for a newer query's, nor end it early.
	stale int
	// again asks once more when the query in flight ends.
	again bool
}

// resize takes the size of the window in cells.
func (q *cellQuery) resize(cols, rows int) { q.cols, q.rows = cols, rows }

// ask starts a query, or has the one in flight ask again when it ends.
func (q *cellQuery) ask() tea.Cmd {
	if q.stage != 0 {
		q.again = true
		return nil
	}
	q.id++
	q.stage, q.cell, q.from = cellAskCell, imgcaps.Cell{}, ""
	q.askCols, q.askRows = q.cols, q.rows
	id := q.id
	return tea.Batch(
		tea.Raw(ansi.WindowOp(16)+ansi.RequestPrimaryDeviceAttributes),
		tea.Tick(probeTimeout, func(time.Time) tea.Msg { return cellTimeoutMsg{id: id} }),
	)
}

// update takes msg, if it is an answer to a query, and returns what to do
// next, and whether msg was the query's alone. A DA1 is the query's only
// while it asks or a timed-out one waits for it; the image probe takes
// the others.
func (q *cellQuery) update(msg tea.Msg) (cmd tea.Cmd, handled bool) {
	switch msg := msg.(type) {
	case uv.CellSizeEvent:
		if c := (imgcaps.Cell{Width: msg.Width, Height: msg.Height}); q.stale == 0 && q.stage == cellAskCell && c.Valid() {
			q.cell, q.from = c, cellFromCell
		}
		return nil, true
	case uv.PixelSizeEvent:
		if c, ok := imgcaps.CellFromWindow(msg.Width, msg.Height, q.askCols, q.askRows); q.stale == 0 && q.stage == cellAskWindow && ok {
			q.cell, q.from = c, cellFromWindow
		}
		return nil, true
	case uv.PrimaryDeviceAttributesEvent:
		switch {
		case q.stale > 0:
			q.stale--
			return nil, true
		case q.stage == cellAskCell:
			if q.cell.Valid() {
				return q.finish(), true
			}
			q.stage = cellAskWindow
			return tea.Raw(ansi.WindowOp(14) + ansi.RequestPrimaryDeviceAttributes), true
		case q.stage == cellAskWindow:
			return q.finish(), true
		}
		return nil, false
	case cellTimeoutMsg:
		if msg.id == q.id && q.stage != 0 {
			// The DA1 of the stage in flight may still come.
			q.stale++
			return q.finish(), true
		}
		return nil, true
	}
	return nil, false
}

// finish ends the query with what it found, or the fallback.
func (q *cellQuery) finish() tea.Cmd {
	msg := sized(q.cell, q.from)
	msg.again, q.again = q.again, false
	q.stage = 0
	return func() tea.Msg { return msg }
}

// graphicsDecided takes the images verdict: it logs it, tells the
// sections, and asks the size of a cell if images are drawn. The size
// found before is kept until the new one comes.
func (m *Model) graphicsDecided(msg graphicsDecidedMsg) tea.Cmd {
	g := msg.graphics
	g.Cell = m.graphics.Cell
	m.graphics = g
	m.term.imagesDecided(m.ctx, msg.attrs)
	return tea.Batch(m.broadcast(ui.GraphicsMsg{Graphics: m.graphics}), m.setGraphics(), m.askCells())
}

// askCells asks the size of a cell, if the terminal draws images: of tmux,
// inside it, and else of the terminal. While images are off only because
// tmux shows the session on several terminals, tmux is asked too, since
// a resize may come of one of them detaching.
func (m *Model) askCells() tea.Cmd {
	switch {
	case !m.graphics.Images && !m.graphics.Shared:
		return nil
	case m.graphics.Tmux:
		return m.images.askCells(m.ctx)
	}
	return m.cells.ask()
}

// cellSized takes the size of a cell, and tells the sections when it
// changed. The fallback doesn't replace a size that was found, since a
// query that timed out says nothing of the cells.
func (m *Model) cellSized(msg cellSizedMsg) tea.Cmd {
	var again tea.Cmd
	if msg.again {
		again = m.askCells()
	}
	if msg.cell == m.graphics.Cell || msg.from == cellFromFallback && m.graphics.Cell.Valid() {
		return again
	}
	m.graphics.Cell = msg.cell
	slog.InfoContext(m.ctx, "cell size", "span", "tui",
		"width", msg.cell.Width, "height", msg.cell.Height, "from", msg.from)
	return tea.Batch(m.broadcast(ui.GraphicsMsg{Graphics: m.graphics}), m.setGraphics(), again)
}
