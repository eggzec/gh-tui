package graph

// SelectMsg tells the parent that the cursor landed on another commit, for
// example to show or prefetch its diff. The graph sends it when the first
// commit loads, and whenever a key moves the cursor to a different commit;
// holding a key sends one for every row passed, so a parent that fetches on
// it may want to wait for the cursor to rest.
type SelectMsg struct {
	// ID is the ID of the graph that sent the message.
	ID int
	// Commit is the commit under the cursor.
	Commit Commit
}

// ChosenMsg asks the parent to open a commit. The graph sends it when enter
// is pressed.
type ChosenMsg struct {
	// ID is the ID of the graph that sent the message.
	ID int
	// Commit is the commit under the cursor.
	Commit Commit
}

// chunkMsg carries the result of one fetch back to the graph that asked. The
// generation tells results from before a Reset apart.
type chunkMsg struct {
	id      int
	gen     int
	cursor  string
	commits []Commit
	next    string
	err     error
}
