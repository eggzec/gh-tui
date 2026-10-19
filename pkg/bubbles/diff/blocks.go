package diff

import (
	"cmp"
	"slices"
	"sort"
	"strings"
)

// Block is a run of rows that the caller puts in the diff, right after the
// row of a line: a note on that line, say. A Block is a value the caller
// builds; the layout keeps a copy.
type Block struct {
	// ID names the block, and must be unique among the blocks given together:
	// a later block with the ID of an earlier one is ignored. The fold state
	// of an ID outlives the calls that set the blocks.
	ID string
	// Anchor is the line the block sits under. A block under line 0, or under
	// a line the diff does not show, sits right under the header of its file;
	// [Layout.HeaderBlocks] lists them. The block of a file that was
	// not fetched yet waits for it. A context line is both of its sides.
	Anchor Pos
	// Render returns the lines of the block, each of at most width cells
	// (longer ones are cut), and with its own styles. The rows are not
	// sanitized, so the lines must hold no control characters but escape
	// sequences, and no newlines (a line that has some is split in them). A
	// folded block is usually one line. A block that renders no line shows one
	// blank line, so that there is a row to fold it from.
	Render func(width int, folded bool) []string
	// Folded is the state the block starts in. It is read the first time its
	// ID is given.
	Folded bool
	// OnlyAtLine has the block show only while its Anchor is a line the diff
	// shows. Without it a block under a line that is not shown sits under the
	// header of its file. A block of this kind makes its file parse as soon as
	// the block is given, as it takes its rows only once it knows the lines.
	OnlyAtLine bool
	// Passive keeps the cursor off the rows of the block: moves by a row, a
	// page or to the ends pass over them, and [Model.NextBlock] and
	// [Model.PrevBlock] never stop at the block. The rows are still drawn.
	Passive bool
}

// placed is a block of a file once it has been rendered, and placed among the
// rows of the file once the file has been parsed.
type placed struct {
	blk  int      // index in the layout's blocks
	at   int      // body row the block follows; -1 for the header
	off  int      // rows of the blocks before this one in the file
	rows []string // the lines as rendered, at least one
}

// SetBlocks replaces the blocks of the layout. The rows of the files that had
// blocks or have them now move; the rows of files before them keep their
// numbers. Blocks of the same line stack in the order they are given in, each
// ID keeps the fold state it has, and the blocks of a file that is not in the
// layout wait for it. A file collapsed to its header hides its blocks.
func (l *Layout) SetBlocks(blocks []Block) { l.applyBlocks(blocks) }

// SetBlockWidth sets the width the blocks are rendered at, which they are
// rendered at again if it changed. A [Model] sets it from its own width. It
// starts at 0, which shows each block as one blank line, as the lines cannot
// hold anything.
func (l *Layout) SetBlockWidth(width int) { l.setBlockWidth(width) }

// BlockAt returns the block that row i is a row of; false for any other row.
func (l *Layout) BlockAt(i int) (Block, bool) {
	r, ok := l.RowAt(i)
	if !ok || r.Kind != KindBlock {
		return Block{}, false
	}
	return l.blocks[l.blockIdx[r.Block]], true
}

// BlockRow returns the first row of the block with ID id. It is false for an
// ID that is not a block, that is of a file not in the layout, or that is in
// a collapsed file, whose rows are hidden; the row is then the file's header,
// or -1 for the first two.
func (l *Layout) BlockRow(id string) (int, bool) {
	f, row, _, ok := l.blockSpan(id)
	if !ok {
		return -1, false
	}
	if l.files[f].collapsed {
		return l.starts[f], false
	}
	return row, true
}

// BlockFolded says whether the block with ID id is folded; false if there is
// none.
func (l *Layout) BlockFolded(id string) bool { return l.folded[id] }

// SetBlockFolded folds or unfolds the block with ID id, and reports whether
// there is one that changed.
func (l *Layout) SetBlockFolded(id string, folded bool) bool {
	_, changed := l.setFold(id, folded)
	return changed
}

// HeaderBlocks returns the IDs of the blocks that sit right under the header
// of their file: those anchored at line 0, and those anchored at a line the
// diff does not show, such as one that a later push changed. It parses the
// files that have blocks.
func (l *Layout) HeaderBlocks() []string {
	var ids []string
	for _, f := range l.blockFiles {
		l.ensure(f)
		for _, p := range l.files[f].blocks {
			if p.at < 0 {
				ids = append(ids, l.blocks[p.blk].ID)
			}
		}
	}
	return ids
}

// hasBlocks reports whether any file has blocks.
func (l *Layout) hasBlocks() bool { return len(l.blockFiles) > 0 }

// applyBlocks does SetBlocks, and returns the files whose rows changed, in
// ascending order.
func (l *Layout) applyBlocks(blocks []Block) []int {
	touched := slices.Clone(l.blockFiles)
	for _, f := range l.blockFiles {
		s := &l.files[f]
		s.blocks, s.held, s.blockRows = nil, nil, 0
	}
	folded := make(map[string]bool, len(blocks))
	l.blocks = make([]Block, 0, len(blocks))
	l.blockIdx = make(map[string]int, len(blocks))
	l.byPath = make(map[string][]int)
	for _, b := range blocks {
		if _, dup := l.blockIdx[b.ID]; dup {
			continue
		}
		folded[b.ID] = b.Folded
		if was, ok := l.folded[b.ID]; ok {
			folded[b.ID] = was
		}
		l.blockIdx[b.ID] = len(l.blocks)
		l.byPath[b.Anchor.Path] = append(l.byPath[b.Anchor.Path], len(l.blocks))
		l.blocks = append(l.blocks, b)
	}
	l.folded = folded
	l.blockFiles = l.blockFiles[:0:0]
	for path, idx := range l.byPath {
		f, ok := l.index[path]
		if !ok {
			continue
		}
		s := &l.files[f]
		for _, i := range idx {
			s.blocks = append(s.blocks, placed{blk: i})
		}
		l.blockFiles = append(l.blockFiles, f)
		touched = append(touched, f)
	}
	slices.Sort(l.blockFiles)
	slices.Sort(touched)
	touched = slices.Compact(touched)
	for _, f := range touched {
		l.renderBlocks(f)
		if l.files[f].parsed {
			l.place(f)
		} else if l.onlyAtLine(f) {
			l.ensure(f)
		}
	}
	if len(touched) > 0 {
		l.resum(touched[0])
	}
	return touched
}

// attach gives file i, just appended, the blocks that wait for its path.
func (l *Layout) attach(i int) {
	s := &l.files[i]
	idx := l.byPath[s.file.Path]
	if len(idx) == 0 || l.index[s.file.Path] != i {
		return
	}
	for _, b := range idx {
		s.blocks = append(s.blocks, placed{blk: b})
	}
	l.renderBlocks(i)
	if l.onlyAtLine(i) {
		l.ensure(i)
	}
	l.blockFiles = append(l.blockFiles, i)
}

// onlyAtLine says whether file f has a block that sits only under a line that
// is shown.
func (l *Layout) onlyAtLine(f int) bool {
	return slices.ContainsFunc(l.files[f].blocks, func(p placed) bool { return l.blocks[p.blk].OnlyAtLine })
}

// setBlockWidth does SetBlockWidth, and returns the files whose rows may have
// changed.
func (l *Layout) setBlockWidth(width int) []int {
	width = max(width, 0)
	if width == l.blockWidth {
		return nil
	}
	l.blockWidth = width
	for _, f := range l.blockFiles {
		l.renderBlocks(f)
	}
	if len(l.blockFiles) > 0 {
		l.resum(l.blockFiles[0])
	}
	return l.blockFiles
}

// setFold does SetBlockFolded, and returns the file whose rows moved, or -1.
func (l *Layout) setFold(id string, folded bool) (file int, changed bool) {
	i, ok := l.blockIdx[id]
	if !ok || l.folded[id] == folded {
		return -1, false
	}
	l.folded[id] = folded
	f, ok := l.index[l.blocks[i].Anchor.Path]
	if !ok {
		return -1, true
	}
	s := &l.files[f]
	for k := range s.blocks {
		if p := &s.blocks[k]; p.blk == i {
			s.blockRows -= len(p.rows)
			p.rows = l.renderBlock(i)
			s.blockRows += len(p.rows)
			break
		}
	}
	s.offsets()
	l.resum(f)
	return f, true
}

// renderBlocks renders the blocks of file f again, and counts their rows.
func (l *Layout) renderBlocks(f int) {
	s := &l.files[f]
	s.blockRows = 0
	for k := range s.blocks {
		p := &s.blocks[k]
		p.rows = l.renderBlock(p.blk)
		s.blockRows += len(p.rows)
	}
	s.offsets()
}

// renderBlock returns the lines of block i, split so that none holds a
// newline, and never none.
func (l *Layout) renderBlock(i int) []string {
	b := &l.blocks[i]
	var lines []string
	if b.Render != nil && l.blockWidth > 0 {
		lines = b.Render(l.blockWidth, l.folded[b.ID])
	}
	if slices.ContainsFunc(lines, func(s string) bool { return strings.Contains(s, "\n") }) {
		split := make([]string, 0, len(lines)+1)
		for _, s := range lines {
			split = append(split, strings.Split(s, "\n")...)
		}
		lines = split
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return slices.Clone(lines)
}

// place decides which row of file f each of its blocks follows, and sorts them
// by it. The file must be parsed.
func (l *Layout) place(f int) {
	s := &l.files[f]
	if len(s.held) > 0 {
		// The lines may have changed, as when the file is shown whole.
		for _, p := range s.held {
			p.rows = l.renderBlock(p.blk)
			s.blocks = append(s.blocks, p)
		}
		s.held = nil
	}
	if len(s.blocks) == 0 {
		return
	}
	type line struct {
		side Side
		n    int
	}
	rowOf := make(map[line]int)
	add := func(k line, row int) {
		if _, ok := rowOf[k]; !ok {
			rowOf[k] = row
		}
	}
	for i := range s.body {
		switch r := &s.body[i]; r.Kind {
		case KindContext:
			add(line{OldSide, r.Old}, i)
			add(line{NewSide, r.New}, i)
		case KindAdded:
			add(line{NewSide, r.New}, i)
		case KindDeleted:
			add(line{OldSide, r.Old}, i)
		case KindFileHeader, KindHunkHeader, KindNoNewline, KindNote, KindRaw, KindBlock:
		}
	}
	for k := range s.blocks {
		p := &s.blocks[k]
		a := l.blocks[p.blk].Anchor
		p.at = -1
		if row, ok := rowOf[line{a.Side, a.Line}]; ok && a.Line > 0 {
			p.at = row
		}
	}
	s.blocks = slices.DeleteFunc(s.blocks, func(p placed) bool {
		if p.at >= 0 || !l.blocks[p.blk].OnlyAtLine {
			return false
		}
		s.held = append(s.held, p)
		return true
	})
	s.blockRows = 0
	for _, p := range s.blocks {
		s.blockRows += len(p.rows)
	}
	slices.SortStableFunc(s.blocks, func(a, b placed) int { return cmp.Or(a.at-b.at, a.blk-b.blk) })
	s.offsets()
}

// offsets counts the rows of the blocks before each block.
func (s *state) offsets() {
	off := 0
	for k := range s.blocks {
		s.blocks[k].off = off
		off += len(s.blocks[k].rows)
	}
}

// locate says what row i of the rows of the file below its header is: a row
// of block k, the sub-th, or, with k = -1, the row of the body numbered body.
// For a row of a block, body is the row the block follows. The file must be
// parsed.
func (s *state) locate(i int) (body, k, sub int) {
	if len(s.blocks) == 0 {
		return i, -1, 0
	}
	// The first block that ends after row i.
	k = sort.Search(len(s.blocks), func(k int) bool {
		p := &s.blocks[k]
		return p.at+1+p.off+len(p.rows) > i
	})
	if k == len(s.blocks) {
		return i - s.blockRows, -1, 0
	}
	p := &s.blocks[k]
	if start := p.at + 1 + p.off; i >= start {
		return p.at, k, i - start
	}
	return i - p.off, -1, 0
}

// shift is how many block rows come before row b of the body.
func (s *state) shift(b int) int {
	k := sort.Search(len(s.blocks), func(k int) bool { return s.blocks[k].at >= b })
	if k == len(s.blocks) {
		return s.blockRows
	}
	return s.blocks[k].off
}

// rowIn returns row i of file f below its header, which must be parsed.
func (l *Layout) rowIn(f, i int) Row {
	s := &l.files[f]
	body, k, sub := s.locate(i)
	if k < 0 {
		return s.body[body]
	}
	p := &s.blocks[k]
	return Row{Kind: KindBlock, File: f, Hunk: -1, Text: p.rows[sub], Block: l.blocks[p.blk].ID}
}

// bodyIndex is the index in the body of the row that row i below the header of
// file f shows, which is not a row of a block.
func (l *Layout) bodyIndex(f, i int) int {
	body, _, _ := l.files[f].locate(i)
	return body
}

// fullRows is the number of rows file f has below its header when it is not
// collapsed.
func (l *Layout) fullRows(f int) int {
	s := &l.files[f]
	return s.bodyRows() + s.blockRows
}

// blockSpan returns the file of the block with ID id, its first row, and how
// many rows it has, as if its file were not collapsed. It parses the file.
func (l *Layout) blockSpan(id string) (file, first, n int, ok bool) {
	i, ok := l.blockIdx[id]
	if !ok {
		return 0, 0, 0, false
	}
	f, ok := l.index[l.blocks[i].Anchor.Path]
	if !ok {
		return 0, 0, 0, false
	}
	l.ensure(f)
	for k := range l.files[f].blocks {
		if p := &l.files[f].blocks[k]; p.blk == i {
			return f, l.starts[f] + 1 + p.at + 1 + p.off, len(p.rows), true
		}
	}
	return 0, 0, 0, false
}

// findBlock returns the first block that pred accepts after row, or the last
// one before it with dir < 0. A block counts as before or after by its first
// row, wherever its file is collapsed or not. It parses the files that have
// blocks, up to the one it finds.
func (l *Layout) findBlock(row, dir int, pred func(Block) bool) (string, bool) {
	f, ok := l.FileAt(row)
	pos := 0 // the row among those of its file below the header
	switch {
	case ok:
		pos = row - l.starts[f] - 1
	case dir > 0:
		return "", false
	default:
		f = len(l.files)
	}
	if dir > 0 {
		for _, g := range l.blockFiles[sort.SearchInts(l.blockFiles, f):] {
			l.ensure(g)
			for _, p := range l.files[g].blocks {
				if g == f && p.at+1+p.off <= pos {
					continue
				}
				if b := l.blocks[p.blk]; !b.Passive && pred(b) {
					return b.ID, true
				}
			}
		}
		return "", false
	}
	for i := sort.SearchInts(l.blockFiles, f+1) - 1; i >= 0; i-- {
		g := l.blockFiles[i]
		l.ensure(g)
		for _, p := range slices.Backward(l.files[g].blocks) {
			if g == f && p.at+1+p.off >= pos {
				continue
			}
			if b := l.blocks[p.blk]; !b.Passive && pred(b) {
				return b.ID, true
			}
		}
	}
	return "", false
}

// rowRef names a row in a way that survives blocks coming and going, for a
// cursor to stay on its row.
type rowRef struct {
	file int    // the row's file; -1 for none, and then end counts from the last row
	end  int    // rows from the end of the layout
	body int    // the row in the body; -1 for the header
	id   string // the block, if the row is one of one
	sub  int    // which row of the block
}

// refOf names row i.
func (l *Layout) refOf(i int) rowRef {
	f, ok := l.FileAt(i)
	if !ok {
		return rowRef{file: -1, end: l.Len() - i}
	}
	local := i - l.starts[f]
	if local == 0 {
		return rowRef{file: f, body: -1}
	}
	l.ensure(f)
	body, k, sub := l.files[f].locate(local - 1)
	if k < 0 {
		return rowRef{file: f, body: body}
	}
	return rowRef{file: f, body: body, id: l.blocks[l.files[f].blocks[k].blk].ID, sub: sub}
}

// rowOf is the row a reference names now. A block that is gone leaves the row
// it followed.
func (l *Layout) rowOf(r rowRef) int {
	if r.file < 0 || r.file >= len(l.files) {
		return l.Len() - r.end
	}
	start := l.starts[r.file]
	if l.files[r.file].collapsed {
		return start
	}
	if r.id != "" {
		if _, first, n, ok := l.blockSpan(r.id); ok {
			return first + min(r.sub, n-1)
		}
	}
	if r.body < 0 {
		return start
	}
	l.ensure(r.file)
	return start + 1 + r.body + l.files[r.file].shift(r.body)
}
