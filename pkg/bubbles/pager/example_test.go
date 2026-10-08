package pager_test

import (
	"fmt"
	"strings"

	"github.com/eggzec/gh-tui/pkg/bubbles/keymap"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eggzec/gh-tui/pkg/bubbles/overlay"
	"github.com/eggzec/gh-tui/pkg/bubbles/pager"
)

func Example() {
	p := pager.New(pager.WithSize(40, 5))
	p.Focus()

	// SetContent shows the text at once and highlights it in the
	// background: run the command, and pass its message to Update.
	highlight := p.SetContent("hello.go", "package main\n\nfunc main() {}\n")
	p, _ = p.Update(highlight())

	printPlain(p.View())
	// Output:
	// 1 package main
	// 2
	// 3 func main() {}
	//
	// hello.go                  line 1/3  100%
}

// modal is a root model that shows a file in a pager over the rest of the
// screen, and closes it when the pager asks.
type modal struct {
	width, height int
	pager         pager.Model
	open          bool
}

func (m modal) Update(msg tea.Msg) (modal, tea.Cmd) {
	switch msg := msg.(type) {
	case pager.CloseMsg:
		if msg.ID == m.pager.ID() {
			m.open = false
			m.pager.Blur()
			return m, nil
		}
	case tea.KeyPressMsg:
		// While it is open, the modal takes every key.
		if !m.open {
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.pager, cmd = m.pager.Update(msg)
	return m, cmd
}

// show opens the modal on a file, with the pager sized to fit inside a
// border over most of the screen.
func (m *modal) show(name, text string) tea.Cmd {
	m.open = true
	// The border and padding take four columns and two rows.
	m.pager.SetSize(m.width*4/5-4, m.height*4/5-2)
	m.pager.Focus()
	return m.pager.SetContent(name, text)
}

func (m modal) view(background string) string {
	if !m.open {
		return background
	}
	box := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1).Render(m.pager.View())
	return overlay.Center(background, box, m.width, m.height)
}

// A root model composites the pager over its own view with the overlay
// package.
func Example_modal() {
	// The parent says which keys do what; here q quits.
	keys := pager.NewKeyMap(keymap.Func(func(action string) []string {
		if action == "global.quit" {
			return []string{"q"}
		}
		return nil
	}))
	m := modal{width: 50, height: 10, pager: pager.New(pager.WithKeyMap(keys))}
	_ = m.show("notes.txt", "Remember the milk.\n")

	bg := lipgloss.NewStyle().Width(m.width).Height(m.height).Render("The rest of the screen.")
	printPlain(m.view(bg))

	// q makes the pager send a CloseMsg, which closes the modal.
	m, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m, _ = m.Update(cmd())
	fmt.Println("open:", m.open)
	// Output:
	// The rest of the screen.
	//      ╭──────────────────────────────────────╮
	//      │ 1 Remember the milk.                 │
	//      │                                      │
	//      │                                      │
	//      │                                      │
	//      │                                      │
	//      │ notes.txt             line 1/1  100% │
	//      ╰──────────────────────────────────────╯
	//
	// open: false
}

// printPlain prints a view without its styles and trailing blanks.
func printPlain(v string) {
	for l := range strings.SplitSeq(ansi.Strip(v), "\n") {
		fmt.Println(strings.TrimRight(l, " "))
	}
}
