package search

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// PreviewReader returns what reads more of the preview of hit than the hit
// holds, such as a README, or nil when the hit's own data is all its preview
// needs, which then shows at once.
type PreviewReader func(h core.SearchHit) func(ctx context.Context) (string, error)

// WithPreviewReader gives the preview a way to read more of a hit, lazily:
// the hit under the cursor is read when it is first shown, and the rows
// around it as prefetch.search.preview says, each once per query.
func WithPreviewReader(r PreviewReader) Option {
	return func(s *Section) { s.previewer = r }
}

// extra is what was read of a hit's preview.
type extra struct {
	text string
	err  error
}

// previewReadMsg tells that the read of a hit's preview ended.
type previewReadMsg struct {
	id   int64
	text string
	key  string
	body string
	err  error
}

// forgetExtras drops what was read for the query before; the reads still in
// flight are canceled with the query's context.
func (s *Section) forgetExtras() {
	s.extras = nil
	s.loading = nil
}

// extraText returns what the preview of h adds to its own text, and the
// suffix of its key that changes as the reading goes, so that the pager is
// filled again when it lands. A hit that needs no read adds nothing.
func (s *Section) extraText(h core.SearchHit) (suffix, add string) {
	if s.previewer == nil || s.previewer(h) == nil {
		return "", ""
	}
	e, ok := s.extras[hitKey(h)]
	switch {
	case !ok:
		return " loading", "\nLoading more" + s.icons.Ellipsis + "\n"
	case e.err != nil:
		return " failed", "\nCould not load more: " + e.err.Error() + "\n"
	default:
		return " loaded", "\n" + e.text
	}
}

// readPreview starts the reads of the previews that need one: the hit under
// the cursor first, then those within prefetch.search.preview rows of it.
func (s *Section) readPreview() tea.Cmd {
	l, shown := s.visibleHits()
	if !shown || s.previewer == nil || s.text == "" || s.textCtx == nil {
		return nil
	}
	at := l.feed.Index()
	var cmds []tea.Cmd
	for d := 0; d <= s.previewRows; d++ {
		for _, i := range [2]int{at + d, at - d} {
			if d == 0 && i != at || d > 0 && i == at {
				continue
			}
			if h, ok := l.feed.Item(i); ok {
				cmds = append(cmds, s.readExtra(h))
			}
		}
	}
	return tea.Batch(cmds...)
}

// readExtra returns the read of h's preview, unless it needs none or was
// read or is being read.
func (s *Section) readExtra(h core.SearchHit) tea.Cmd {
	read := s.previewer(h)
	key := hitKey(h)
	if _, done := s.extras[key]; read == nil || done || s.loading[key] {
		return nil
	}
	if s.loading == nil {
		s.loading = map[string]bool{}
	}
	s.loading[key] = true
	ctx, id, text := s.textCtx, s.id, s.text
	return func() tea.Msg {
		body, err := read(ctx)
		return previewReadMsg{id: id, text: text, key: key, body: body, err: err}
	}
}

// readPreviewDone keeps what a read found, unless the query changed or the
// read was canceled.
func (s *Section) readPreviewDone(msg previewReadMsg) {
	if msg.id != s.id || msg.text != s.text {
		return
	}
	delete(s.loading, msg.key)
	if errors.Is(msg.err, context.Canceled) {
		return
	}
	if s.extras == nil {
		s.extras = map[string]extra{}
	}
	s.extras[msg.key] = extra{text: msg.body, err: msg.err}
}
