package dev

import (
	"context"
	"sync/atomic"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/core"
)

// Reader is what [Previews] reads. The host adapts its service to it.
type Reader interface {
	CachedIssue(t core.Target) (core.IssuePreview, bool)
	Issue(ctx context.Context, t core.Target) (core.IssuePreview, error)
	CachedPull(t core.Target) (core.PullPreview, bool)
	Pull(ctx context.Context, t core.Target) (core.PullPreview, error)
}

var previewIDs atomic.Int64

// previewMsg is what a read of the preview of an item brought.
type previewMsg struct {
	id    int64
	key   string
	pull  bool
	issue core.IssuePreview
	prev  core.PullPreview
	err   error
}

// Option customizes [NewPreviews].
type Option func(*Previews)

// WithContext sets the context that the reads run with, which cancels them.
func WithContext(ctx context.Context) Option {
	return func(p *Previews) { p.ctx = ctx }
}

// drawnKey names the text of a preview at a width.
type drawnKey struct {
	key   string
	width int
}

// Previews turns the hovered row of a tree into the preview of its issue or
// pull request: a cached read first, else a read in a command, drawn once per
// width until what it shows changes. It is used by pointer, since it holds
// maps that the drawn cache and the per-row guards change as it goes.
type Previews struct {
	r   Reader
	ctx context.Context
	id  int64
	// issues and pulls are the previews read, by the key of the row.
	issues map[string]core.IssuePreview
	pulls  map[string]core.PullPreview
	// loading holds the keys with a read out, and failed those whose read
	// failed, which are read again after Retry.
	loading map[string]bool
	failed  map[string]bool
	drawn   map[drawnKey]string
	stamp   int
	reads   int
}

// NewPreviews returns Previews that read through r.
func NewPreviews(r Reader, opts ...Option) *Previews {
	p := &Previews{
		r: r, ctx: context.Background(), id: previewIDs.Add(1),
		issues: map[string]core.IssuePreview{}, pulls: map[string]core.PullPreview{},
		loading: map[string]bool{}, failed: map[string]bool{}, drawn: map[drawnKey]string{},
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// ID is what the messages of the previews carry.
func (p *Previews) ID() int64 { return p.id }

// Version changes when a read lands.
func (p *Previews) Version() int { return p.reads }

// Load returns the read of the preview of the item under key, when it has one
// to show and neither holds nor is reading it. A preview the reader has cached
// needs no read.
func (p *Previews) Load(key string, it Item) tea.Cmd {
	t := it.Ref.Target
	if it.Ref.Problem != "" || p.loading[key] || p.failed[key] {
		return nil
	}
	r, ctx, id := p.r, p.ctx, p.id
	switch it.Kind {
	case Issue, Blocker:
		if _, ok := p.issues[key]; ok {
			return nil
		}
		if v, ok := r.CachedIssue(t); ok {
			p.issues[key] = v
			p.reads++
			return nil
		}
		p.loading[key] = true
		return func() tea.Msg {
			v, err := r.Issue(ctx, t)
			return previewMsg{id: id, key: key, issue: v, err: err}
		}
	case Pull:
		if _, ok := p.pulls[key]; ok {
			return nil
		}
		if v, ok := r.CachedPull(t); ok {
			p.pulls[key] = v
			p.reads++
			return nil
		}
		p.loading[key] = true
		return func() tea.Msg {
			v, err := r.Pull(ctx, t)
			return previewMsg{id: id, key: key, pull: true, prev: v, err: err}
		}
	case Header, More, None:
	}
	return nil
}

// Handles reports whether msg is a read of these previews.
func (p *Previews) Handles(msg tea.Msg) bool {
	m, ok := msg.(previewMsg)
	return ok && m.id == p.id
}

// Update keeps the preview that a read brought, though the cursor has moved on
// since: the preview shown stays where it is.
func (p *Previews) Update(msg tea.Msg) {
	m, ok := msg.(previewMsg)
	if !ok || m.id != p.id {
		return
	}
	delete(p.loading, m.key)
	switch {
	case m.err != nil:
		if p.ctx.Err() != nil {
			return
		}
		p.failed[m.key] = true
	case m.pull:
		p.pulls[m.key] = m.prev
	default:
		p.issues[m.key] = m.issue
	}
	for k := range p.drawn {
		if k.key == m.key {
			delete(p.drawn, k)
		}
	}
	p.reads++
}

// Retry forgets the reads that failed, so that they are made again.
func (p *Previews) Retry() { clear(p.failed) }

// Row is the hovered row to draw the preview of.
type Row struct {
	// Key names the row, and "" when nothing is hovered.
	Key  string
	Item Item
	// Refs feeds the preview of a header: the items under it.
	Refs []core.Reference
	// Loaded says that the tree has its rows, so that an empty one shows
	// nothing rather than loading.
	Loaded bool
	// Stamp changes when the links or rows change, which drops the drawn text.
	Stamp int
}

// View is the preview of the row, drawn once per width until what it shows
// changes.
func (p *Previews) View(r Row, o PreviewOptions) string {
	if r.Stamp != p.stamp {
		clear(p.drawn)
		p.stamp = r.Stamp
	}
	if r.Key == "" {
		if r.Loaded {
			return Nothing(o)
		}
		return Loading(o)
	}
	k := drawnKey{r.Key, o.Width}
	if s, ok := p.drawn[k]; ok {
		return s
	}
	var s string
	switch r.Item.Kind {
	case Header:
		s = HeaderPreview(r.Refs, o)
	case Pull:
		if v, ok := p.pulls[r.Key]; ok {
			s = PullPreview(v, o)
		}
	case Issue, Blocker:
		if v, ok := p.issues[r.Key]; ok {
			s = IssuePreview(v, o)
		}
	case More, None:
	}
	if s == "" {
		if p.failed[r.Key] {
			return Nothing(o)
		}
		return Loading(o)
	}
	p.drawn[k] = s
	return s
}
