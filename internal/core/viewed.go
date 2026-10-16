package core

// ViewedState is how the viewer marked a file of a pull request on GitHub.
type ViewedState int

// The states of a file's viewed mark.
const (
	// Unviewed is a file the viewer has not marked.
	Unviewed ViewedState = iota
	// Viewed is a file the viewer marked and that is as it was then.
	Viewed
	// ViewedChanged is a file the viewer marked and that changed since
	// (GitHub calls it dismissed).
	ViewedChanged
)

// PullViewed is the viewed marks of a pull request's files, as of its head:
// GitHub keeps them per head, not per range of commits.
type PullViewed struct {
	// States holds the files that have a mark. A path it lacks is Unviewed.
	States map[string]ViewedState
	// Capped is set when the pull request has more files than GitHub lists:
	// the last are missing.
	Capped bool
	// Supported is false on a server that keeps no viewed marks.
	Supported bool
}

// State returns the mark of the file at path.
func (v PullViewed) State(path string) ViewedState {
	return v.States[path]
}
