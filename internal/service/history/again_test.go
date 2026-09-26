package history

// again returns q for the read that follows one that came back Stale.
func (q BranchesQuery) again() BranchesQuery {
	q.Again = true
	return q
}

// again returns q for the read that follows one that came back Stale.
func (q CommitsQuery) again() CommitsQuery {
	q.Again = true
	return q
}
