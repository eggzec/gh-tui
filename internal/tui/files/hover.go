package files

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
	"github.com/eggzec/gh-tui/internal/obs"
	filesvc "github.com/eggzec/gh-tui/internal/service/files"
)

// hover reads the file under the cursor once the cursor rests on it, so its
// preview opens at once. Moving on before the delay reads nothing, and a
// newer read cancels an older one still in flight, so at most one runs.
type hover struct {
	delay time.Duration
	// max is the largest file read, or 0 to read none.
	max int64
	// id is the node the cursor was last seen on, and seq counts the
	// times it moved, so that only the latest delay fires.
	id     string
	seq    int
	entry  core.TreeEntry
	cancel context.CancelFunc
}

// hoverMsg reports that the cursor rested on a node for the delay.
type hoverMsg struct {
	tree int
	seq  int
}

// reset forgets the cursor and cancels the read in flight, for another
// tree.
func (h *hover) reset() {
	h.stop()
	h.id, h.entry = "", core.TreeEntry{}
}

func (h *hover) stop() {
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

// moved starts the delay when the cursor moved to a file that isn't cached.
func (s *Section) moved() tea.Cmd {
	h := &s.hover
	if h.max <= 0 {
		return nil
	}
	n, _ := s.tree.Selected()
	if n.ID == h.id {
		return nil
	}
	h.id = n.ID
	h.seq++
	e, ok := entryOf(n)
	if !ok || !worthReading(e, h.max) {
		return nil
	}
	if _, ok := s.svc.CachedBlob(s.blobQuery(e)); ok {
		s.seen.Count(obs.PrefetchCached)
		return nil
	}
	h.entry = e
	msg := hoverMsg{tree: s.tree.ID(), seq: h.seq}
	return tea.Tick(h.delay, func(time.Time) tea.Msg { return msg })
}

// rested reads the file the cursor rested on, unless it moved since.
func (s *Section) rested(msg hoverMsg) tea.Cmd {
	h := &s.hover
	if msg.tree != s.tree.ID() || msg.seq != h.seq {
		return nil
	}
	h.stop()
	ctx, cancel := context.WithCancel(obs.WithTrace(obs.ForPrefetch(s.treeCtx), "prefetch.hover"))
	h.cancel = cancel
	svc, q, seen := s.svc, s.blobQuery(h.entry), s.seen
	return func() tea.Msg {
		defer cancel()
		_, _ = readBlob(ctx, svc, seen, q)
		return nil
	}
}

func (s *Section) blobQuery(e core.TreeEntry) filesvc.BlobQuery {
	return filesvc.BlobQuery{Repo: s.repo, SHA: e.SHA, Size: e.Size}
}
