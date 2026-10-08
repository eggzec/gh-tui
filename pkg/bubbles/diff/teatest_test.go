package diff

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// app hosts a diff view as the program root, the way the tui would.
type app struct {
	diff Model
}

func (a app) Init() tea.Cmd { return a.diff.Init() }

func (a app) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.diff.SetSize(msg.Width, msg.Height)
		return a, nil
	case FilesMsg:
		// Keep to the end until the last page, then stop.
		if msg.Done {
			return a, tea.Quit
		}
		return a, func() tea.Msg { return press("G") }
	case tea.KeyPressMsg:
		if msg.String() == "q" {
			return a, tea.Quit
		}
	}
	var cmd tea.Cmd
	a.diff, cmd = a.diff.Update(msg)
	return a, cmd
}

func (a app) View() tea.View { return tea.NewView(a.diff.View()) }

func TestProgram(t *testing.T) {
	files := sizedFiles(12, 4)
	src := &source{size: 3, files: files}
	m := New(src.fetch, WithKeyMap(testKeyMap), WithFocused(true))
	tm := teatest.NewTestModel(t, app{diff: m}, teatest.WithInitialTermSize(80, 12))

	// Every page that arrives takes the cursor to the end, until the last.

	final := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(app)
	if !final.diff.Done() || final.diff.Files() != 12 {
		t.Errorf("done %v with %d files, want all 12", final.diff.Done(), final.diff.Files())
	}
}
