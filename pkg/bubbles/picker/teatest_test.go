package picker

import (
	"bytes"
	"sync"
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
	// listed runs once the picker lists the results of what it waits for,
	// if set.
	want   string
	listed func()
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
	if h.listed != nil && h.picker.Query().Text == h.want && !h.picker.Loading() && h.picker.Len() > 0 {
		h.listed()
	}
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
			listed := make(chan struct{})
			h := host{picker: m, want: "crash", listed: sync.OnceFunc(func() { close(listed) })}
			tm := teatest.NewTestModel(t, h, teatest.WithInitialTermSize(70, 12))
			teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
				return bytes.Contains(b, []byte("2 results"))
			}, teatest.WithDuration(5*time.Second))
			// Every prefix of the query lists #7 too, and a slow key may let
			// the debounce search one, so wait for the results of the query
			// itself before moving through them.
			tm.Type("crash")
			select {
			case <-listed:
			case <-time.After(5 * time.Second):
				t.Fatal("the picker listed nothing for crash")
			}
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
			// A slow key may let the debounce search a prefix too, which
			// TestDebounce covers, so only the first and last are known.
			if got := f.Queries(); len(got) < 2 || got[0].Text != "" || got[len(got)-1].Text != "crash" {
				t.Errorf("queries = %+v, want the empty one first and one for crash last", got)
			}
		})
	}
}
