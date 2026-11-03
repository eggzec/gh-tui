package prpanes

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
	"github.com/eggzec/gh-tui/pkg/bubbles/picker"
)

// TickOverlay is a checklist of rows keyed by K, each the Value of a picker
// item. Space ticks the highlighted row, enter hands the ticks to Change, and
// esc or backspace closes without applying.
type TickOverlay[K comparable] struct {
	P       picker.Model
	Heading string
	Rows    int
	// On is which rows are ticked, and Was which were when it opened.
	On, Was map[K]bool
	// Order names a key for the order of the first ticked row.
	Order func(K) string
	// Change turns the ticks into the command that applies them, or nil.
	Change func(on, was map[K]bool) tea.Cmd
	// Locked, when set, answers an attempt to change a locked row.
	Locked func(picker.Item) tea.Cmd
	// Flat is a short list that keeps its order: no rule between the ticked
	// and the others, each move named in the footer, and the keys of the
	// search not offered while typing.
	Flat bool
	done bool
}

// Title is the heading of the frame, with the picker's state.
func (o *TickOverlay[K]) Title() string { return o.P.Title(o.Heading) }

// Kind says the frame is a picker.
func (o *TickOverlay[K]) Kind() panes.OverlayKind { return panes.Picker }

// Items is the rows of the picker and the rule between the ticked and the
// others.
func (o *TickOverlay[K]) Items() int {
	if o.Flat {
		return o.Rows
	}
	return o.Rows + 1
}

// Text is zero: the picker has no text to find in.
func (o *TickOverlay[K]) Text() int { return 0 }

// Clear reports false: the picker clears its own search.
func (o *TickOverlay[K]) Clear() bool { return false }

// Closed reports whether the overlay is done.
func (o *TickOverlay[K]) Closed() bool { return o.done }

// Typing says that the search takes the keys as text.
func (o *TickOverlay[K]) Typing() bool { return o.P.Typing() }

// Layers lists the keys of the picker for the help.
func (o *TickOverlay[K]) Layers() []keyhelp.Layer {
	ctx := "picker_normal"
	if o.P.Typing() && !o.Flat {
		ctx = "picker"
	}
	return []keyhelp.Layer{ui.ContextHelp(ctx, TickHelp{&o.P, o.Flat}, false)}
}

// TickHelp is the picker's help with a footer that says what the keys do
// here: space ticks a row and enter applies the ticks.
type TickHelp struct {
	P    *picker.Model
	Flat bool
}

// ShortHelp lists the keys of the picker as its mode has them.
func (h TickHelp) ShortHelp() []key.Binding {
	if h.P.Typing() && !h.Flat {
		return h.P.ShortHelp()
	}
	k := h.P.KeyMap()
	toggle := k.Normal.Toggle
	toggle.SetHelp(toggle.Help().Key, "toggle")
	apply := k.Choose
	apply.SetHelp(apply.Help().Key, "apply")
	if h.Flat {
		return []key.Binding{k.Normal.Up, k.Normal.Down, k.Normal.Search, toggle, apply, k.Cancel}
	}
	return []key.Binding{moving(k.Normal.Down, k.Normal.Up), k.Normal.Search, toggle, apply, k.Cancel}
}

// FullHelp lists every key of the picker.
func (h TickHelp) FullHelp() [][]key.Binding { return h.P.FullHelp() }

// moving is the one hint for the keys that move, named by the first key of
// each, such as "j/k"; a move that has no key is left out of the name.
func moving(down, up key.Binding) key.Binding {
	var names, keys []string
	for _, b := range []key.Binding{down, up} {
		ks := b.Keys()
		if len(ks) == 0 {
			continue
		}
		names = append(names, keymap.Label(ks[0]))
		keys = append(keys, ks...)
	}
	// The keys are what makes the hint enabled; it is shown, never pressed.
	hint := key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(names, "/"), "move"))
	hint.SetEnabled(len(keys) > 0)
	return keymap.Derive(hint, down, up)
}

// View draws the picker at the size of the frame.
func (o *TickOverlay[K]) View(w, h int) string {
	// The frame keeps a cell of room on each side of what it holds.
	o.P.SetSize(w-2, h)
	return o.P.View()
}

// FirstTicked returns the ticked key that is listed first, and whether there
// is one.
func (o *TickOverlay[K]) FirstTicked() (K, bool) {
	var first K
	found := false
	for k, on := range o.On {
		if on && (!found || strings.ToLower(o.Order(k)) < strings.ToLower(o.Order(first))) {
			first, found = k, true
		}
	}
	return first, found
}

// Marks returns the ticked keys.
func (o *TickOverlay[K]) Marks() []any {
	var marked []any
	for k, on := range o.On {
		if on {
			marked = append(marked, k)
		}
	}
	return marked
}

// Update passes a message to the picker.
func (o *TickOverlay[K]) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case picker.ChosenMsg:
		if msg.ID != o.P.ID() {
			return nil
		}
		o.On = map[K]bool{}
		for _, it := range msg.Ticked {
			if k, ok := it.Value.(K); ok {
				o.On[k] = true
			}
		}
		o.done = true
		return o.Change(o.On, o.Was)
	case picker.LockedMsg:
		if msg.ID != o.P.ID() || o.Locked == nil {
			return nil
		}
		return o.Locked(msg.Item)
	case picker.CancelMsg:
		if msg.ID == o.P.ID() {
			o.done = true
		}
		return nil
	}
	var cmd tea.Cmd
	o.P, cmd = o.P.Update(msg)
	return cmd
}
