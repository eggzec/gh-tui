package core

// Page is one page of a paginated list. An empty Next means there are no
// more pages.
type Page[T any] struct {
	Items []T
	Next  string
	// Stale reports that the page was kept by an earlier session and is
	// served before GitHub was asked whether it changed. Reading it again
	// with the query's Again set asks GitHub; any other read is served the
	// kept page again.
	Stale bool
	// Offline reports that GitHub couldn't be reached, so the page is the
	// one read last.
	Offline bool
	// Limited reports that GitHub rate limited the read, so the page is
	// the one read last.
	Limited bool
}

// Last reports whether this is the final page.
func (p Page[T]) Last() bool {
	return p.Next == ""
}
