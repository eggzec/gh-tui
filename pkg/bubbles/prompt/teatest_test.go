package prompt

import (
	"bytes"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// host shows a prompt the way a parent view would: it focuses it, keeps
// what it submits, and quits when the prompt is done.
type host struct {
	prompt    Model
	submitted string
	cancelled bool
}

func (h host) Init() tea.Cmd { return nil }

func (h host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.prompt.SetSize(msg.Width, msg.Height)
		return h, nil
	case SubmitMsg:
		if msg.ID == h.prompt.ID() {
			h.submitted = msg.Value
			return h, tea.Quit
		}
	case CancelMsg:
		if msg.ID == h.prompt.ID() {
			h.cancelled = true
			return h, tea.Quit
		}
	}
	var cmd tea.Cmd
	h.prompt, cmd = h.prompt.Update(msg)
	return h, cmd
}

func (h host) View() tea.View { return tea.NewView(h.prompt.View()) }

func TestProgram(t *testing.T) {
	tests := []struct {
		name          string
		opts          []Option
		keys          []tea.KeyPressMsg
		wantSubmitted string
		wantCancelled bool
	}{
		{
			name:          "comment",
			keys:          []tea.KeyPressMsg{enter, runeKey("q"), ctrlS},
			wantSubmitted: "Fixed, thanks!\nq",
		},
		{
			name:          "labels",
			opts:          []Option{WithMode(SingleLine), WithValue("bug")},
			keys:          []tea.KeyPressMsg{enter},
			wantSubmitted: "bugFixed, thanks!",
		},
		{
			name:          "cancel",
			keys:          []tea.KeyPressMsg{esc},
			wantCancelled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := newKeyed(t, append(tt.opts, WithTitle("Comment on #7"))...)
			m.Focus()
			tm := teatest.NewTestModel(t, host{prompt: m}, teatest.WithInitialTermSize(60, 8))
			teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
				return bytes.Contains(b, []byte("Comment on #7"))
			}, teatest.WithDuration(5*time.Second))
			// The program handles messages in order, so the keys after the
			// text need no wait; the renderer only draws what changed, so
			// the output couldn't show the text whole anyway.
			tm.Type("Fixed, thanks!")
			for _, k := range tt.keys {
				tm.Send(k)
			}
			final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(host)
			if !ok {
				t.Fatal("final model is not a host")
			}
			if final.submitted != tt.wantSubmitted || final.cancelled != tt.wantCancelled {
				t.Errorf("submitted %q, cancelled %v; want %q, %v",
					final.submitted, final.cancelled, tt.wantSubmitted, tt.wantCancelled)
			}
		})
	}
}
