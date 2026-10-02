package ui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// Hints is the key map of the status bar, over layers in the order a key
// reaches them. It offers the bindings that some key reaches, as
// keyhelp.Analyze finds them: each layer's short help in the short view,
// and a column of all its bindings a layer in the full view. The layers
// are offered innermost first, the reverse of the order they take keys
// in, so the keys of what has the focus lead and the app's own come last.
// Lead comes before them all in the short view, where a key reaches it,
// such as the way out of a zoom, which a narrow line still shows.
type Hints struct {
	Layers []keyhelp.Layer
	Lead   []key.Binding
}

// ShortHelp implements help.KeyMap. A layer's short help may name its
// bindings for what they do now, so a hint stands for the binding of its
// layer that holds the same keys.
func (h Hints) ShortHelp() []key.Binding {
	reached, seen := h.reached(), make(map[string]bool)
	var out []key.Binding
	for _, b := range h.Lead {
		id := identify(b)
		if b.Enabled() && !seen[id] && slices.ContainsFunc(reached, func(r map[string]bool) bool { return r[id] }) {
			seen[id] = true
			out = append(out, b)
		}
	}
	for li, l := range slices.Backward(h.Layers) {
		for _, b := range l.Short {
			id := identify(b)
			if b.Enabled() && reached[li][keysOf(b)] && !seen[id] {
				seen[id] = true
				out = append(out, b)
			}
		}
	}
	return out
}

// FullHelp implements help.KeyMap.
func (h Hints) FullHelp() [][]key.Binding {
	reached, seen := h.reached(), make(map[string]bool)
	var out [][]key.Binding
	for li, l := range slices.Backward(h.Layers) {
		var col []key.Binding
		for _, b := range l.Bindings {
			id := identify(b)
			if reached[li][id] && !seen[id] {
				seen[id] = true
				col = append(col, b)
			}
		}
		if len(col) > 0 {
			out = append(out, col)
		}
	}
	return out
}

// Off returns l with every binding turned off, for a part that takes no
// keys for now, such as a list with nothing to show.
func Off(l keyhelp.Layer) keyhelp.Layer {
	l.Bindings = slices.Clone(l.Bindings)
	for i := range l.Bindings {
		l.Bindings[i].SetEnabled(false)
	}
	return l
}

// reached returns, for each layer, the bindings that some key reaches, by
// what identifies them and by their keys: those that keep any of their
// keys, and those that lose them only to their twins, such as a key a
// section claims before the app, or one it names that the app handles,
// as the twin does what they say.
func (h Hints) reached() []map[string]bool {
	out := make([]map[string]bool, len(h.Layers))
	for i := range out {
		out[i] = make(map[string]bool)
	}
	for _, r := range keyhelp.Analyze(h.Layers) {
		if r.Status == keyhelp.Disabled || r.Status == keyhelp.Typed {
			continue
		}
		id := identify(r.Binding)
		ok := len(r.Lost) < len(r.Binding.Keys()) || !slices.ContainsFunc(r.Lost, func(l keyhelp.Loss) bool {
			return l.Status != keyhelp.Typed && identify(l.By) != id
		})
		if ok {
			out[r.Layer][id] = true
			out[r.Layer][keysOf(r.Binding)] = true
		}
	}
	return out
}

// identify returns what tells a binding apart: its keys and its help.
func identify(b key.Binding) string {
	h := b.Help()
	return keysOf(b) + "\x00" + h.Key + "\x00" + h.Desc
}

// keysOf returns the keys of b, as one string.
func keysOf(b key.Binding) string { return strings.Join(b.Keys(), " ") }

// NameKeys returns b with its help, the keys it names and what it says
// they do, in the words of ic, as Icons.Key puts them, such as "up/k" for
// "↑/k" in the ASCII set. Help passes through it, or through Icons.Key,
// before it is drawn, since it comes from key maps that know no icon set.
func NameKeys(ic Icons, b key.Binding) key.Binding {
	h := b.Help()
	if k, d := ic.Key(h.Key), ic.Key(h.Desc); k != h.Key || d != h.Desc {
		b.SetHelp(k, d)
	}
	return b
}

// NameLayerKeys returns layers with the keys of their bindings named as
// NameKeys names them, for the hints and the help to show. It copies what
// it changes, so the layers given stay as they were.
func NameLayerKeys(ic Icons, layers []keyhelp.Layer) []keyhelp.Layer {
	out := slices.Clone(layers)
	for i := range out {
		out[i].Bindings = nameAll(ic, out[i].Bindings)
		out[i].Short = nameAll(ic, out[i].Short)
	}
	return out
}

func nameAll(ic Icons, bs []key.Binding) []key.Binding {
	if bs == nil {
		return nil
	}
	out := make([]key.Binding, len(bs))
	for i, b := range bs {
		out[i] = NameKeys(ic, b)
	}
	return out
}
