package tui

import (
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
)

// The help lists the keys of what has them when it opens, over the modal
// and under the toasts. It isn't a modal, so it leaves the open one be,
// and the help key reaches it from inside one, unless what has the keys
// types it into an input.

// newHelp returns the help, which closes with the help key too.
func newHelp(k KeyMap) keyhelp.Model {
	km := keyhelp.DefaultKeyMap()
	km.Close = key.NewBinding(key.WithKeys(k.Help.Keys()...), key.WithHelp(k.Help.Help().Key, "close"))
	return keyhelp.New(keyhelp.WithKeyMap(km))
}

// helpOpen reports whether the help is open.
func (m *Model) helpOpen() bool { return m.keyhelp.Focused() }

// openHelp opens the help on the keys that reach what has them now.
func (m *Model) openHelp() tea.Cmd {
	m.keyhelp.SetLayers(m.keyLayers())
	m.keyhelp.SetTitle(m.helpTitle())
	m.keyhelp.Reset()
	m.keyhelp.SetSize(m.helpSize())
	return m.keyhelp.Focus()
}

// updateHelp passes msg to the open help.
func (m *Model) updateHelp(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	m.keyhelp, cmd = m.keyhelp.Update(msg)
	return cmd
}

// helpTitle names the help for what has the keys: the open modal, or the
// focused section.
func (m *Model) helpTitle() string {
	var name string
	if mod := m.topModal(); mod != nil {
		name = mod.Title()
	} else if p := m.focused(); p != nil {
		name = p.section.Title()
	}
	// A modal's title may hold text from GitHub.
	if name = ui.OneLine(name); name == "" {
		return "Help"
	}
	return "Help" + m.icons.Separator + name
}

// helpFrameSize is the size of the help with its frame: most of the
// screen, as a modal's is.
func (m *Model) helpFrameSize() (width, height int) {
	return max(m.width-2*max(m.width/10, 2), 0), max(m.height-2*max(m.height/10, 1), 0)
}

// helpSize is the size of the help inside its frame.
func (m *Model) helpSize() (width, height int) {
	w, h := m.helpFrameSize()
	return max(w-4, 0), max(h-2, 0)
}

// helpFrame draws the help in a frame like a modal's, whose top edge is
// plain, as the help shows its title in its first line.
func (m *Model) helpFrame() string {
	w, _ := m.helpFrameSize()
	if w < 4 {
		return ""
	}
	b := m.icons.Border
	top := m.theme.Accent.Render(b.TopLeft + strings.Repeat(b.Top, w-2) + b.TopRight)
	return top + "\n" + m.frameStyle().Render(m.keyhelp.View())
}

// keyLayers returns the keys that reach something now, as layersNow
// finds them, with their help naming the keys in the words of the icon
// set, as the hints and the help show them.
func (m *Model) keyLayers() []keyhelp.Layer {
	return ui.NameLayerKeys(m.icons, m.layersNow())
}

// layersNow returns the keys that reach something now, in the order a key
// reaches them. The open command line takes every key, ctrl+c too.
// Otherwise the help key comes first, and ctrl+c, which quits from the
// help and a modal, since they take every other key. Then come the open
// help's, or else an open modal's, or else, while the focused section
// captures keys, the app's that hold ctrl+c and the section's, or else
// the keys the section claims, then the app's, and then the section's.
func (m *Model) layersNow() []keyhelp.Layer {
	if m.line.Focused() {
		return []keyhelp.Layer{keyhelp.FromHelp("command line", m.line, true)}
	}
	quit := keyhelp.Layer{Source: "app", Bindings: []key.Binding{forceQuit}}
	if m.helpOpen() {
		return []keyhelp.Layer{quit, keyhelp.FromHelp("help", m.keyhelp, true)}
	}
	inner, modal := m.innerLayers()
	help := m.helpKey(inner)
	always := keyhelp.Layer{Source: "app", Short: []key.Binding{help}}
	if modal {
		always.Bindings = append(always.Bindings, forceQuit)
		if m.commandsOver(m.topModal()) {
			always.Bindings = append(always.Bindings, m.keys.Command)
		}
	}
	always.Bindings = append(always.Bindings, help)
	return append([]keyhelp.Layer{always}, inner...)
}

// innerLayers returns the layers after the app's always ones, and whether
// they are an open modal's.
func (m *Model) innerLayers() (layers []keyhelp.Layer, modal bool) {
	if mod := m.topModal(); mod != nil {
		return mod.KeyLayers(), true
	}
	app := keyhelp.FromHelp("app", m.keys.state(m), false)
	// The help key comes before them all.
	isHelp := func(b key.Binding) bool { return sameBinding(b, m.keys.Help) }
	app.Bindings = slices.DeleteFunc(app.Bindings, isHelp)
	app.Short = slices.DeleteFunc(app.Short, isHelp)
	p := m.focused()
	if p == nil {
		return []keyhelp.Layer{app}, false
	}
	if c, ok := p.section.(ui.Capturer); ok && c.Capturing() {
		// ctrl+c passes a capturing section by, to the app's keys, so
		// that it can't trap the user.
		return append([]keyhelp.Layer{quitReach(app)}, p.section.KeyLayers()...), false
	}
	if c, ok := p.section.(ui.Claimer); ok {
		layers = append(layers, keyhelp.Layer{Source: p.section.Title(), Bindings: c.Claimed()})
	}
	layers = append(layers, app)
	return append(layers, p.section.KeyLayers()...), false
}

// quitReach returns the bindings of l that ctrl+c reaches, with that key
// alone.
func quitReach(l keyhelp.Layer) keyhelp.Layer {
	out := keyhelp.Layer{Source: l.Source}
	for _, b := range l.Bindings {
		if b.Enabled() && slices.Contains(b.Keys(), forceQuit.Keys()[0]) {
			out.Bindings = append(out.Bindings, key.NewBinding(key.WithKeys(forceQuit.Keys()...), key.WithHelp(forceQuit.Help().Key, b.Help().Desc)))
		}
	}
	return out
}

// helpKey returns the help key as it works before layers: without the
// keys that type a character while one of the layers types into an
// input, so that a ? typed into a query stays there.
func (m *Model) helpKey(layers []keyhelp.Layer) key.Binding {
	b := m.keys.Help
	if !slices.ContainsFunc(layers, func(l keyhelp.Layer) bool { return l.Typing }) {
		return b
	}
	keys := slices.DeleteFunc(slices.Clone(b.Keys()), keyhelp.Printable)
	switch {
	case len(keys) == len(b.Keys()):
		return b
	case len(keys) == 0:
		b.SetEnabled(false)
		return b
	}
	return key.NewBinding(key.WithKeys(keys...), key.WithHelp(strings.Join(keys, "/"), b.Help().Desc))
}

// opensHelp reports whether msg opens the help from where the keys are.
func (m *Model) opensHelp(msg tea.KeyPressMsg) bool {
	if !key.Matches(msg, m.keys.Help) {
		return false
	}
	inner, _ := m.innerLayers()
	return key.Matches(msg, m.helpKey(inner))
}
