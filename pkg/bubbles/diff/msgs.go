package diff

// FilesMsg tells the parent that a page of files arrived and was added to
// the view of the ID that asked, so it can look at them, such as to count
// them. The view reads its own results; the parent need not pass this
// message back, but may.
type FilesMsg struct {
	ID    int
	Files []File
	// Done says that no page follows.
	Done bool
}

// pageMsg carries the result of one fetch back to the view that asked.
type pageMsg struct {
	id     int
	cursor string
	files  []File
	next   string
	err    error
}
