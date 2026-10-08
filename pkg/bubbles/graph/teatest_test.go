package graph

import (
	"fmt"
	"slices"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// app hosts a graph as the program root, the way the tui would. It reports
// what the graph selects and how many commits it loaded on events, since
// the rows look alike and the renderer draws only the cells that changed.
// It quits when a commit is chosen.
type app struct {
	graph  Model
	events chan<- string
	chosen string
}

func (a app) Init() tea.Cmd {
	return a.graph.Init()
}

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.graph.SetSize(msg.Width, msg.Height)
		return a, nil
	case SelectMsg:
		if msg.ID == a.graph.ID() {
			a.events <- "select " + msg.Commit.Title
		}
		return a, nil
	case ChosenMsg:
		if msg.ID == a.graph.ID() {
			a.chosen = msg.Commit.Title
		}
		return a, tea.Quit
	}
	n := a.graph.Len()
	var cmd tea.Cmd
	a.graph, cmd = a.graph.Update(msg)
	if a.graph.Len() != n {
		a.events <- fmt.Sprint("loaded ", a.graph.Len())
	}
	return a, cmd
}

func (a app) View() tea.View {
	return tea.NewView(a.graph.View())
}

func TestProgram(t *testing.T) {
	events := make(chan string, 100)
	m := newModel(newSource(history(60), 20).fetch, WithFocused(true))
	tm := teatest.NewTestModel(t, app{graph: m, events: events}, teatest.WithInitialTermSize(60, 10))

	// waitFor waits for every event in wants, which commands may send in
	// any order.
	waitFor := func(wants ...string) {
		t.Helper()
		timeout := time.After(5 * time.Second)
		for len(wants) > 0 {
			select {
			case e := <-events:
				wants = slices.DeleteFunc(wants, func(w string) bool { return w == e })
			case <-timeout:
				t.Fatalf("no events %q", wants)
			}
		}
	}
	waitFor("loaded 20", "select commit number 59")

	// The end of the first chunk loads the second one below it.
	tm.Send(press("G"))
	waitFor("select commit number 40", "loaded 40")
	tm.Send(press("j"))
	waitFor("select commit number 39")
	tm.Send(press("enter"))

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(app)
	if final.chosen != "commit number 39" {
		t.Fatalf("chosen %q, want commit number 39", final.chosen)
	}
}
