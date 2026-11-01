package treeview

import (
	"bytes"
	"errors"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// app hosts a tree as the program root, the way the tui would.
type app struct{ tree Model }

func (a app) Init() tea.Cmd { return a.tree.Init() }

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyPressMsg); ok && k.String() == "q" {
		return a, tea.Quit
	}
	var cmd tea.Cmd
	a.tree, cmd = a.tree.Update(msg)
	return a, cmd
}

func (a app) View() tea.View { return tea.NewView(a.tree.View()) }

func TestProgramLoadsLazily(t *testing.T) {
	w := &world{}
	m := New(Lazy(w.load), WithKeyMap(testKeyMap), WithFocused(true), WithSize(40, 10))
	tm := teatest.NewTestModel(t, app{tree: m}, teatest.WithInitialTermSize(40, 10))
	waitFor := func(s string) {
		t.Helper()
		teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
			return bytes.Contains(b, []byte(s))
		}, teatest.WithDuration(5*time.Second))
	}
	waitFor("b")
	tm.Send(press("l"))
	waitFor("a2")
	tm.Send(press("q"))
	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(app)
	if got := visible(final.tree); !slices.Equal(got, []string{"a", "a1", "a2", "b"}) {
		t.Fatalf("rows = %v", got)
	}
}

func TestIgnoresOtherInstances(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	other := New(Lazy(w.load))
	m, _ = m.Update(childrenMsg{Tree: other.id, ID: "a", Seq: 1, Kids: []Node{kid("zz", false)}})
	if got := visible(m); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("another tree's result changed the rows: %v", got)
	}
}

func TestResetForgetsReveal(t *testing.T) {
	w := &world{}
	m := lazyTree(w)
	_, _ = m.Reveal("a", "a2", "x")
	m = run(m, m.Reset())
	if selected(m) == "x" {
		t.Errorf("the reveal went on after Reset; rows %v", visible(m))
	}
}

func TestViewLazyStates(t *testing.T) {
	w := &world{fail: map[string]error{"a": errors.New("boom")}}
	m := lazyTree(w, WithSize(30, 6))
	m = pressRun(t, m, "l", "j")
	v := m.View()
	assertFits(t, v, 30, 6)
	golden.RequireEqual(t, plain(v))
}

func TestViewFiltered(t *testing.T) {
	w := &world{}
	m := lazyTree(w, WithSize(30, 6))
	m = pressRun(t, m, "*")
	m.SetFilter("a2")
	v := m.View()
	assertFits(t, v, 30, 6)
	golden.RequireEqual(t, plain(v))
}
