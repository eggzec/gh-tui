package config

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"go.yaml.in/yaml/v3"

	"github.com/eggzec/gh-tui/internal/keyname"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// Keymap maps the contexts of keys, such as "global" or "pulls", to the
// actions each has, and those to their keys. An action is named by its
// context and its name joined by a dot, such as "pulls.merge", as the
// Action constants are. The contexts are those of [Contexts].
type Keymap map[string]map[string][]string

// ContextGlobal is the context of the keys that work everywhere.
const ContextGlobal = "global"

// Actions of the global context, which work everywhere. No other context
// may bind their keys or redefine them.
const (
	ActionQuit    = "global.quit"
	ActionHelp    = "global.help"
	ActionRefresh = "global.refresh"
	ActionSearch  = "global.search"
	// ActionOpen opens what is selected in the browser.
	ActionOpen = "global.open"
	// ActionNextPane and ActionPrevPane cycle the focus through the panes
	// of the screen, or of a modal with panes.
	ActionNextPane = "global.next_pane"
	ActionPrevPane = "global.prev_pane"
	// ActionPane1 to ActionPane5 focus the pane with that number on the
	// screen: files, pull requests and issues on the repository screen,
	// and pinned, repositories, work, contributions and notifications on
	// the dashboard.
	ActionPane1 = "global.pane_1"
	ActionPane2 = "global.pane_2"
	ActionPane3 = "global.pane_3"
	ActionPane4 = "global.pane_4"
	ActionPane5 = "global.pane_5"
	// ActionNextTab and ActionPrevTab switch between the tabs of the
	// focused pane or modal, such as All, Failing, Running and Mine of the
	// Actions modal, the states of the pull requests and the issues, the
	// owners of the dashboard's repositories, the lists of the page of a
	// user or an organization, and the Filters and Sort tabs of the
	// filter modal.
	ActionNextTab = "global.next_tab"
	ActionPrevTab = "global.prev_tab"
	// ActionBack goes back through the owner pages and the screens shown,
	// as a browser does.
	ActionBack = "global.back"
	// ActionNotifications shows the notifications, from any screen.
	ActionNotifications = "global.notifications"
	// ActionDashboard shows the dashboard, from any screen.
	ActionDashboard = "global.dashboard"
	// ActionFindFile opens the file finder of the repository screen, which
	// finds a file by some letters of its path, and in the modal of a pull
	// request that of the files it changes.
	ActionFindFile = "global.find_file"
	// ActionCommand opens the command line at the bottom of the screen,
	// where commands such as goto are typed.
	ActionCommand = "global.command"
	// ActionSelect opens or chooses what is under the cursor, and
	// ActionDismiss clears what is transient and then closes what is
	// open.
	ActionSelect  = "global.select"
	ActionDismiss = "global.dismiss"
	// ActionOwner shows the page of the person or organization behind what
	// is selected: the author of a pull request or issue, the owner of a
	// repository or of what is in it, such as a file or a notification,
	// or an organization of the dashboard's Repositories pane.
	ActionOwner = "global.owner"
	// ActionRepo shows the repository of what is selected: a result of
	// the search, a row of the dashboard's Repositories, or a
	// notification.
	ActionRepo = "global.repo"
	// ActionZoom shows the focused pane of the dashboard, the repository
	// screen, or a modal such as Actions, alone, or all of them again.
	ActionZoom = "global.zoom"
	// ActionMaximize toggles the open modal between its size and the
	// whole screen.
	ActionMaximize = "global.maximize"
)

// Actions of the repository screen, whose keys the app itself takes. The
// actions of the other contexts are named where their panes and modals
// bind them, by the context of each.
const (
	// ActionHistory opens the History modal of the repository screen.
	ActionHistory = "repo.history"
	// ActionActions opens the Actions modal of the repository screen.
	ActionActions = "repo.actions"
	// ActionStar stars the repository, or unstars it, after asking. It has
	// no key by default, and is the star command.
	ActionStar = "repo.star"
)

// Of returns the keys of action, such as "pulls.merge", or none.
func (k Keymap) Of(action string) []string {
	if f := readWatch.Load(); f != nil {
		(*f)(action)
	}
	ctx, name, _ := strings.Cut(action, ".")
	return k[ctx][name]
}

// readWatch is the function that WatchReads set, if any.
var readWatch atomic.Pointer[func(action string)]

// WatchReads has f called with the name of each action that [Keymap.Of]
// is asked for, found or not, until the returned function is called. It
// is for tests that check which actions the code reads; only one watch
// can be set at a time.
func WatchReads(f func(action string)) (stop func()) {
	readWatch.Store(&f)
	return func() { readWatch.Store(nil) }
}

// Has reports whether the config has the action, bound or not.
func (k Keymap) Has(action string) bool {
	ctx, name, _ := strings.Cut(action, ".")
	_, ok := k[ctx][name]
	return ok
}

// Set binds action, such as "pulls.merge", to keys, adding its context
// if k lacks it.
func (k Keymap) Set(action string, keys []string) {
	ctx, name, _ := strings.Cut(action, ".")
	if k[ctx] == nil {
		k[ctx] = map[string][]string{}
	}
	k[ctx][name] = keys
}

// Actions returns the actions of k, each as its context and name joined
// by a dot, sorted.
func (k Keymap) Actions() []string {
	var out []string
	for ctx, actions := range k {
		for name := range actions {
			out = append(out, ctx+"."+name)
		}
	}
	slices.Sort(out)
	return out
}

// clone returns k with maps and slices of its own.
func (k Keymap) clone() Keymap {
	if k == nil {
		return nil
	}
	out := make(Keymap, len(k))
	for ctx, actions := range k {
		if actions == nil {
			out[ctx] = nil
			continue
		}
		out[ctx] = make(map[string][]string, len(actions))
		for name, keys := range actions {
			out[ctx][name] = slices.Clone(keys)
		}
	}
	return out
}

// UnmarshalYAML reads the contexts of keys. A context that isn't a
// mapping, such as an action set where a context goes, is kept without
// actions, for Validate to refuse with its name.
func (k *Keymap) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: keys: want a mapping of contexts, such as keys.global", n.Line)
	}
	out := make(Keymap, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		name, v := n.Content[i].Value, n.Content[i+1]
		if v.Kind == yaml.ScalarNode && v.ShortTag() == "!!null" {
			// A context with no actions sets nothing.
			out[name] = map[string][]string{}
			continue
		}
		if v.Kind != yaml.MappingNode {
			out[name] = nil
			continue
		}
		actions := map[string][]string{}
		for j := 0; j+1 < len(v.Content); j += 2 {
			action, val := v.Content[j].Value, v.Content[j+1]
			path := "keys." + name + "." + action
			keys, err := decodeKeys(path, val)
			if err != nil {
				return err
			}
			actions[action] = keys
		}
		out[name] = actions
	}
	*k = out
	return nil
}

// decodeKeys reads the keys of the action at path from n, a list of key
// names. An empty value, which is no keys, unbinds the action, as [] does.
func decodeKeys(path string, n *yaml.Node) ([]string, error) {
	if n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	switch {
	case n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null":
		return nil, nil
	case n.Kind != yaml.SequenceNode:
		return nil, fmt.Errorf("line %d: %s: want a list of keys, such as [%s]", n.Line, path, exampleKey(n))
	}
	keys := make([]string, 0, len(n.Content))
	for _, item := range n.Content {
		if item.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf("line %d: %s: want a list of key names", item.Line, path)
		}
		keys = append(keys, item.Value)
	}
	return keys, nil
}

// exampleKey returns the value of n, if it is a plain key, to show as the
// list the setting wants, or a stand-in.
func exampleKey(n *yaml.Node) string {
	if n.Kind == yaml.ScalarNode && n.Value != "" && !strings.ContainsAny(n.Value, ",[]{}\"'") {
		return n.Value
	}
	return "x"
}

// actions are the names of the actions of each context: those that
// default.yaml gives keys.
var actions = sync.OnceValue(func() Keymap { return Default().Keys })

// validate refuses what no press could use, in three layers. Every
// context and action must exist, so that a typo in the config file doesn't
// silently leave the default binding in place, and every key must be a
// name a press has, and not ctrl+c, which always quits. Within a context,
// no key may do two things. And the keys of the contexts that work
// together, global, a screen or modal, and one of its panes, may not
// overlap, since the app would match the outer one first.
func (k Keymap) validate() error {
	var errs []error
	known := actions()
	for _, ctx := range slices.Sorted(maps.Keys(k)) {
		if _, ok := LookupContext(ctx); !ok {
			errs = append(errs, fmt.Errorf("keys.%s: unknown context", ctx))
			continue
		}
		if k[ctx] == nil {
			errs = append(errs, fmt.Errorf("keys.%s: want the actions of the context and their keys, such as keys.%s.%s", ctx, ctx, firstAction(known[ctx])))
			continue
		}
		for _, name := range slices.Sorted(maps.Keys(k[ctx])) {
			errs = append(errs, validateKeys(ctx, name, k[ctx][name]))
		}
	}
	return errors.Join(append(errs, k.clashes()...)...)
}

// firstAction returns the first of the names of actions, sorted, or a
// stand-in for a context that has none.
func firstAction(actions map[string][]string) string {
	names := slices.Sorted(maps.Keys(actions))
	if len(names) == 0 {
		return "action"
	}
	return names[0]
}

// validateKeys refuses an action that ctx doesn't have, or one of keys
// that no press can match, or that is ctrl+c, or that a typing context
// would type, unless its action may be bound to one. No keys, [], unbinds
// the action.
func validateKeys(ctx, name string, keys []string) error {
	path := "keys." + ctx + "." + name
	if _, ok := actions()[ctx][name]; !ok {
		if why, removed := removedActions[ctx+"."+name]; removed {
			return fmt.Errorf("%s: removed, %s", path, why)
		}
		if _, global := actions()[ContextGlobal][name]; global && ctx != ContextGlobal {
			return fmt.Errorf("%s: %s is a global action, which no context may redefine: set keys.global.%s", path, name, name)
		}
		return fmt.Errorf("%s: unknown action", path)
	}
	if slices.Contains(keys, "") {
		return fmt.Errorf("%s: empty key", path)
	}
	for _, key := range keys {
		if err := keyname.Check(key); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if key == forcedQuit {
			return fmt.Errorf("%s: %s always quits and can't be bound", path, key)
		}
		if c, ok := LookupContext(ctx); ok && c.Typing && !slices.Contains(c.Printable, name) && keyhelp.Printable(key) {
			return fmt.Errorf("%s: %s would be typed into the %s; use a key that types nothing, such as one with ctrl or alt", path, key, strings.ToLower(c.Title))
		}
		// An error toast takes the dismiss key before the widgets that type
		// see it, so a printable one could not be typed there.
		if ctx+"."+name == ActionDismiss && keyhelp.Printable(key) {
			return fmt.Errorf("%s: %s would be typed into the command line and the help filter; use a key that types nothing, such as one with ctrl or alt", path, key)
		}
	}
	return nil
}

// removedActions are the actions that no context has any more, each with
// what takes its place, so that a file that still sets one is told what to
// do instead of that it is unknown.
var removedActions = map[string]string{
	"notifications.read_all": "mark all notifications read with the :read all command, which asks first and needs no key",
}

// forcedQuit is the key that always quits, from anywhere, and that no
// context may bind.
const forcedQuit = "ctrl+c"

// clashError is a key that an action of an outer layer, outer, shares with
// the actions inners of the layers inside it. Where the file sets one side
// and not the other, the problem is reported at the setting the file has.
type clashError struct {
	key    string
	outer  string
	kind   string
	inners []string
}

func (e *clashError) Error() string {
	return strings.Join(e.lines(func(string) bool { return false }), "\n")
}

// lines returns the problem as lines, each naming the setting it is at,
// where sets says whether the file sets a setting: one line for each inner
// action the file sets while it doesn't set the outer one, or one for the
// outer action listing the inner ones, if the file sets it or there are
// several, and otherwise one for the only inner one.
func (e *clashError) lines(sets func(path string) bool) []string {
	one := func(inner string) string {
		msg := inner + ": " + e.key + " is already " + e.outer
		if e.kind != "" {
			msg += ", " + e.kind
		}
		return msg
	}
	all := e.outer + ": " + e.key + " is also " + strings.Join(e.inners, ", ") + ": unbind or rebind them there"
	if sets(e.outer) {
		return []string{all}
	}
	var own []string
	for _, in := range e.inners {
		if sets(in) {
			own = append(own, one(in))
		}
	}
	switch {
	case len(own) > 0:
		return own
	case len(e.inners) == 1:
		return []string{one(e.inners[0])}
	}
	return []string{all}
}

// placeClashes returns err with each clashError of it said at the settings
// the file sets, by sets.
func placeClashes(err error, sets func(path string) bool) error {
	if err == nil {
		return nil
	}
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		var out []error
		for _, c := range j.Unwrap() {
			out = append(out, placeClashes(c, sets))
		}
		return errors.Join(out...)
	}
	if e, ok := errors.AsType[*clashError](err); ok {
		return errors.New(strings.Join(e.lines(sets), "\n"))
	}
	return err
}

// clashes returns the problems of keys that overlap: those of one context
// that does two things, and those that the app would match before the
// action it belongs to, between global, a screen or modal, and its panes.
// Unknown contexts and actions are left to validate.
func (k Keymap) clashes() []error {
	known := actions()
	// bound returns the actions of ctx that are known, by key.
	bound := func(ctx string) map[string][]string {
		out := map[string][]string{}
		for _, name := range slices.Sorted(maps.Keys(k[ctx])) {
			if _, ok := known[ctx][name]; !ok {
				continue
			}
			for _, key := range k[ctx][name] {
				if !slices.Contains(out[key], name) {
					out[key] = append(out[key], name)
				}
			}
		}
		return out
	}

	var errs []error
	for _, ctx := range slices.Sorted(maps.Keys(k)) {
		if _, ok := LookupContext(ctx); !ok {
			continue
		}
		// One key on two actions of one context.
		keys := bound(ctx)
		for _, key := range slices.Sorted(maps.Keys(keys)) {
			for _, name := range keys[key][1:] {
				errs = append(errs, fmt.Errorf("keys.%s: %s is both %s and %s", ctx, key, keys[key][0], name))
			}
		}
	}

	// A key of a screen or modal, or of a pane, may not be one of an outer
	// layer: the global context's for either, and the screen's for a pane.
	// The clashes of one outer action are kept together, to list as one
	// when there are several.
	global := bound(ContextGlobal)
	var groups []*clashError
	group := map[string]*clashError{}
	add := func(key, outer, kind, inner string) {
		id := outer + " " + key
		if group[id] == nil {
			group[id] = &clashError{key: key, outer: outer, kind: kind}
			groups = append(groups, group[id])
		}
		group[id].inners = append(group[id].inners, inner)
	}
	for _, c := range contexts {
		if c.Reach != ReachScreen && c.Reach != ReachPane {
			continue
		}
		keys := bound(c.Name)
		var parent Context
		var outer map[string][]string
		if c.Reach == ReachPane {
			parent, _ = LookupContext(c.Parent)
			outer = bound(c.Parent)
		}
		for _, key := range slices.Sorted(maps.Keys(keys)) {
			for _, name := range keys[key] {
				inner := "keys." + c.Name + "." + name
				if g, ok := global[key]; ok {
					add(key, "keys.global."+g[0], "", inner)
				}
				if o, ok := outer[key]; ok {
					add(key, "keys."+c.Parent+"."+o[0], "which works in every pane of the "+parent.Title+" "+parent.kind(), inner)
				}
			}
		}
	}
	for _, g := range groups {
		errs = append(errs, g)
	}
	return errs
}
