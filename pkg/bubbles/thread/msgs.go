package thread

import tea "charm.land/bubbletea/v2"

// fetchedMsg carries a fetched chunk back to the instance that asked for it.
type fetchedMsg[T any] struct {
	id    int64
	gen   int
	seq   int
	index int // the chunk it fills, or len(chunks) at request time for the tail
	tail  bool

	cursor string
	items  []T
	next   string
	err    error
}

// fetchCmd fetches the chunk after cursor. It copies what it needs, so the
// command never touches the model.
func (m *Model[T]) fetchCmd(index, seq int, tail bool, cursor string) tea.Cmd {
	ctx, fetch, id, gen := m.ctx, m.fetch, m.id, m.gen
	return func() tea.Msg {
		items, next, err := fetch(ctx, cursor)
		return fetchedMsg[T]{
			id: id, gen: gen, seq: seq, index: index, tail: tail,
			cursor: cursor, items: items, next: next, err: err,
		}
	}
}
