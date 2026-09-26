package pulls

// again returns q for the read that follows one that came back Stale.
func (q ListQuery) again() ListQuery {
	q.Again = true
	return q
}
