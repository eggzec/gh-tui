package diff

import (
	"fmt"
	"sort"
)

// state is one file of a Layout.
type state struct {
	file      File
	lines     int  // lines of the patch
	collapsed bool // only the header row shows
	parsed    bool
	hunks     []Hunk
	body      []Row // every row after the header, once parsed
	err       error
}

// bodyRows is how many rows the file has below its header, known without
// parsing: a row per patch line, a note in place of a missing patch, and a
// note after a patch that was cut short.
func (s *state) bodyRows() int {
	if s.lines > 0 {
		if s.file.Truncated {
			return s.lines + 1
		}
		return s.lines
	}
	if s.noteKind() != NoteNone {
		return 1
	}
	return 0
}

// noteKind is the note that stands in for a file's missing patch.
func (s *state) noteKind() NoteKind {
	f := &s.file
	switch {
	case f.Truncated || f.Additions+f.Deletions > 0:
		return NoteTruncated
	case f.Status == StatusRenamed || f.Status == StatusCopied:
		return NoteRenamed
	case f.Status == StatusUnchanged:
		return NoteNone
	}
	return NoteBinary
}

var noteText = map[NoteKind]string{
	NoteBinary:    "binary file or no text changes",
	NoteTruncated: "diff too large to show",
	NoteRenamed:   "renamed or copied; no text changes",
}

// Layout maps the rows of many files, one row per diff row: for each file a
// header, then its hunk headers, lines and notes. A file's rows are parsed
// when one of them is first asked for; until then it costs a count of its
// lines. A Layout is not safe for use by several goroutines.
type Layout struct {
	files        []state
	index        map[string]int // path to file
	starts       []int          // starts[i] is the first row of file i; the last is the total
	collapseOver int
	firstFiles   int
	onParse      func(path string) // called as each file parses; for tests
}

// NewLayout lays out files, copying them. It reads the options that concern
// a layout and ignores those of the view.
func NewLayout(files []File, opts ...Option) *Layout {
	l := &Layout{
		files: make([]state, len(files)),
		index: make(map[string]int, len(files)),
	}
	var set settings
	for _, o := range opts {
		o(&set)
	}
	l.collapseOver, l.firstFiles = set.collapseOver, set.firstFiles
	for i, f := range files {
		s := &l.files[i]
		s.file = f
		s.lines = patchLines(f.Patch)
		s.collapsed = l.collapseOver > 0 && s.lines > l.collapseOver
		if _, dup := l.index[f.Path]; !dup {
			l.index[f.Path] = i
		}
	}
	l.starts = make([]int, len(files)+1)
	l.resum(0)
	return l
}

// Append adds files after the last one, copying them. The rows of the files
// before keep their numbers, and so do their folds.
func (l *Layout) Append(files ...File) {
	from := len(l.files)
	for _, f := range files {
		s := state{file: f, lines: patchLines(f.Patch)}
		s.collapsed = l.collapseOver > 0 && s.lines > l.collapseOver
		if _, dup := l.index[f.Path]; !dup {
			l.index[f.Path] = len(l.files)
		}
		l.files = append(l.files, s)
	}
	l.starts = append(l.starts, make([]int, len(files))...)
	l.resum(from)
}

// rowsOf is how many rows file i shows now.
func (l *Layout) rowsOf(i int) int {
	s := &l.files[i]
	if s.collapsed {
		return 1
	}
	return 1 + s.bodyRows()
}

// resum recomputes the row offsets from file i on.
func (l *Layout) resum(i int) {
	for ; i < len(l.files); i++ {
		l.starts[i+1] = l.starts[i] + l.rowsOf(i)
	}
}

// Len is the number of rows.
func (l *Layout) Len() int {
	n := l.starts[len(l.files)]
	if l.firstFiles > 0 {
		n++
	}
	return n
}

// Files is the number of files.
func (l *Layout) Files() int { return len(l.files) }

// File returns file i as it was given; false when i is not a file.
//
// Every accessor takes an index that may be out of range, and never panics:
// it returns the zero value, with false where it has a bool to say so.
func (l *Layout) File(i int) (File, bool) {
	if i < 0 || i >= len(l.files) {
		return File{}, false
	}
	return l.files[i].file, true
}

// FileIndex is the index of the file at path, or -1. With two files at one
// path it is the first.
func (l *Layout) FileIndex(path string) int {
	if i, ok := l.index[path]; ok {
		return i
	}
	return -1
}

// FileRow is the row of file i's header. For i == Files() it is the row just
// past the last file's rows, where the first-files note is, if there is one.
// It is false for any other i.
func (l *Layout) FileRow(i int) (int, bool) {
	if i < 0 || i > len(l.files) {
		return 0, false
	}
	return l.starts[i], true
}

// FileAt is the index of the file that row i belongs to; false for a row
// that is no file's (the first-files note) or out of range.
func (l *Layout) FileAt(i int) (int, bool) {
	if i < 0 || i >= l.starts[len(l.files)] {
		return 0, false
	}
	// The first file whose next start is past i.
	return sort.Search(len(l.files), func(f int) bool { return l.starts[f+1] > i }), true
}

// Collapsed says whether file i shows only its header.
// It is false for an i that is not a file.
func (l *Layout) Collapsed(i int) bool {
	return i >= 0 && i < len(l.files) && l.files[i].collapsed
}

// SetCollapsed collapses file i to its header, or expands it, and does
// nothing for an i that is not a file. The rows after it move; rows of
// earlier files keep their numbers.
func (l *Layout) SetCollapsed(i int, collapsed bool) {
	if i < 0 || i >= len(l.files) || l.files[i].collapsed == collapsed {
		return
	}
	l.files[i].collapsed = collapsed
	l.resum(i)
}

// Hunks returns the hunks of file i, parsing it. It is empty for a file with
// no patch or one that did not parse, and for an i that is not a file.
func (l *Layout) Hunks(i int) []Hunk {
	if i < 0 || i >= len(l.files) {
		return nil
	}
	l.ensure(i)
	return append([]Hunk(nil), l.files[i].hunks...)
}

// ParseErr is why file i did not parse, or nil. It parses the file, and is
// nil for an i that is not a file.
func (l *Layout) ParseErr(i int) error {
	if i < 0 || i >= len(l.files) {
		return nil
	}
	l.ensure(i)
	return l.files[i].err
}

// RowAt returns row i, parsing its file if need be; false when i is not a
// row.
func (l *Layout) RowAt(i int) (Row, bool) {
	if i < 0 || i >= l.Len() {
		return Row{}, false
	}
	f, ok := l.FileAt(i)
	if !ok {
		return Row{Kind: KindNote, File: -1, Hunk: -1, Note: NoteFirstFiles,
			Text: fmt.Sprintf("showing the first %d files", l.firstFiles)}, true
	}
	local := i - l.starts[f]
	if local == 0 {
		return l.header(f), true
	}
	l.ensure(f)
	return l.files[f].body[local-1], true
}

func (l *Layout) header(f int) Row {
	return Row{Kind: KindFileHeader, File: f, Hunk: -1, Text: l.files[f].file.Path}
}

// ensure parses file i unless it has been.
func (l *Layout) ensure(i int) {
	s := &l.files[i]
	if s.parsed {
		return
	}
	s.parsed = true
	if s.lines == 0 {
		if k := s.noteKind(); k != NoteNone {
			s.body = []Row{{Kind: KindNote, File: i, Hunk: -1, Note: k, Text: noteText[k]}}
		}
		return
	}
	if l.onParse != nil {
		l.onParse(s.file.Path)
	}
	s.hunks, s.body, s.err = Parse(s.file.Patch)
	for r := range s.body {
		s.body[r].File = i
	}
	if s.file.Truncated {
		s.body = append(s.body, Row{Kind: KindNote, File: i, Hunk: -1, Note: NoteTruncated, Text: noteText[NoteTruncated]})
	}
}
