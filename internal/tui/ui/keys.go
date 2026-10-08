package ui

import (
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

// Binding makes the binding of action from the configured keys, labelled with
// desc in help. An action without keys, which the config unbinds, gives a
// disabled binding that still says what it does, so the help lists it
// without a key rather than as a blank line.
func Binding(keys config.Keymap, action, desc string) key.Binding {
	return Either(keys, desc, action)
}

// Either makes one binding of the keys of actions, labelled with desc in
// help by all of them, for where actions of different contexts do
// the same, such as the tabs of a pane that its own keys switch too.
func Either(keys config.Keymap, desc string, actions ...string) key.Binding {
	ks := make([]string, 0, len(actions))
	for _, a := range actions {
		ks = append(ks, keys.Of(a)...)
	}
	return bindingOf(desc, ks, actions...)
}

// bindingOf makes the binding of ks, which is disabled and without a key
// if there are none, and known by actions, which it has the keys of.
func bindingOf(desc string, ks []string, actions ...string) key.Binding {
	ks = slices.Compact(slices.Clone(ks))
	if len(ks) == 0 {
		return keymap.Record(keymap.Unbound(key.NewBinding(key.WithHelp("", desc))), actions...)
	}
	return keymap.Record(key.NewBinding(
		key.WithKeys(ks...),
		key.WithHelp(keymap.Labels(ks), desc),
	), actions...)
}

// Context is the keys of one context, such as "pulls", for a pane or modal
// to make its bindings from. An action named with a dot, such as
// "global.select", names another context's, which is how a pane takes the
// keys of an intent it implements without binding them itself.
type Context struct {
	keys config.Keymap
	name string
}

// In returns the keys of context ctx.
func In(keys config.Keymap, ctx string) Context { return Context{keys: keys, name: ctx} }

// Of returns the keys of action: of ctx if it has no dot, else of the
// context it names.
func (c Context) Of(action string) []string {
	path := c.path(action)
	if ks := c.keys.Of(path); ks != nil || !c.keys.Has(path) {
		return ks
	}
	// The action is unbound, which differs from one the config lacks.
	return []string{}
}

// Lookup returns the keys of the actions of context ctx, for the key maps
// of the bubbles to be filled from: a name with a dot, such as
// "global.select", is another context's.
func Lookup(keys config.Keymap, ctx string) keymap.Lookup { return In(keys, ctx) }

// Scope implements keymap.Scoped: the name of the context.
func (c Context) Scope() string { return c.name }

// Binding makes the binding of action, labelled with desc in help, like
// the package's Binding.
func (c Context) Binding(action, desc string) key.Binding {
	return bindingOf(desc, c.Of(action), c.path(action))
}

// path returns the full name of action, as the config has it.
func (c Context) path(action string) string {
	if strings.Contains(action, ".") {
		return action
	}
	return c.name + "." + action
}

// Yield returns b as the help lists it beside held, a binding matched
// before it that shares some of its keys: while held is disabled, which
// the help shows as taking no key, it still takes those keys, to say why
// it can't act, so b is listed without them. The keys are matched as
// they are; only the help uses it.
func Yield(b, held key.Binding) key.Binding {
	if held.Enabled() {
		return b
	}
	keys := slices.DeleteFunc(slices.Clone(b.Keys()), func(k string) bool { return slices.Contains(held.Keys(), k) })
	switch len(keys) {
	case len(b.Keys()):
		return b
	case 0:
		return keymap.Derive(key.NewBinding(key.WithHelp("", b.Help().Desc), key.WithDisabled()), b)
	}
	y := key.NewBinding(key.WithKeys(keys...), key.WithHelp(keymap.Labels(keys), b.Help().Desc))
	y.SetEnabled(b.Enabled())
	return keymap.Derive(y, b)
}

// OpenHint returns the hint that open, the key that opens something on
// GitHub, does so, such as "o to open on GitHub", with the key named as ic
// names it, or "" when it has no key.
func OpenHint(ic Icons, open key.Binding) string {
	if !open.Enabled() || open.Help().Key == "" {
		return ""
	}
	return ic.Key(open.Help().Key) + " to open on GitHub"
}

// Jump returns the binding that stands for the enabled keys of panes in
// help, such as "1-5 focus pane", or a disabled one while none has a key.
// Keys that don't run on, one after the other, as a config that unbinds
// or rebinds some may leave them, are listed each, such as "1/3/4".
func Jump(panes ...key.Binding) key.Binding {
	var keys, labels []string
	on := slices.DeleteFunc(slices.Clone(panes), func(b key.Binding) bool { return !b.Enabled() })
	for _, b := range panes {
		if b.Enabled() {
			keys = append(keys, b.Keys()...)
			labels = append(labels, b.Help().Key)
		}
	}
	if len(labels) == 0 {
		return keymap.Derive(key.NewBinding(key.WithHelp("", "focus pane"), key.WithDisabled()), panes...)
	}
	name := strings.Join(labels, "/")
	if len(labels) > 2 && runOn(labels) {
		name = labels[0] + "-" + labels[len(labels)-1]
	}
	return keymap.Derive(key.NewBinding(key.WithKeys(keys...), key.WithHelp(name, "focus pane")), on...)
}

// runOn reports whether labels are single characters that follow each
// other, such as 1, 2 and 3.
func runOn(labels []string) bool {
	prev := rune(-1)
	for i, l := range labels {
		r, n := utf8.DecodeRuneInString(l)
		if n != len(l) || i > 0 && r != prev+1 {
			return false
		}
		prev = r
	}
	return true
}

// ContextLayer returns the layer of the keys of context ctx: the bindings
// that work there, in the order they are matched, with the hints of
// short. Help titles it as the context is titled.
func ContextLayer(ctx string, bindings, short []key.Binding) keyhelp.Layer {
	return keyhelp.Layer{Source: contextTitle(ctx), Context: ctx, Bindings: bindings, Short: short}
}

// ContextHelp returns the layer of context ctx from the help of the
// bubbles and keys that km lists, and whether they type.
func ContextHelp(ctx string, km help.KeyMap, typing bool) keyhelp.Layer {
	l := keyhelp.FromHelp(contextTitle(ctx), km, typing)
	l.Context = ctx
	return l
}

// MergeLayers returns one layer of context ctx for the keys of several,
// such as those of a pane and of the bubble it shows: a context is one
// layer, whatever code holds its keys. Its bindings are in the order the
// layers are given, which is the order they are matched in, and its hints
// in the reverse, so that the keys of the innermost come first, as they
// would of layers of their own.
func MergeLayers(ctx string, layers ...keyhelp.Layer) keyhelp.Layer {
	out := ContextLayer(ctx, nil, nil)
	for _, l := range layers {
		out.Bindings = append(out.Bindings, l.Bindings...)
		out.Typing = out.Typing || l.Typing
	}
	for _, l := range slices.Backward(layers) {
		out.Short = append(out.Short, l.Short...)
	}
	return out
}

// contextTitle returns the title of context ctx, or its name if the
// config has no such context.
func contextTitle(ctx string) string {
	if c, ok := config.LookupContext(ctx); ok {
		return c.Title
	}
	return ctx
}

// PagerLayer returns the keys of a pager that is the one pane of the
// context ctx, with own, the keys of ctx that the pager's parent matches:
// one layer of ctx, or while the pager takes every key, one of what it
// waits for, a search or an option, which stands alone.
func PagerLayer(ctx string, p *pager.Model, own ...key.Binding) keyhelp.Layer {
	if !p.Capturing() {
		return MergeLayers(ctx, keyhelp.Layer{Bindings: own, Short: own}, keyhelp.FromHelp("", p, false))
	}
	l := keyhelp.FromHelp("pager", p, true)
	switch {
	case p.Prompting():
		l.Context, l.Source = "search_prompt", contextTitle("search_prompt")
	case p.ChoosingOption():
		l.Context, l.Source = "pager_option", contextTitle("pager_option")
	}
	return l
}

// SearchPromptKeys returns the keys of the prompt that a list's find and
// quick filter open: those of the search_prompt context that run it, close
// it, and close it on an empty line.
func SearchPromptKeys(keys config.Keymap) cmdline.KeyMap {
	in := In(keys, "search_prompt")
	return cmdline.NewKeyMap(keymap.Func(func(action string) []string {
		switch action {
		case "run", "cancel", "cancel_empty":
			return in.Of(action)
		}
		return nil
	}))
}

// FeedHelp is a list's feed as the help reads it.
type FeedHelp interface {
	help.KeyMap
	Capturing() bool
}

// FeedLayer returns the keys of a list's feed, to merge into the layer of
// the pane that shows it, and, while the feed takes every key, the layer
// of the prompt that does, which stands alone.
func FeedLayer(f FeedHelp) (l keyhelp.Layer, prompting bool) {
	if !f.Capturing() {
		return keyhelp.FromHelp("", f, false), false
	}
	l = keyhelp.FromHelp("search_prompt", f, true)
	l.Context, l.Source = "search_prompt", contextTitle("search_prompt")
	return l, true
}

// NameLayerActions returns layers with the actions each binding was made
// for, which the help matches the config paths of. It copies what it
// changes, so the layers given stay as they were, and keeps the names a
// layer already has. The help does it when it opens, not each time the
// keys are drawn.
func NameLayerActions(layers []keyhelp.Layer) []keyhelp.Layer {
	out := slices.Clone(layers)
	for i, l := range out {
		if len(l.Actions) > 0 {
			continue
		}
		out[i].Actions = make([][]string, len(l.Bindings))
		for j, b := range l.Bindings {
			out[i].Actions[j] = keymap.Actions(b)
		}
	}
	return out
}
