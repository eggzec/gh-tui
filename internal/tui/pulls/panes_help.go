package pulls

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
	"github.com/eggzec/gh-tui/pkg/bubbles/keyhelp"
	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"
	"github.com/eggzec/gh-tui/pkg/bubbles/panes"
)

// OpenHelp implements ui.HelpHost: the help of layers opens over the panes,
// and over an overlay that is open, as a viewer. Where an overlay types, the
// app does not offer the help key, so this is never asked then.
func (m *panesModal) OpenHelp(layers []keyhelp.Layer, title string) (tea.Cmd, bool) {
	h := keyhelp.New(keyhelp.WithKeyMap(m.keys.helpKeys), keyhelp.WithStyles(m.theme.KeyHelp(m.icons)))
	h.SetLayers(layers)
	h.SetTitle(title)
	o := &helpOverlay{m: m, h: h}
	m.open(o)
	return o.h.Focus(), true
}

// helpOverlay lists the keys that work now. It is a viewer, and takes every
// key itself: its own keys close it.
type helpOverlay struct {
	m    *panesModal
	h    keyhelp.Model
	done bool
}

func (o *helpOverlay) Title() string           { return "Help" }
func (o *helpOverlay) Kind() panes.OverlayKind { return panes.Viewer }
func (o *helpOverlay) Items() int              { return 0 }
func (o *helpOverlay) Text() int               { return 0 }
func (o *helpOverlay) Clear() bool             { return false }
func (o *helpOverlay) Closed() bool            { return o.done }

// typing reports that the overlay takes the keys of the app's actions as text,
// since the help filters its list by what is typed.
func (o *helpOverlay) Typing() bool { return true }

func (o *helpOverlay) Layers() []keyhelp.Layer {
	return []keyhelp.Layer{ui.ContextHelp("help", o.h, true)}
}

func (o *helpOverlay) View(w, h int) string {
	o.h.SetSize(w, h)
	return o.h.View()
}

func (o *helpOverlay) Update(msg tea.Msg) tea.Cmd {
	if c, ok := msg.(keyhelp.CloseMsg); ok {
		if c.ID == o.h.ID() {
			o.done = true
		}
		return nil
	}
	// The help's own keys close it with esc, and backspace goes back like it
	// does from any viewer, unless it has a query or a key to clear first.
	if press, ok := msg.(tea.KeyPressMsg); ok && !o.h.Capturing() && o.h.Query() == "" && o.h.Key() == "" && keymap.Matches(press, o.m.keys.cancel) {
		o.done = true
		return nil
	}
	var cmd tea.Cmd
	o.h, cmd = o.h.Update(msg)
	return cmd
}
