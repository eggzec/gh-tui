package tui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// command is one that the command line runs, named by the first word of
// the line. Each command has one name, with no aliases, so that the names
// stay easy to learn and to complete. A command that does what a key does
// runs the key's own code, so that it acts just as the key would.
type command struct {
	name string
	// detail says what the command does, beside its name in the
	// candidates.
	detail string
	// args reports whether the command takes an argument.
	args bool
	// quits reports whether the command ends the program, which then
	// waits for the history to be saved.
	quits bool
	// over says where the command runs while a modal is open. A command
	// that acts on the app alone, or on nothing the modal hides, runs over
	// every modal; one that acts on what a modal shows runs over those
	// that name it. Every other command would change what is behind the
	// modal, or replace it and lose the place in it.
	over over
	run  func(m *Model, arg string) tea.Cmd
	// complete, if set, completes the argument: arg is the line from
	// after the name to the cursor, which is at cursor, and end is the
	// end of the word under it, which atEnd reports is the end of the
	// line. It reads memory only.
	complete func(m *Model, arg string, cursor, end int, atEnd bool) []cmdline.Candidate
}

// over says over which modals a command runs.
type over int

const (
	// A command with no over, the zero, is one that a modal refuses: it
	// acts on the screen behind it.
	_ over = iota
	// overAny is a command that runs over every modal.
	overAny
	// overNamed is a command that runs over the modals that name it in
	// [ui.Commanded.Commands].
	overNamed
)

// commands are those of the command line, in the order they complete.
var commands = []command{
	{name: ui.AuthCommand, detail: "show what the token may do, and grant it more", run: (*Model).authCommand},
	{name: "config", detail: "show the config, or with defaults, default.yaml", args: true, over: overAny, run: (*Model).configCommand, complete: completeConfig},
	{name: ui.CommandCopy, detail: "copy the url, ref, sha or path of what is selected", args: true, over: overNamed, run: (*Model).copyCommand, complete: completeCopy},
	{name: "filter", detail: "filter the focused list", run: filtering(filterform.FiltersTab)},
	{name: "goto", detail: "open a repository, issue, pull request, profile or link", args: true, run: (*Model).gotoCommand, complete: (*Model).completeTarget},
	{name: "help", detail: "list the keys", over: overAny, run: pressing(config.ActionHelp)},
	{name: "images", detail: "show whether images are drawn here, and why", over: overAny, run: (*Model).imagesCommand},
	{name: "open", detail: "open on GitHub what follows, or what is selected", args: true, over: overAny, run: (*Model).openCommand, complete: (*Model).completeTarget},
	{name: "q", detail: "quit", quits: true, over: overAny, run: func(*Model, string) tea.Cmd { return tea.Quit }},
	{name: ui.CommandRaw, detail: "show the open file as its source with on, or rendered with off", args: true, over: overNamed, run: (*Model).rawCommand, complete: completeRaw},
	{name: "refresh", detail: "read the focused view again", run: pressing(config.ActionRefresh)},
	{name: "search", detail: "search GitHub, for what follows if anything", args: true, run: (*Model).searchCommand},
	{name: "set", detail: "change a setting for this session, or show it", args: true, over: overAny, run: (*Model).setCommand, complete: (*Model).completeSet},
	{name: "sort", detail: "sort the focused list", run: filtering(filterform.SortTab)},
}

// pressing returns the run of a command that presses the key of action.
func pressing(action string) func(m *Model, arg string) tea.Cmd {
	return func(m *Model, _ string) tea.Cmd { return m.press(action) }
}

// filtering returns the run of a command that opens the filter modal of
// the focused view on tab, as the filter and sort keys do. Where the key
// would go on to a view that has no such tab, the command says so.
func filtering(tab filterform.Tab) func(m *Model, arg string) tea.Cmd {
	return func(m *Model, _ string) tea.Cmd {
		if p := m.focused(); p != nil && m.openFilter(p.section, tab) {
			return nil
		}
		if tab == filterform.SortTab {
			return m.toast.Push(toast.Warning, "Nothing here to sort.")
		}
		return m.toast.Push(toast.Warning, "Nothing here to filter.")
	}
}

// Searcher is a search page that searches for a query, as if the user
// typed it and pressed enter.
type Searcher interface {
	Search(query string) tea.Cmd
}

// Fresher is a search page that can drop its query, and the results of
// it, so the next one starts empty.
type Fresher interface {
	Fresh()
}

// searchCommand shows the search page, as the search key does, and
// searches for query there, unless it is empty.
func (m *Model) searchCommand(query string) tea.Cmd {
	if m.srch == nil {
		return m.toast.Push(toast.Warning, "There is no search page.")
	}
	// It goes somewhere, so a goto still waiting mustn't take the user
	// elsewhere after it, even when the search page is already on view.
	m.cancelGoto()
	show := m.showSearch()
	s, ok := m.srch.section.(Searcher)
	if query == "" || !ok {
		return show
	}
	return tea.Batch(show, s.Search(query))
}

// commandsOver reports whether the command key opens the command line over
// the open modal now: whenever the modal doesn't take the key as text, as
// a query or an answer does.
func (m *Model) commandsOver() bool {
	return !m.modalTakesKeys()
}

// runsOver reports whether c runs now: with no modal open, or over the
// open one, which either takes every such command or names c.
func (m *Model) runsOver(c command) bool {
	mod := m.topModal()
	if mod == nil {
		return true
	}
	return c.over == overAny || c.over == overNamed && slices.Contains(mod.Commands(), c.name)
}

// findCommand returns the command named name.
func findCommand(name string) (command, bool) {
	for _, c := range commands {
		if c.name == name {
			return c, true
		}
	}
	return command{}, false
}

// linePlaceholder is shown while the line is empty.
const linePlaceholder = "goto owner/name, @login, #number or a link"

// newLine returns the command line, blurred until the command key opens
// it, which recalls the last history lines typed.
func newLine(keys config.Keymap, history int) cmdline.Model {
	return cmdline.New(history,
		cmdline.WithKeyMap(lineKeys(keys)),
		cmdline.WithPlaceholder(linePlaceholder),
	)
}

// openLine opens the command line in place of the status bar.
func (m *Model) openLine() tea.Cmd {
	cmd := m.line.Open("")
	m.layout()
	return cmd
}

// updateLine passes msg to the open command line, and lays the screen out
// again when the line takes another height or closes.
func (m *Model) updateLine(msg tea.Msg) tea.Cmd {
	h := m.footerHeight()
	var cmd tea.Cmd
	m.line, cmd = m.line.Update(msg)
	if m.footerHeight() != h {
		m.layout()
	}
	return cmd
}

// lineDone lays the screen out again once the command line has closed.
func (m *Model) lineDone() {
	m.layout()
}

// runLine runs the command that line names, and reports an unknown one,
// along with save, which saves the history. A command that quits waits
// for it.
func (m *Model) runLine(line string, save tea.Cmd) tea.Cmd {
	name, arg, _ := strings.Cut(line, " ")
	arg = strings.TrimSpace(arg)
	c, ok := findCommand(name)
	switch {
	case !ok:
		return tea.Batch(save, m.toast.Push(toast.Warning, "Unknown command: "+name+"."))
	case !m.runsOver(c):
		return tea.Batch(save, m.toast.Push(toast.Warning, "Close "+m.modalName(m.topModal())+" first to use "+c.name+"."))
	case !c.args && arg != "":
		return tea.Batch(save, m.toast.Push(toast.Warning, "The "+c.name+" command takes no argument."))
	case c.quits:
		return m.quit(save)
	}
	return tea.Batch(save, c.run(m, arg))
}
