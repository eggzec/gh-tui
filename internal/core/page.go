package core

// Page is one page of a paginated list. An empty Next means there are no
// more pages.
type Page[T any] struct {
	Items []T
	Next  string
}

// Last reports whether this is the final page.
func (p Page[T]) Last() bool {
	return p.Next == ""
}
