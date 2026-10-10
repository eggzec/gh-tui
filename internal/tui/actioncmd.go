package tui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/config"
	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/cmdline"
	"github.com/eggzec/gh-tui/pkg/bubbles/filterform"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/toast"
)

// Every action of the config that [config.Commandable] says so is a
// command of the command line, named as the action is without its context:
// merge is the pulls.merge action and the pull_modal.merge one, whichever
// has the keys now. A built-in command of the table has its name first,
// so that an action never shadows one.

// focusChain returns the contexts of keys that reach something now, the
// innermost first and the global one last: the pane that has the focus,
// the screen or modal around it, and the keys that work everywhere. A
// widget that takes every key is no part of it.
func (m *Model) focusChain() []string {
	inner, _ := m.innerLayers()
	chain := []string{config.ContextGlobal}
	for _, l := range inner {
		c, ok := config.LookupContext(l.Context)
		if !ok || c.Reach == config.ReachCapture || slices.Contains(chain, l.Context) {
			continue
		}
		chain = append(chain, l.Context)
	}
	slices.Reverse(chain)
	return chain
}

// resolveAction returns the action that the command name stands for where
// the focus is: the one of the innermost context of focusChain that has it.
func (m *Model) resolveAction(name string) (action string, ok bool) {
	for _, ctx := range m.focusChain() {
		action = ctx + "." + name
		if m.cfg.Keys.Has(action) && config.Commandable(action) {
			return action, true
		}
	}
	return "", false
}

// pressAction does action as its key would, wherever the key goes, so that
// it passes the same gates and asks the same questions, whether the config
// binds the action to a key or not. A modal takes the intents it has, such
// as the owner of what it shows, in the order it does for the key.
func (m *Model) pressAction(action string) tea.Cmd {
	return m.key(keymap.ActionPress(action))
}

// actionRead is the action that marks the selected notifications read,
// whose command also takes "all".
const actionRead = "notifications.read"

// actionCommand runs the command name that is no built-in one: the action
// of that name that the focus has, or else a refusal that says where it
// works, or that there is no such command. read with the argument all is
// the one that marks every notification read, for which no action has a
// key.
func (m *Model) actionCommand(name, arg string) tea.Cmd {
	what := name
	readAll := name == "read" && arg == "all"
	if readAll {
		what = "read all"
	}
	// A widget that takes every key has no commands but those of the
	// line, and a modal like it is closed before the app acts.
	if mod := m.topModal(); mod != nil && m.modalTakesKeys() {
		return m.toast.Push(toast.Warning, "Close "+m.modalName(mod)+" first to use "+what+".")
	}
	if p := m.focused(); m.topModal() == nil && p != nil {
		if c, ok := p.section.(ui.Capturer); ok && c.Capturing() {
			return m.toast.Push(toast.Warning, "Finish typing first to use "+what+".")
		}
	}
	action, ok := m.resolveAction(name)
	if !ok {
		where := m.whereActions(name)
		switch mod := m.topModal(); {
		case where == "":
			return m.toast.Push(toast.Warning, "Unknown command: "+name+".")
		case mod != nil:
			// A modal refuses what it doesn't own, naming itself.
			return m.toast.Push(toast.Warning, "Close "+m.modalName(mod)+" first to use "+what+".")
		}
		return m.toast.Push(toast.Warning, what+" works in "+where+".")
	}
	if arg != "" && !readAll {
		if action == actionRead {
			return m.toast.Push(toast.Warning, "The read command takes no argument but all.")
		}
		return m.toast.Push(toast.Warning, "The "+name+" command takes no argument.")
	}
	if mod := m.topModal(); mod != nil && action == config.ActionFindFile {
		return m.toast.Push(toast.Warning, "Close "+m.modalName(mod)+" first to use "+name+".")
	}
	if tab, ok := filterTabs[name]; ok && !m.canFilter(tab) {
		return m.toast.Push(toast.Warning, "Nothing here to "+name+".")
	}
	if !m.actionOpen(action) {
		return m.toast.Push(toast.Warning, "There is nothing to "+name+" here.")
	}
	if readAll {
		if p := m.focused(); m.topModal() == nil && p != nil {
			if cmd := p.section.Update(ui.MarkAllReadMsg{}); cmd != nil {
				return cmd
			}
		}
		return m.toast.Push(toast.Warning, "There is nothing to read here.")
	}
	// An action whose own guard makes the key do nothing here, such as
	// reopening what is open, would leave the line as silent as the key;
	// the line says so.
	before := m.View().Content
	if cmd := m.pressAction(action); cmd != nil || m.View().Content != before {
		return cmd
	}
	return m.toast.Push(toast.Warning, "There is nothing to "+name+" here.")
}

// filterTabs are the commands that open the filter modal of a list, on the
// tab each names.
var filterTabs = map[string]filterform.Tab{"filter": filterform.FiltersTab, "sort": filterform.SortTab}

// canFilter reports whether the focused list of the screen has the tab
// to open, which the key leaves to nothing happening when it has not. A
// modal has its own.
func (m *Model) canFilter(tab filterform.Tab) bool {
	p := m.focused()
	if m.topModal() != nil || p == nil {
		return true
	}
	_, _, ok := filterOf(p.section, tab)
	return ok
}

// actionOpen reports whether the app can do action now, for the actions
// that its own keys gate on what the screen has, which the key leaves
// to nothing happening.
func (m *Model) actionOpen(action string) bool {
	if m.topModal() != nil {
		return true
	}
	switch action {
	case config.ActionHistory:
		return m.canOpenHistory()
	case config.ActionActions:
		return m.canOpenActions()
	case config.ActionStar:
		return m.canStar()
	case config.ActionFindFile:
		return m.fileFinder() != nil
	case config.ActionZoom:
		// The other screens zoom their own panes.
		return m.screen != repoScreen || m.canZoom() && m.width >= narrowWidth
	}
	return true
}

// refusedOverModal reports whether the global action name shows or acts
// on the screen behind the modal, which a modal refuses while it is open
// (see overModals), apart from the owner that a pull request or an issue
// takes as its author's, so that the line doesn't complete it over a
// modal.
func (m *Model) refusedOverModal(name string) bool {
	return name == "find_file" || slices.ContainsFunc(m.overModals(), func(o overModal) bool { return o.action == name })
}

// maxPlaces is the most places a refusal names, before "and N more".
const maxPlaces = 4

// whereActions says where the command name works, by the titles of the
// contexts that have it, such as "Pull requests (Repository) and Pull
// request modal", or "" for a name that no context has.
func (m *Model) whereActions(name string) string {
	var places []string
	for _, c := range config.ActionContexts(m.cfg.Keys, name) {
		if p := contextPlace(c); !slices.Contains(places, p) {
			places = append(places, p)
		}
	}
	more := len(places) - maxPlaces
	if more > 0 {
		places = places[:maxPlaces]
	}
	switch {
	case len(places) == 0:
		return ""
	case more > 0:
		return strings.Join(places, ", ") + " and " + strconv.Itoa(more) + " more"
	case len(places) == 1:
		return places[0]
	}
	return strings.Join(places[:len(places)-1], ", ") + " and " + places[len(places)-1]
}

// contextPlace names the place that c is, as the titles of the contexts
// name it: a pane by the screen or modal it is in, as "Pull requests
// (Repository)", and a screen or a modal by its title and what it is.
func contextPlace(c config.Context) string {
	switch {
	case c.Reach == config.ReachGlobal:
		return "every screen"
	case c.Reach == config.ReachPane:
		if parent, ok := config.LookupContext(c.Parent); ok {
			return c.Title + " (" + parent.Title + ")"
		}
		return c.Title
	case c.Modal:
		return c.Title + " modal"
	}
	return c.Title + " screen"
}

// actionCandidates returns the candidates that complete prefix with the
// actions of the focus: those of the innermost context first, and the
// rest, with the built-in commands that run, by name. A name that a
// built-in command has is its candidate alone.
func (m *Model) actionCandidates(prefix string, start, end int, atEnd bool) []cmdline.Candidate {
	help := m.actionHelp()
	var out []cmdline.Candidate
	seen := map[string]bool{}
	for _, c := range commands {
		seen[c.name] = true
	}
	add := func(name, action string) {
		if !strings.HasPrefix(name, prefix) || seen[name] {
			return
		}
		seen[name] = true
		out = append(out, cmdline.Candidate{Text: name, Label: name, Detail: help[action], Start: start, End: end})
	}
	chain := m.focusChain()
	for _, ctx := range chain[:len(chain)-1] {
		for _, name := range m.actionNames(ctx) {
			add(name, ctx+"."+name)
		}
	}
	var rest []cmdline.Candidate
	for _, c := range commands {
		if !strings.HasPrefix(c.name, prefix) || !m.runsOver(c) {
			continue
		}
		text := c.name
		if c.args && atEnd {
			text += " "
		}
		rest = append(rest, cmdline.Candidate{Text: text, Label: c.name, Detail: c.detail, Start: start, End: end})
	}
	n := len(out)
	for _, name := range m.actionNames(config.ContextGlobal) {
		if m.topModal() != nil && m.refusedOverModal(name) {
			continue
		}
		add(name, config.ContextGlobal+"."+name)
	}
	rest = append(rest, out[n:]...)
	out = out[:n]
	slices.SortStableFunc(rest, func(a, b cmdline.Candidate) int { return cmp.Compare(a.Label, b.Label) })
	return append(out, rest...)
}

// actionNames returns the names of the commands that the actions of ctx
// are, sorted.
func (m *Model) actionNames(ctx string) []string {
	var out []string
	for name := range m.cfg.Keys[ctx] {
		if config.Commandable(ctx + "." + name) {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out
}

// actionHelp returns the words that the keys now name each action with,
// such as "merge" for pulls.merge, which a candidate says beside its name.
func (m *Model) actionHelp() map[string]string {
	out := map[string]string{}
	for _, l := range m.layersNow() {
		for _, b := range l.Bindings {
			for _, a := range keymap.Actions(b) {
				if _, ok := out[a]; !ok && b.Help().Desc != "" {
					out[a] = b.Help().Desc
				}
			}
		}
	}
	return out
}
