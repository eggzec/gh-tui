// Package dev turns what a pull request links into the Development pane: a
// tree that folds, of the issues the pull request closes with what blocks each
// of them, and of the other pull requests linked to it.
//
// The component holds no keys of its own beyond those of the tree that draws
// the rows, and it opens nothing: what enter on a row asks for is left to the
// caller, which reads the row with [Model.Selected].
package dev

import "github.com/eggzec/gh-tui/internal/core"

// Context is the context of the keys of the pane.
const Context = "pull_development"

// Kind says what a row stands for.
type Kind int

// The kinds of rows.
const (
	// Header is the row of a group: what the pull request closes, or the
	// pull requests linked to it.
	Header Kind = iota
	// Issue is an issue the pull request closes.
	Issue
	// Blocker is an issue that blocks the one above it.
	Blocker
	// Pull is a pull request linked to this one.
	Pull
	// More says that the group has items that were not read.
	More
	// None says that a group has nothing in it.
	None
)

// Item is what a row stands for, for the caller to open or preview.
type Item struct {
	Kind Kind
	// Ref is the issue or pull request of a row of those kinds.
	Ref core.Reference
	// Group is "closes" or "linked" for a header.
	Group string
	// Title is the text of a header.
	Title string
}

// The groups of headers.
const (
	GroupCloses = "closes"
	GroupLinked = "linked"
	// GroupReferencedBy lists the items that name the one shown.
	GroupReferencedBy = "referenced"
	// GroupMentions lists the items the one shown names in its own text.
	GroupMentions = "mentions"
)
