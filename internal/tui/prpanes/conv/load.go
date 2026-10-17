package conv

import (
	"context"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// maxCached bounds the walk over cached pages, so a cursor that loops in a
// cache cannot spin it.
const maxCached = 1000

// Source reads one pull request's conversation. The host adapts its services
// to these functions; the cached reads do no I/O, and a nil function stands
// for a source that has nothing to read.
type Source struct {
	CachedTimeline func(cursor string) (core.Page[core.TimelineItem], bool)
	Timeline       func(ctx context.Context, cursor string) (core.Page[core.TimelineItem], error)
	CachedThreads  func(cursor string) (core.ThreadPage, bool)
	Threads        func(ctx context.Context, cursor string) (core.ThreadPage, error)
	// CachedCommits and Commits read the commits of the pull request, whose
	// checks mark the commits of a push. Without them a commit shows no icon.
	CachedCommits func() (core.PullCommits, bool)
	Commits       func(ctx context.Context) (core.PullCommits, error)
}

// which names one of the two sources of a conversation.
type which int

const (
	timeline which = iota
	threads
)

// chunk is what reading one or more pages of a source brought.
type chunk struct {
	items   []core.TimelineItem
	threads []core.Thread
	// total is how many threads the pull request has.
	total     int
	truncated bool
	// next is the cursor of the page after the last one read.
	next string
	// have reports that at least one page was read.
	have                    bool
	offline, limited, stale bool
}

func (c *chunk) addItems(p core.Page[core.TimelineItem]) {
	c.have = true
	c.items = append(c.items, p.Items...)
	c.next = p.Next
	c.truncated = c.truncated || p.Truncated
	c.offline, c.limited, c.stale = c.offline || p.Offline, c.limited || p.Limited, c.stale || p.Stale
}

func (c *chunk) addThreads(p core.ThreadPage) {
	c.have = true
	c.threads = append(c.threads, p.Threads...)
	c.total = p.Total
	c.next = p.Next
	c.offline, c.limited = c.offline || p.Offline, c.limited || p.Limited
}

// loadMsg carries what a read of a source brought to the model that asked.
type loadMsg struct {
	model, seq int
	src        which
	// cached marks the pages that were kept, and first the first page of a
	// read from the network, which replaces what the model has.
	cached, first bool
	// cursor is the one a read from the network started at.
	cursor string
	chunk  chunk
	err    error
}

// cachedCmd walks the cached pages of src, which costs no I/O.
func (m Model) cachedCmd(src which) tea.Cmd {
	id, seq, s := m.id, m.seq, m.src
	return func() tea.Msg {
		var c chunk
		cursor := ""
		for range maxCached {
			switch src {
			case timeline:
				if s.CachedTimeline == nil {
					return loadMsg{model: id, seq: seq, src: src, cached: true}
				}
				p, ok := s.CachedTimeline(cursor)
				if !ok {
					return loadMsg{model: id, seq: seq, src: src, cached: true, chunk: c}
				}
				c.addItems(p)
			case threads:
				if s.CachedThreads == nil {
					return loadMsg{model: id, seq: seq, src: src, cached: true}
				}
				p, ok := s.CachedThreads(cursor)
				if !ok {
					return loadMsg{model: id, seq: seq, src: src, cached: true, chunk: c}
				}
				c.addThreads(p)
			}
			if c.next == "" || c.next == cursor {
				break
			}
			cursor = c.next
		}
		return loadMsg{model: id, seq: seq, src: src, cached: true, chunk: c}
	}
}

// readCmd reads the page of src after cursor from the network.
func (m Model) readCmd(src which, cursor string, first bool) tea.Cmd {
	id, seq, ctx, s := m.id, m.seq, m.ctx, m.src
	return func() tea.Msg {
		msg := loadMsg{model: id, seq: seq, src: src, first: first, cursor: cursor}
		switch src {
		case timeline:
			if s.Timeline == nil {
				msg.chunk.have = true
				return msg
			}
			p, err := s.Timeline(ctx, cursor)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.chunk.addItems(p)
		case threads:
			if s.Threads == nil {
				msg.chunk.have = true
				return msg
			}
			p, err := s.Threads(ctx, cursor)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.chunk.addThreads(p)
		}
		return msg
	}
}

// feed is what the model knows of one source.
type feed struct {
	// have reports that a first page was read, kept or fresh.
	have bool
	// done reports that the reads ended, at the last page or at an error.
	done                    bool
	err                     error
	offline, limited, stale bool
	truncated               bool
}

// load takes in what a read of a source brought and goes on to the next page.
func (m Model) load(msg loadMsg) (Model, tea.Cmd) {
	f := &m.feeds[msg.src]
	c := msg.chunk
	var cmd tea.Cmd
	switch {
	case msg.err != nil:
		f.err, f.done = msg.err, true
	case msg.cached:
		if c.have {
			m.take(msg.src, c, true)
		}
		// The kept pages only come first: GitHub is asked for all of them.
		cmd = m.readCmd(msg.src, "", true)
	default:
		m.take(msg.src, c, msg.first)
		f.err = nil
		// A cursor that repeats would read the same page for ever.
		if c.next == "" || c.next == msg.cursor {
			f.done = true
		} else {
			cmd = m.readCmd(msg.src, c.next, false)
		}
	}
	m.version++
	m.sync()
	return m, cmd
}

// take replaces the data of src with c, or adds c to it.
func (m *Model) take(src which, c chunk, replace bool) {
	f := &m.feeds[src]
	f.have = true
	if replace {
		f.offline, f.limited, f.stale, f.truncated = false, false, false, false
	}
	f.offline, f.limited, f.stale = f.offline || c.offline, f.limited || c.limited, f.stale || c.stale
	f.truncated = f.truncated || c.truncated
	switch src {
	case timeline:
		if replace {
			m.items = slices.Clone(c.items)
		} else {
			m.items = slices.Concat(m.items, c.items)
		}
	case threads:
		if replace {
			m.threads = slices.Clone(c.threads)
		} else {
			m.threads = slices.Concat(m.threads, c.threads)
		}
		m.total = c.total
	}
}

// reload reads both sources again from their first page, as when GitHub
// answers after what was shown was kept.
func (m Model) reload() (Model, tea.Cmd) {
	m.cancel()
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.seq++
	for i := range m.feeds {
		m.feeds[i].done, m.feeds[i].err = false, nil
	}
	m.commitsErr = nil
	m.version++
	m.sync()
	return m, tea.Batch(m.readCmd(timeline, "", true), m.readCmd(threads, "", true), m.readCommitsCmd())
}
