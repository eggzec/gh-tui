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
	// Truncated reports that the list holds fewer items than exist, for
	// the API serves no more than a limit. A list sets it on its first
	// page, where the API tells how far the pages go.
	Truncated bool
	// Total is the number of items the whole list holds, as the API counts
	// them, or 0 when the page doesn't tell. A list the API says is empty has
	// no page to count, so 0 is never a count worth showing.
	Total int
}

// Last reports whether this is the final page.
func (p Page[T]) Last() bool {
	return p.Next == ""
}
