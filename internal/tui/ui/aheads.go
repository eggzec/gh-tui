package ui

import (
	"context"
	"log/slog"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
)

// AheadKind is one kind of item that a page reads ahead for its rows, such
// as the detail of a pull request, or its first comments.
type AheadKind[K comparable] struct {
	// Name is the kind in the settings, as in prefetch.<page>.<name>.
	Name string
	// Log names the kind in the log and the summary, such as pull.
	Log string
	// Read reads the item of k into the cache its view reads from.
	Read func(ctx context.Context, k K) error
	// Current reports whether the item of k is cached so that reading it
	// costs no request. It must not do I/O.
	Current func(k K) bool
}

// Aheads reads the kinds of items of a page's rows ahead, each with an
// [Ahead] of its own, and with the knobs that prefetch.<page>.<kind>
// resolves to: a kind may be off while another reads, and each has its
// own window and rest. Create it with [NewAheads]; a nil *Aheads reads
// nothing. It reads nothing until [Aheads.Configure] gives it the
// settings.
type Aheads[K comparable] struct {
	page   string
	kinds  []AheadKind[K]
	aheads []*Ahead[K]
}

// NewAheads returns the reads ahead of kinds on page, such as pulls, whose
// reads ctx bounds until the first [Aheads.Reset].
func NewAheads[K comparable](ctx context.Context, page string, kinds ...AheadKind[K]) *Aheads[K] {
	a := &Aheads[K]{page: page, kinds: kinds, aheads: make([]*Ahead[K], len(kinds))}
	for i, k := range kinds {
		a.aheads[i] = NewAhead(k.Log, k.Read, k.Current, 0, 0)
		a.aheads[i].Configure(config.Resolved{})
		a.aheads[i].Reset(ctx)
	}
	return a
}

// Share bounds the reads of every kind with those of the other readers
// ahead that share s, as [Ahead.Share] does.
func (a *Aheads[K]) Share(s *Slots) {
	if a == nil {
		return
	}
	for _, ah := range a.aheads {
		ah.Share(s)
	}
}

// Configure applies the settings p resolves for each kind, as
// [Ahead.Configure] does with them.
func (a *Aheads[K]) Configure(p config.PrefetchLayers) {
	if a == nil {
		return
	}
	for i, k := range a.kinds {
		a.aheads[i].Configure(Resolve(p, a.page, k.Name))
	}
}

// On reports whether any kind reads ahead.
func (a *Aheads[K]) On() bool {
	return a != nil && slices.ContainsFunc(a.aheads, (*Ahead[K]).On)
}

// Window tells each kind that the cursor is on row i of the list, as
// [Ahead.Window] does. Call it whenever the cursor may have moved or the
// list changed.
func (a *Aheads[K]) Window(at func(i int) (K, bool), i int) tea.Cmd {
	if a == nil {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(a.aheads))
	for _, ah := range a.aheads {
		cmds = append(cmds, ah.Window(at, i))
	}
	return tea.Batch(cmds...)
}

// Rested reads the window of the kind whose cursor rested, unless it moved
// since.
func (a *Aheads[K]) Rested(msg AheadMsg) tea.Cmd {
	if a == nil {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(a.aheads))
	for _, ah := range a.aheads {
		cmds = append(cmds, ah.Rested(msg))
	}
	return tea.Batch(cmds...)
}

// Reset cancels the reads of the list shown, for a new list whose reads
// parent bounds.
func (a *Aheads[K]) Reset(parent context.Context) {
	if a == nil {
		return
	}
	for _, ah := range a.aheads {
		ah.Reset(parent)
	}
}

// Resume reads ahead again after GitHub reported the rate limit, such as
// for another repository.
func (a *Aheads[K]) Resume() {
	if a == nil {
		return
	}
	for _, ah := range a.aheads {
		ah.Resume()
	}
}

// Opened records that the items of k were opened, so that the summary
// counts each kind read ahead as used.
func (a *Aheads[K]) Opened(k K) {
	if a == nil {
		return
	}
	for _, ah := range a.aheads {
		ah.Opened(k)
	}
}

// Pause holds the reads of every kind that haven't started, as
// [Ahead.Pause] does, until the returned resume is called.
func (a *Aheads[K]) Pause() (resume func()) {
	if a == nil {
		return func() {}
	}
	ps := make([]Pauser, 0, len(a.aheads))
	for _, ah := range a.aheads {
		ps = append(ps, ah)
	}
	return PauseAll(ps...)
}

// Resolve returns the knobs that p resolves for kind on page, as
// [config.PrefetchLayers.Resolve] does. A page or kind the settings don't
// have is a mistake in the code, not in the config: it is logged, and
// reads nothing ahead.
func Resolve(p config.PrefetchLayers, page, kind string) config.Resolved {
	r, err := p.Resolve(page, kind)
	if err != nil {
		slog.Error("prefetch settings", "span", "tui", "page", page, "kind", kind, "err", err.Error())
		return config.Resolved{}
	}
	return r
}
