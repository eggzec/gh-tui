package finder

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// host shows a finder the way the root would: it focuses it, keeps what
// the user picks, and quits when the finder is done.
type host struct {
	finder    Model
	chosen    Item
	cancelled bool
}

func (h host) Init() tea.Cmd { return h.finder.Init() }

func (h host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.finder.SetSize(msg.Width, msg.Height)
		return h, nil
	case ChosenMsg:
		if msg.ID == h.finder.ID() {
			h.chosen = msg.Item
			return h, tea.Quit
		}
	case CancelMsg:
		if msg.ID == h.finder.ID() {
			h.cancelled = true
			return h, tea.Quit
		}
	}
	var cmd tea.Cmd
	h.finder, cmd = h.finder.Update(msg)
	return h, cmd
}

func (h host) View() tea.View { return tea.NewView(h.finder.View()) }

// TestProgram types a query into a finder of many paths, matched in
// commands, and picks a match. The query is typed at once, so the
// matches of its keys race each other; only the last may be shown.
func TestProgram(t *testing.T) {
	paths := append(bigPaths(30_000), "pkg/tea/qqq_renderer.go")
	tests := []struct {
		name          string
		keys          []tea.KeyPressMsg
		wantChosen    string
		wantCancelled bool
	}{
		{name: "choose", keys: []tea.KeyPressMsg{press("enter")}, wantChosen: "pkg/tea/qqq_renderer.go"},
		{name: "cancel", keys: []tea.KeyPressMsg{press("esc")}, wantCancelled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := New(loader(paths...))
			m.Focus()
			tm := teatest.NewTestModel(t, host{finder: m}, teatest.WithInitialTermSize(70, 12))
			teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
				return bytes.Contains(b, []byte("30001 files"))
			}, teatest.WithDuration(5*time.Second))
			tm.Type("qqqrend")
			teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
				return bytes.Contains(b, []byte(" · 1 match"))
			}, teatest.WithDuration(5*time.Second))
			for _, k := range tt.keys {
				tm.Send(k)
			}
			final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(host)
			if !ok {
				t.Fatal("final model is not a host")
			}
			if final.chosen.Path != tt.wantChosen || final.cancelled != tt.wantCancelled {
				t.Errorf("chosen %q, cancelled %v; want %q, %v", final.chosen.Path, final.cancelled, tt.wantChosen, tt.wantCancelled)
			}
			if final.finder.Query() != "qqqrend" || final.finder.Matches() != 1 {
				t.Errorf("query %q with %d matches", final.finder.Query(), final.finder.Matches())
			}
		})
	}
}
