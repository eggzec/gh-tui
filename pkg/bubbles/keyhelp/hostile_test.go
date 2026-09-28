package keyhelp

import (
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/termtext/termtexttest"
)

// Text from outside, such as a title that names a pull request, reaches
// the terminal clean: the title, a source, a description and a captured
// key.
func TestViewCleansHostileText(t *testing.T) {
	h := termtexttest.Hostile
	b := key.NewBinding(key.WithKeys("x"), key.WithHelp("x", h))
	layers := []Layer{{Source: h, Bindings: []key.Binding{b}}, {Source: "app", Bindings: []key.Binding{b}}}
	for _, w := range []int{30, 60, 200} {
		m := New(WithLayers(layers), WithTitle("Help · "+h), WithSize(w, 8))
		m.Focus()
		termtexttest.AssertClean(t, m.View(), w)
		m, _ = press(t, m, tea.KeyPressMsg{Code: tea.KeyTab}, tea.KeyPressMsg{Code: '\u202e', Text: "\u202e"})
		termtexttest.AssertClean(t, m.View(), w)
	}
}
