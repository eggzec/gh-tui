package feed

// chunkMsg carries the result of one fetch back to the feed that asked. The
// generation tells results from before a Reset or Reload apart.
type chunkMsg[T any] struct {
	id     int
	gen    int
	index  int
	cursor string
	items  []T
	next   string
	err    error
}
