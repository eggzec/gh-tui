package actions

// again returns q for the read that follows one that came back Stale.
func (q RunsQuery) again() RunsQuery {
	q.Again = true
	return q
}
