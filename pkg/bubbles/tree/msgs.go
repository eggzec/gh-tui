package tree

// OpenMsg asks the parent to open a leaf, for example to show a preview. The
// tree sends it when enter is pressed on a leaf.
type OpenMsg struct {
	// ID is the ID of the tree that sent the message.
	ID int
	// Node is the leaf to open.
	Node Node
}

// childrenMsg carries the result of one load back to the tree that asked.
// The sequence number tells results of older loads of the same node apart.
type childrenMsg struct {
	tree int
	node string
	seq  int
	kids []Node
	err  error
}
