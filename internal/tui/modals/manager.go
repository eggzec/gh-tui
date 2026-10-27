package modals

import (
	"iter"
	"reflect"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// Refusal says why an open or a quit did not happen.
type Refusal int

// The refusals.
const (
	NotRefused Refusal = iota
	// Lower: the open modal ranks higher than the new one.
	Lower
	// Stale: the modal the open was made over is no longer on top.
	Stale
	// Same: the new modal is the one on top.
	Same
	// Kept: a guard refused the close.
	Kept
)

// Change is what a request did, for the root to act on.
type Change struct {
	// Opened is set when the top changed by an open; Closed when it
	// changed by closing.
	Opened, Closed bool
	// Refused says why an open or a quit did not happen.
	Refused Refusal
	// Named is the modal the root names in its toast: the top for Lower and
	// Same, or the one whose guard kept it. For Lower it is the top, not
	// necessarily the modal that outranks the new one.
	Named ui.Modal
	// Toast is a guard's toast, if it kept its modal.
	Toast string
	// Cmd holds the guards' commands, then the ReopenedMsg to the new top.
	Cmd tea.Cmd
}

// Room is what the manager sizes the top modal in.
type Room struct {
	Width, Height int
	// Footer is the height of what stays below the frame, such as the
	// command line.
	Footer int
	Narrow bool
}

// Option sets an optional part of an open.
type Option func(*opening)

type opening struct {
	over      ui.Modal
	guard     Guard
	maximized bool
}

// WithOver makes the open conditional on from still being the top: it is
// refused as Stale otherwise. It only checks staleness and never lets an open
// past the rank rule. A nil from checks nothing.
func WithOver(from ui.Modal) Option { return func(o *opening) { o.over = from } }

// WithGuard asks g before the app closes the modal on its own account. It is
// ignored for a modal of a negative priority, which has no guards.
func WithGuard(g Guard) Option { return func(o *opening) { o.guard = g } }

// WithMaximized sets whether the modal starts maximized. The entry keeps its
// toggles.
func WithMaximized(on bool) Option { return func(o *opening) { o.maximized = on } }

type entry struct {
	mod       ui.Modal
	prio      Priority
	guard     Guard
	maximized bool
}

// Manager owns the trail of open modals, the last of which is on top.
type Manager struct {
	trail   []*entry
	maxHeld int

	theme    ui.Theme
	hasTheme bool

	room    Room
	hasRoom bool
	sized   struct {
		w, h   int
		narrow bool
	}
}

// New returns a manager that keeps at most maxHeld modals beneath the top;
// opening past that evicts the oldest. Modals of a negative priority are not
// counted and are never evicted, so a burst of them cannot push real content
// out; the oldest of the others goes.
func New(maxHeld int) *Manager { return &Manager{maxHeld: maxHeld} }

// counted counts the modals that are not of a negative priority.
func (s *Manager) counted() int {
	n := 0
	for _, e := range s.trail {
		if e.prio >= 0 {
			n++
		}
	}
	return n
}

// highest is the entry of the highest non-negative priority, the topmost of
// equals, or nil.
func (s *Manager) highest() *entry {
	var hi *entry
	for _, e := range s.trail {
		if e.prio >= 0 && (hi == nil || e.prio >= hi.prio) {
			hi = e
		}
	}
	return hi
}

// Add opens mod at priority p. A higher priority opens over the top, an equal
// one replaces it (the old one is held for Back only), and a lower one than
// the highest non-negative modal in the trail, even beneath an anywhere
// modal, is refused. A negative priority opens over any top, closes nothing,
// is never replaced and takes no guard: WithGuard is ignored for it. A
// refused open calls nothing on mod.
func (s *Manager) Add(mod ui.Modal, p Priority, opts ...Option) Change {
	var o opening
	for _, opt := range opts {
		opt(&o)
	}
	top := s.top()
	if o.over != nil && (top == nil || top.mod != o.over) {
		return Change{Refused: Stale}
	}
	if top != nil && reflect.TypeOf(top.mod) == reflect.TypeOf(mod) && top.mod.Title() == mod.Title() {
		return Change{Refused: Same, Named: top.mod}
	}
	if p >= 0 {
		// A modal that is not on top can still outrank the new one: a search
		// over a prompt must not let a list open over the prompt.
		if hi := s.highest(); hi != nil && p < hi.prio {
			return Change{Refused: Lower, Named: top.mod}
		}
	}
	if p < 0 {
		o.guard = nil
	}

	if p < 0 {
		// Each negative type is held once: an older copy goes first.
		s.trail = slices.DeleteFunc(s.trail, func(e *entry) bool {
			if e.prio >= 0 || reflect.TypeOf(e.mod) != reflect.TypeOf(mod) {
				return false
			}
			discard(e.mod)
			return true
		})
	}
	if covered := s.top(); covered != nil {
		if h, ok := covered.mod.(ui.Hider); ok {
			h.Hide()
		}
	}
	s.trail = append(s.trail, &entry{mod: mod, prio: p, guard: o.guard, maximized: o.maximized})

	var cmds []tea.Cmd
	for s.counted() > s.maxHeld+1 {
		i := slices.IndexFunc(s.trail, func(e *entry) bool { return e.prio >= 0 })
		oldest := s.trail[i]
		s.trail = slices.Delete(s.trail, i, i+1)
		if oldest.guard != nil {
			cmds = append(cmds, oldest.guard(Dismiss).Cmd)
		}
		discard(oldest.mod)
	}
	s.present()
	return Change{Opened: true, Cmd: tea.Batch(cmds...)}
}

// Close is mod's own close, or esc: it pops the run of equals that ends at
// mod, a negative one alone, and returns to what is under. Guards of the
// others in the run are asked with Dismiss; one that keeps its modal stops
// the run there. It does nothing unless mod is the top.
func (s *Manager) Close(mod ui.Modal) Change {
	top := s.top()
	if top == nil || top.mod != mod {
		return Change{}
	}
	return s.pop(func(i int) bool {
		e := s.trail[i]
		return e == top || (top.prio >= 0 && e.prio == top.prio)
	})
}

// Back pops mod alone, to return to the modal under it. Its guard is asked
// with Dismiss and may keep it. It does nothing unless mod is the top.
func (s *Manager) Back(mod ui.Modal) Change {
	top := s.top()
	if top == nil || top.mod != mod {
		return Change{}
	}
	var ans Closing
	if top.guard != nil {
		ans = top.guard(Dismiss)
		if ans.Keep {
			return Change{Refused: Kept, Named: top.mod, Toast: ans.Toast, Cmd: ans.Cmd}
		}
	}
	ch := s.pop(func(int) bool { return false })
	ch.Cmd = tea.Batch(ans.Cmd, ch.Cmd)
	return ch
}

// pop closes the top, then each entry below it that more reports as part of
// the run, asking its guard.
func (s *Manager) pop(more func(i int) bool) Change {
	ch := Change{Closed: true}
	var cmds []tea.Cmd
	s.drop()
	for len(s.trail) > 0 && more(len(s.trail)-1) {
		e := s.trail[len(s.trail)-1]
		if e.guard != nil {
			ans := e.guard(Dismiss)
			cmds = append(cmds, ans.Cmd)
			if ans.Keep {
				ch.Refused, ch.Named, ch.Toast = Kept, e.mod, ans.Toast
				break
			}
		}
		s.drop()
	}
	if top := s.top(); top != nil {
		s.present()
		cmds = append(cmds, func() tea.Msg { return ui.ReopenedMsg{Modal: top.mod} })
	}
	ch.Cmd = tea.Batch(cmds...)
	return ch
}

// drop removes the top entry and discards it.
func (s *Manager) drop() {
	e := s.trail[len(s.trail)-1]
	s.trail = s.trail[:len(s.trail)-1]
	discard(e.mod)
}

func discard(mod ui.Modal) {
	if d, ok := mod.(ui.Discarder); ok {
		d.Discard()
	}
}

// Quit asks every guard with Quit, top first, and closes nothing. All are
// asked even after one keeps its modal; the first that keeps refuses the quit,
// and the guards' commands are collected in order. With no guards the Cmd is
// nil.
func (s *Manager) Quit() Change {
	var ch Change
	var cmds []tea.Cmd
	for _, e := range slices.Backward(s.trail) {
		if e.guard == nil {
			continue
		}
		ans := e.guard(Quit)
		cmds = append(cmds, ans.Cmd)
		if ans.Keep && ch.Refused == NotRefused {
			ch.Refused, ch.Named, ch.Toast = Kept, e.mod, ans.Toast
		}
	}
	ch.Cmd = tea.Batch(cmds...)
	return ch
}

func (s *Manager) top() *entry {
	if len(s.trail) == 0 {
		return nil
	}
	return s.trail[len(s.trail)-1]
}

// Top returns the open modal, or nil.
func (s *Manager) Top() ui.Modal {
	if e := s.top(); e != nil {
		return e.mod
	}
	return nil
}

// CanBack reports whether a modal is held under the top.
func (s *Manager) CanBack() bool { return len(s.trail) > 1 }

// Held lists the modals, the top first.
func (s *Manager) Held() iter.Seq[ui.Modal] {
	return func(yield func(ui.Modal) bool) {
		for _, e := range slices.Backward(s.trail) {
			if !yield(e.mod) {
				return
			}
		}
	}
}

// Maximized reports whether the top is maximized.
func (s *Manager) Maximized() bool {
	e := s.top()
	return e != nil && e.maximized
}

// ToggleMaximized flips the top's maximize and sizes it again.
func (s *Manager) ToggleMaximized() (resized bool) {
	e := s.top()
	if e == nil {
		return false
	}
	e.maximized = !e.maximized
	return s.size()
}

// Resize sets the room and sizes the top in it.
func (s *Manager) Resize(r Room) (resized bool) {
	s.room, s.hasRoom = r, true
	return s.size()
}

// Refit sizes the top again if what it asks for has changed, such as a form
// that opens a picker and needs the room. Only a ui.Fitter asks.
func (s *Manager) Refit() (resized bool) {
	e := s.top()
	if e == nil {
		return false
	}
	if _, ok := e.mod.(ui.Fitter); !ok {
		return false
	}
	return s.size()
}

// FrameSize is the size of the top with its frame: most of the screen, so
// that the edges of the screen behind it stay in view, or less if the modal
// fits in less. A maximized one fills the screen but for the footer.
func (s *Manager) FrameSize() (w, h int) {
	e := s.top()
	if e == nil {
		return 0, 0
	}
	r := s.room
	if e.maximized {
		return r.Width, max(r.Height-r.Footer, 0)
	}
	w = max(r.Width-2*max(r.Width/10, 2), 0)
	h = max(r.Height-2*max(r.Height/10, r.Footer), 0)
	if f, ok := e.mod.(ui.Fitter); ok {
		fw, fh := f.Fit(max(w-4, 0), max(h-2, 0))
		w, h = min(w, fw+4), min(h, fh+2)
	}
	return w, h
}

// SetTheme gives t to the top now, and to each other modal when it returns.
func (s *Manager) SetTheme(t ui.Theme) {
	s.theme, s.hasTheme = t, true
	if e := s.top(); e != nil {
		e.mod.SetTheme(t)
	}
}

// present gives the new top its theme and size.
func (s *Manager) present() {
	e := s.top()
	if e == nil {
		return
	}
	if s.hasTheme {
		e.mod.SetTheme(s.theme)
	}
	s.sized.w, s.sized.h, s.sized.narrow = -1, -1, !s.room.Narrow
	s.size()
}

// size applies the frame's inside to the top and reports whether it differs
// from what was last applied.
func (s *Manager) size() bool {
	e := s.top()
	if e == nil || !s.hasRoom {
		return false
	}
	fw, fh := s.FrameSize()
	w, h := max(fw-4, 0), max(fh-2, 0)
	if w == s.sized.w && h == s.sized.h && s.room.Narrow == s.sized.narrow {
		return false
	}
	s.sized.w, s.sized.h, s.sized.narrow = w, h, s.room.Narrow
	if n, ok := e.mod.(ui.Narrower); ok {
		n.SetNarrow(s.room.Narrow)
	}
	e.mod.SetSize(w, h)
	return true
}
