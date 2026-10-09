package config

import "slices"

// Reach is how far a context reaches. The keys of the app go through up
// to three layers, outermost first: those of the whole app, those of the
// screen or modal on view, and those of the pane that has the focus. A
// widget that takes every key while it is open stands outside them.
type Reach int

const (
	// ReachGlobal is the one context whose keys work everywhere, unless a
	// capturing context is open.
	ReachGlobal Reach = iota + 1
	// ReachScreen holds the keys that work in every pane of a screen or
	// modal. A screen or modal with a single pane has this one context
	// only, with all of its keys.
	ReachScreen
	// ReachPane holds the keys of one pane of a screen or modal, which has
	// that screen or modal as its parent.
	ReachPane
	// ReachCapture is a widget that takes every key but ctrl+c while it
	// is open, such as the command line or a prompt.
	ReachCapture
)

// Context is a place that keys are set for: keys.<name>.<action>.
type Context struct {
	// Name is the name in the config, such as "pulls".
	Name string
	// Title names the context in help, such as "Pull requests".
	Title string
	Reach Reach
	// Parent is the screen or modal that a pane belongs to, whose keys
	// work in the pane too. A capturing context has the place it opens in,
	// which matters to nothing but readers.
	Parent string
	// Modal is whether a screen-layer context is a modal over the screens,
	// rather than a screen.
	Modal bool
	// Within is the modal that this one is a step of, if it never opens
	// first: it shows inside that modal, in place of what it showed.
	Within string
	// Step is whether a screen-layer context is a step shown inside any of
	// several modals, which never opens first, such as the links of a pull
	// request or an issue.
	Step bool
	// Typing is whether a capturing context types the printable keys that
	// none of its actions bind, such as the command line. Binding a
	// printable key there would make the key untypable, so validation
	// refuses it, except for the actions Printable names.
	Typing bool
	// Printable names the actions of a typing context that may be bound
	// to printable keys: those that act only where the key would not be
	// typed.
	Printable []string
}

// contexts are all the contexts of keys there are.
var contexts = []Context{
	{Name: ContextGlobal, Title: "global", Reach: ReachGlobal},

	{Name: "dashboard", Title: "Dashboard", Reach: ReachScreen},
	{Name: "dashboard_pinned", Title: "Pinned", Reach: ReachPane, Parent: "dashboard"},
	{Name: "dashboard_repos", Title: "Repositories", Reach: ReachPane, Parent: "dashboard"},
	{Name: "dashboard_work", Title: "Work", Reach: ReachPane, Parent: "dashboard"},
	{Name: "dashboard_calendar", Title: "Contributions", Reach: ReachPane, Parent: "dashboard"},
	{Name: "dashboard_inbox", Title: "Notifications", Reach: ReachPane, Parent: "dashboard"},

	{Name: "repo", Title: "Repository", Reach: ReachScreen},
	{Name: "files", Title: "Files", Reach: ReachPane, Parent: "repo"},
	{Name: "pulls", Title: "Pull requests", Reach: ReachPane, Parent: "repo"},
	{Name: "issues", Title: "Issues", Reach: ReachPane, Parent: "repo"},

	{Name: "notifications", Title: "Notifications", Reach: ReachScreen},

	{Name: "search", Title: "Search", Reach: ReachScreen},
	{Name: "search_query", Title: "Query", Reach: ReachCapture, Parent: "search", Typing: true},
	{Name: "search_results", Title: "Results", Reach: ReachPane, Parent: "search"},

	{Name: "owner", Title: "Profile", Reach: ReachScreen},
	{Name: "owner_pinned", Title: "Pinned", Reach: ReachPane, Parent: "owner"},
	{Name: "owner_list", Title: "List", Reach: ReachPane, Parent: "owner"},
	{Name: "owner_readme", Title: "README", Reach: ReachPane, Parent: "owner"},
	{Name: "owner_calendar", Title: "Contributions", Reach: ReachPane, Parent: "owner"},

	{Name: "pull_modal", Title: "Pull request", Reach: ReachScreen, Modal: true},
	{Name: "pull_conversation", Title: "Conversation", Reach: ReachPane, Parent: "pull_modal"},
	// The panes of the Files tab: the tree of the files changed, and their diff.
	{Name: "pull_files", Title: "Changed files", Reach: ReachPane, Parent: "pull_modal"},
	{Name: "pull_diff", Title: "Diff", Reach: ReachPane, Parent: "pull_modal"},
	// The panes of the Checks step of the pull request.
	{Name: "pull_check_list", Title: "Checks", Reach: ReachPane, Parent: "pull_modal"},
	{Name: "pull_check_log", Title: "Log", Reach: ReachPane, Parent: "pull_modal"},
	{Name: "pull_check_annotations", Title: "Annotations", Reach: ReachPane, Parent: "pull_modal"},
	{Name: "pull_check_detail", Title: "Detail", Reach: ReachPane, Parent: "pull_modal"},
	{Name: "issue_modal", Title: "Issue", Reach: ReachScreen, Modal: true},
	// The links of a pull request or an issue show in place of what its
	// modal showed, so its keys are a layer of their own and the modal's
	// don't work.
	{Name: "references", Title: "Linked items", Reach: ReachScreen, Modal: true, Step: true},
	{Name: "release_modal", Title: "Release", Reach: ReachScreen, Modal: true},

	{Name: "history", Title: "History", Reach: ReachScreen, Modal: true},
	{Name: "history_branches", Title: "Branches", Reach: ReachPane, Parent: "history"},
	{Name: "history_graph", Title: "Graph", Reach: ReachPane, Parent: "history"},
	{Name: "history_files", Title: "Files", Reach: ReachPane, Parent: "history"},
	{Name: "history_patch", Title: "Patch", Reach: ReachPane, Parent: "history"},

	{Name: "actions", Title: "Actions", Reach: ReachScreen, Modal: true},
	{Name: "actions_runs", Title: "Runs", Reach: ReachPane, Parent: "actions"},
	{Name: "actions_jobs", Title: "Jobs", Reach: ReachPane, Parent: "actions"},
	{Name: "actions_log", Title: "Log", Reach: ReachPane, Parent: "actions"},
	{Name: "actions_annotations", Title: "Annotations", Reach: ReachPane, Parent: "actions"},

	{Name: "preview", Title: "File", Reach: ReachScreen, Modal: true},
	{Name: "text", Title: "Text", Reach: ReachScreen, Modal: true},
	{Name: "filter", Title: "Filter", Reach: ReachScreen, Modal: true},
	// The filter of the runs replaces the Actions modal while it is open.
	{Name: "actions_filter", Title: "Filter", Reach: ReachScreen, Modal: true, Within: "actions"},

	{Name: "command_line", Title: "Command line", Reach: ReachCapture, Typing: true},
	// ? closes the help while its query is empty, and is typed otherwise.
	{Name: "help", Title: "Help", Reach: ReachCapture, Typing: true, Printable: []string{"close"}},
	{Name: "confirm", Title: "Confirm", Reach: ReachCapture},
	{Name: "prompt", Title: "Prompt", Reach: ReachCapture, Typing: true},
	{Name: "finder", Title: "Finder", Reach: ReachCapture, Parent: "repo", Typing: true},
	// Space checks the item under the cursor of a dropdown with several
	// choices while its filter doesn't have the keys, and is typed in the
	// filter.
	{Name: "picker", Title: "Picker", Reach: ReachCapture, Typing: true, Printable: []string{"toggle"}},
	// A picker in normal mode, where letters move and open its filter
	// instead of being typed.
	{Name: "picker_normal", Title: "Picker (normal mode)", Reach: ReachCapture},
	{Name: "search_prompt", Title: "Search", Reach: ReachCapture, Typing: true},
	{Name: "pager_option", Title: "Option", Reach: ReachCapture},
	// What names an option after a log's option key: it takes one key and
	// types nothing.
	{Name: "log_option", Title: "Option", Reach: ReachCapture},
	// The query line of the filter form, and the editor of a text field.
	{Name: "filter_query", Title: "Filter query", Reach: ReachCapture, Typing: true},
}

// kind says what the context is, a screen or a modal, for messages.
func (c Context) kind() string {
	if c.Modal {
		return "modal"
	}
	return "screen"
}

// Contexts returns every context of keys, for help, tests and what lists
// them. The result is the caller's to modify.
func Contexts() []Context { return slices.Clone(contexts) }

// LookupContext returns the context called name.
func LookupContext(name string) (Context, bool) {
	i := slices.IndexFunc(contexts, func(c Context) bool { return c.Name == name })
	if i < 0 {
		return Context{}, false
	}
	return Contexts()[i], true
}

// Chain returns the names of the contexts whose keys work while name has
// the focus, outermost first: the global one, the screen or modal of a
// pane, and name itself. A capturing context stands alone. It returns
// nothing for a name that is no context.
func Chain(name string) []string {
	c, ok := LookupContext(name)
	switch {
	case !ok:
		return nil
	case c.Reach == ReachCapture:
		return []string{name}
	case c.Reach == ReachGlobal:
		return []string{ContextGlobal}
	case c.Reach == ReachScreen:
		return []string{ContextGlobal, name}
	}
	return []string{ContextGlobal, c.Parent, name}
}
