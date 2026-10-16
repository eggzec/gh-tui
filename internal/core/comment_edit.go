package core

import "time"

// Edited is what an edit of a comment, a review summary or a reply returns:
// the node, its body as GitHub keeps it, and when it was last edited.
type Edited struct {
	ID   string
	Body string
	// At is the node's lastEditedAt, zero if GitHub gives none.
	At time.Time
}
