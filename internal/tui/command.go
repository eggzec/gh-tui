package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// command is one that the command line runs, named by the first word of
// the line. Each command has one name, with no aliases, so that the names
// stay easy to learn and to complete.
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
	run   func(m *Model, arg string) tea.Cmd
}

// commands are those of the command line.
var commands = []command{
	{name: "goto", detail: "open a repository, issue, pull request or link", args: true, run: (*Model).gotoCommand},
	{name: "q", detail: "quit", quits: true, run: func(*Model, string) tea.Cmd { return tea.Quit }},
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
const linePlaceholder = "goto owner/name, #number or a link"

// newLine returns the command line, blurred until the command key opens
// it.
func newLine(keys map[string][]string) cmdline.Model {
	return cmdline.New(
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
		return tea.Batch(save, m.toast.Push(toast.Error, "Unknown command: "+name+"."))
	case !c.args && arg != "":
		return tea.Batch(save, m.toast.Push(toast.Error, "The "+c.name+" command takes no argument."))
	case c.quits:
		return m.quit(save)
	}
	// The command replaces a goto still waiting.
	m.cancelGoto()
	return tea.Batch(save, c.run(m, arg))
}
