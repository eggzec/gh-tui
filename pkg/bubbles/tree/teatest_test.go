package tree

import (
	"bytes"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// app hosts a tree as the program root, the way the tui would. It records
// the leaf it is asked to open and quits.
type app struct {
	tree   Model
	opened []string
	// expanded, if set, runs once * has been pressed and every load of the
	// expand-all it started has landed.
	expanding *bool
	expanded  func()
}

func (a app) Init() tea.Cmd {
	return a.tree.Init()
}

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.tree.SetSize(msg.Width, msg.Height)
		return a, nil
	case tea.KeyPressMsg:
		if msg.String() == "q" {
			return a, tea.Quit
		}
	case OpenMsg:
		if msg.ID != a.tree.ID() {
			return a, nil
		}
		a.opened = append(a.opened, msg.Node.ID)
		return a, tea.Quit
	}
	var cmd tea.Cmd
	a.tree, cmd = a.tree.Update(msg)
	if a.expanded != nil {
		if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "*" {
			*a.expanding = true
		}
		if *a.expanding && !a.tree.bulk.active() {
			a.expanded()
		}
	}
	return a, cmd
}

func (a app) View() tea.View {
	return tea.NewView(a.tree.View())
}

func TestProgram(t *testing.T) {
	m := newModel(repo().children, WithFocused(true))
	expanded := make(chan struct{})
	a := app{tree: m, expanding: new(bool), expanded: sync.OnceFunc(func() { close(expanded) })}
	tm := teatest.NewTestModel(t, a, teatest.WithInitialTermSize(80, 12))

	waitFor := func(s string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(s))
		}, teatest.WithDuration(5*time.Second))
	}
	waitFor("README.md")

	tm.Send(press("j"))
	tm.Send(press("j"))
	tm.Send(press("*"))
	// The expand-all loads the branches of internal at once, so wait for
	// all of them: tui's app.go may show while core still loads.
	select {
	case <-expanded:
	case <-time.After(5 * time.Second):
		t.Fatal("the expand-all never finished")
	}
	for range 3 {
		tm.Send(press("j"))
	}
	tm.Send(press("enter"))

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(app)
	if len(final.opened) != 1 || final.opened[0] != "internal/core/repo.go" {
		t.Fatalf("opened %v, want [internal/core/repo.go]", final.opened)
	}
}
