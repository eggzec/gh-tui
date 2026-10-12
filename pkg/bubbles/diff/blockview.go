package diff

// SetBlocks replaces the blocks shown in the diff; see [Block] and
// [Layout.SetBlocks]. The cursor and the window stay on the rows they were
// on, and a search shown reads the text of the new blocks.
func (m *Model) SetBlocks(blocks []Block) {
	h := m.holdRows()
	files := m.layout.applyBlocks(blocks)
	m.searchRefile(files...)
	m.restore(h)
	m.dirty = true
}

// HeaderBlocks returns the IDs of the blocks that sit right under their file's
// header, because they are anchored at line 0 or at a line the diff does not
// show; see [Layout.HeaderBlocks]. The blocks of a file that has not been
// fetched are not among them.
func (m Model) HeaderBlocks() []string { return m.layout.HeaderBlocks() }

// CursorBlock returns the block the cursor is on, and false when it is on a
// row of the diff.
func (m Model) CursorBlock() (Block, bool) { return m.layout.BlockAt(m.cursor) }

// BlockFolded says whether the block with ID id is folded.
func (m Model) BlockFolded(id string) bool { return m.layout.BlockFolded(id) }

// SetBlockFolded folds or unfolds the block with ID id, as the fold key does
// on one of its rows, and reports whether there is a block that changed. The
// cursor, if it is on the block, goes to its first row.
func (m *Model) SetBlockFolded(id string, folded bool) bool {
	b, onIt := m.layout.BlockAt(m.cursor)
	onIt = onIt && b.ID == id
	h := m.holdRows()
	f, changed := m.layout.setFold(id, folded)
	if !changed {
		return false
	}
	if f >= 0 {
		m.searchRefile(f)
	}
	m.restore(h)
	if row, ok := m.layout.BlockRow(id); ok && onIt {
		m.cursor = row
		m.scroll()
	}
	m.dirty = true
	return true
}

// SeekBlock puts the cursor on the first row of the block with ID id,
// unfolding its file if it is folded, and reports whether the block is shown.
// If the block's file has not been fetched, and pages remain, the view keeps
// fetching pages until the file arrives, or the last page, and then does the
// same; a key press cancels that.
func (m *Model) SeekBlock(id string) bool {
	m.pg.seek = nil
	i, ok := m.layout.blockIdx[id]
	if !ok {
		return false
	}
	path := m.layout.blocks[i].Anchor.Path
	f := m.layout.FileIndex(path)
	if f < 0 {
		if !m.pg.done {
			m.pg.seek = &pendingSeek{pos: Pos{Path: path}, block: id}
			m.dirty = true
		}
		return false
	}
	m.layout.SetCollapsed(f, false)
	row, ok := m.layout.BlockRow(id)
	if ok {
		m.moveTo(row)
	}
	return ok
}

// NextBlock puts the cursor on the first block after the cursor that pred
// accepts, unfolding its file if it is folded, and reports whether there is
// one. Blocks go by the row they start on, so from inside a block the next
// one is the one after it. It does not wrap round.
func (m *Model) NextBlock(pred func(Block) bool) bool {
	id, ok := m.layout.findBlock(m.cursor, 1, pred)
	return ok && m.SeekBlock(id)
}

// PrevBlock puts the cursor on the last block before the cursor that pred
// accepts, and reports whether there is one. From inside a block, the block
// itself does not count, so it is the one before it. It does not wrap round.
func (m *Model) PrevBlock(pred func(Block) bool) bool {
	row := m.cursor
	if b, ok := m.layout.BlockAt(row); ok {
		if first, ok := m.layout.BlockRow(b.ID); ok {
			row = first
		}
	}
	id, ok := m.layout.findBlock(row, -1, pred)
	return ok && m.SeekBlock(id)
}

// held names the rows the cursor and the top of the window are on. It is a
// value, not a closure over the model, so that the model does not escape to
// the heap on every Update.
type held struct{ cursor, top rowRef }

// holdRows names the rows the cursor and the top of the window are on now.
func (m *Model) holdRows() held {
	return held{m.layout.refOf(m.cursor), m.layout.refOf(m.top)}
}

// restore puts the cursor and the top of the window back on the rows h names,
// whichever rows moved since.
func (m *Model) restore(h held) {
	m.cursor, m.top = m.layout.rowOf(h.cursor), m.layout.rowOf(h.top)
	m.scroll()
}

// setBlockWidth renders the blocks at the width of the view, keeping the
// cursor and the window where they are.
func (m *Model) setBlockWidth() {
	if !m.layout.hasBlocks() {
		m.layout.setBlockWidth(m.blockWidth())
		return
	}
	h := m.holdRows()
	m.searchRefile(m.layout.setBlockWidth(m.blockWidth())...)
	m.restore(h)
}
