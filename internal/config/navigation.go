package config

import (
	"slices"
	"strings"
)

// navigation names the actions that have no command, by their name
// without the context: those that move the cursor, a view, the focus or
// the open screen, and those that edit a query or a form, which are keys
// to press where they work. Every other action of a context outside the
// capturing ones is a command of the command line.
var navigation = []string{
	// Moving in a list, a tree or a reading view.
	"up", "down", "left", "right",
	"page_up", "page_down", "half_page_up", "half_page_down",
	"top", "bottom",
	// Acting on what is under the cursor, and stepping out.
	"select", "back", "dismiss", "mark",
	// Moving between panes and tabs.
	"next_pane", "prev_pane", "next_tab", "prev_tab",
	"pane_1", "pane_2", "pane_3", "pane_4", "pane_5",
	// Moving between matches, and the choices of a search page, which has
	// no such actions yet.
	"next_match", "prev_match", "results", "kinds",
	// quit is the q command, and command opens the line that commands
	// are typed in.
	"quit", "command",
	// The keys that edit a filter form and a search query.
	"insert", "append", "toggle", "clear",
}

// Navigation returns the names of the actions that have no command, such
// as "up" and "select": a key moves, and no one types a command for that.
// The result is the caller's to modify.
func Navigation() []string { return slices.Clone(navigation) }

// Commandable reports whether action, such as "pulls.merge", is a
// command of the command line: every action of the contexts that aren't
// capturing, but those of [Navigation].
func Commandable(action string) bool {
	ctx, name, ok := strings.Cut(action, ".")
	if !ok || slices.Contains(navigation, name) {
		return false
	}
	c, ok := LookupContext(ctx)
	return ok && c.Reach != ReachCapture
}

// ActionContexts returns the contexts whose actions include a command
// named name, in the order of [Contexts], such as "pulls" and
// "pull_modal" for "merge".
func ActionContexts(keys Keymap, name string) []Context {
	var out []Context
	for _, c := range contexts {
		if keys.Has(c.Name+"."+name) && Commandable(c.Name+"."+name) {
			out = append(out, c)
		}
	}
	return out
}
