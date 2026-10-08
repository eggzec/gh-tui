// Package diff turns the per-file unified patches of a pull request into
// rows: the pure core of a diff view, with no terminal or Bubble Tea in it.
//
// A patch is the text GitHub's pull request files API gives for one file: the
// hunks only, with no "diff --git" or "---"/"+++" header. A [Layout] holds
// many files and maps a row number to a file header, a hunk header, a line or
// a note, and a [Pos] (a side and a line number of a file) to a row and back.
// Files are parsed the first time one of their rows is asked for.
package diff

// Status is how a file changed, in the words GitHub uses.
type Status string

// The statuses of a changed file.
const (
	StatusAdded     Status = "added"
	StatusRemoved   Status = "removed"
	StatusModified  Status = "modified"
	StatusRenamed   Status = "renamed"
	StatusCopied    Status = "copied"
	StatusChanged   Status = "changed"
	StatusUnchanged Status = "unchanged"
)

// File is one changed file and its patch.
type File struct {
	Path    string // the path after the change
	OldPath string // the path before a rename or copy; empty otherwise
	Status  Status
	// Additions and Deletions are the counts the source reported, which
	// stay known when the patch itself is missing.
	Additions int
	Deletions int
	// Patch is the unified patch, hunks only. It is empty for a binary file,
	// a rename without changes, and a diff too large to give.
	Patch string
	// Truncated says the source cut the patch short or left it out for size.
	Truncated bool
}

// Side is one side of a diff: the old file or the new one.
type Side int

// The two sides of a diff.
const (
	Old Side = iota
	New
)

// Pos is a line of a file on one side. Line is 1-based.
type Pos struct {
	Path string
	Side Side
	Line int
}

// Hunk is the range a hunk header names. A count the header leaves out is 1.
type Hunk struct {
	OldStart, OldLines int
	NewStart, NewLines int
	Section            string // the text after the closing @@, such as a function name
}

// Kind says what a row is.
type Kind int

// The kinds of rows.
const (
	KindFileHeader Kind = iota
	KindHunkHeader
	KindContext
	KindAdded
	KindDeleted
	KindNoNewline // "\ No newline at end of file"
	KindNote      // a note in place of patch lines; see NoteKind
	KindRaw       // a patch line shown as it is, since the patch did not parse
)

// NoteKind says why a note row stands in for patch lines.
type NoteKind int

// The notes a row can carry.
const (
	NoteNone NoteKind = iota
	// NoteBinary: no patch, nothing counted as changed, and not a rename or
	// copy. That is a binary file, but also an empty file or a mode change,
	// since none of them has text to show.
	NoteBinary
	// NoteTruncated: lines changed but the patch is missing or cut short.
	NoteTruncated
	// NoteRenamed: a rename or copy with nothing counted as changed. GitHub
	// gives the same for a pure rename and a renamed binary file.
	NoteRenamed
	// NoteFirstFiles: the last row, when the layout was asked to say that it
	// holds only the first N files.
	NoteFirstFiles
)

// Row is one rendered row of the diff.
type Row struct {
	Kind Kind
	// File is the index of the file the row belongs to, or -1 for a row of
	// no file (the NoteFirstFiles row).
	File int
	// Hunk is the index of the row's hunk within its file, or -1.
	Hunk int
	// Old and New are the line numbers on each side; 0 where the row has
	// none (an added line has no old number). A KindNoNewline row has those
	// of the line it follows, so they say which side lacks the newline.
	Old, New int
	// Text is the row's text without the leading +, - or space of a line;
	// the whole header for a header; the note's words for a note. It keeps
	// control characters, which the renderer must sanitize.
	Text string
	Note NoteKind
	// Err is why the patch did not parse; set on every KindRaw row.
	Err string
}
