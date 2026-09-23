package feed

// chunkMsg carries the result of one fetch back to the feed that asked.
type chunkMsg[T any] struct {
	id     int
	index  int
	cursor string
	items  []T
	next   string
	err    error
}
