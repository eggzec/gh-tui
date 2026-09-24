package picker

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// host shows a picker the way the root would: it focuses it, keeps what the
// user picks, and quits when the picker is done.
type host struct {
	picker    Model
	chosen    Item
	cancelled bool
}

func (h host) Init() tea.Cmd { return h.picker.Init() }

func (h host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.picker.SetSize(msg.Width, msg.Height)
		return h, nil
	case ChosenMsg:
		if msg.ID == h.picker.ID() {
			h.chosen = msg.Item
			return h, tea.Quit
		}
	case CancelMsg:
		if msg.ID == h.picker.ID() {
			h.cancelled = true
			return h, tea.Quit
		}
	}
	var cmd tea.Cmd
	h.picker, cmd = h.picker.Update(msg)
	return h, cmd
}

func (h host) View() tea.View { return tea.NewView(h.picker.View()) }

func TestProgram(t *testing.T) {
	tests := []struct {
		name          string
		keys          []tea.KeyPressMsg
		wantChosen    Item
		wantCancelled bool
	}{
		{name: "choose", keys: []tea.KeyPressMsg{down, enter}, wantChosen: fix},
		{name: "cancel", keys: []tea.KeyPressMsg{esc}, wantCancelled: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSearch{}
			m := New(f.search, WithDebounce(20*time.Millisecond), WithScopes(kindRepos, kindIssues, kindPulls))
			m.Focus()
			tm := teatest.NewTestModel(t, host{picker: m}, teatest.WithInitialTermSize(70, 12))
			teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
				return bytes.Contains(b, []byte("2 results"))
			}, teatest.WithDuration(5*time.Second))
			tm.Type("crash")
			teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
				return bytes.Contains(b, []byte("eggzec/gh-tui#7"))
			}, teatest.WithDuration(5*time.Second))
			for _, k := range tt.keys {
				tm.Send(k)
			}
			final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(host)
			if !ok {
				t.Fatal("final model is not a host")
			}
			if final.chosen != tt.wantChosen || final.cancelled != tt.wantCancelled {
				t.Errorf("chosen %+v, cancelled %v; want %+v, %v",
					final.chosen, final.cancelled, tt.wantChosen, tt.wantCancelled)
			}
			if got := f.Queries(); len(got) != 2 || got[1].Text != "crash" {
				t.Errorf("queries = %+v, want the empty one and one for crash", got)
			}
		})
	}
}
