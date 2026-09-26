package cmdline

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"
)

// host shows a command line the way a parent view would: it opens it on
// ":", keeps what it submits, and quits when it closes.
type host struct {
	line      Model
	submitted string
	cancelled bool
}

func (h host) Init() tea.Cmd { return nil }

func (h host) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h.line.SetSize(msg.Width, MaxHeight)
		return h, nil
	case SubmitMsg:
		if msg.ID == h.line.ID() {
			h.submitted = msg.Line
			return h, tea.Quit
		}
	case CancelMsg:
		if msg.ID == h.line.ID() {
			h.cancelled = true
			return h, tea.Quit
		}
	case tea.KeyPressMsg:
		if !h.line.Focused() && msg.String() == ":" {
			return h, h.line.Open("")
		}
	}
	var cmd tea.Cmd
	h.line, cmd = h.line.Update(msg)
	return h, cmd
}

func (h host) View() tea.View {
	if !h.line.Focused() {
		return tea.NewView("press : for a command")
	}
	return tea.NewView(h.line.View())
}

func TestProgram(t *testing.T) {
	tests := []struct {
		name          string
		opts          []Option
		typed         string
		keys          []tea.KeyPressMsg
		wantSubmitted string
		wantCancelled bool
	}{
		{
			name:          "submit",
			typed:         "goto cli/cli",
			keys:          []tea.KeyPressMsg{enter},
			wantSubmitted: "goto cli/cli",
		},
		{
			name:          "complete",
			opts:          []Option{WithComplete(repoComplete)},
			typed:         "goto gam",
			keys:          []tea.KeyPressMsg{tab, tab, enter},
			wantSubmitted: "goto gammons/slk-web",
		},
		{
			name:          "history",
			opts:          []Option{WithHistory([]string{"goto cli/cli", "theme dark", "goto gammons/slk"})},
			typed:         "goto",
			keys:          []tea.KeyPressMsg{up, up, enter},
			wantSubmitted: "goto cli/cli",
		},
		{
			name:          "cancel",
			typed:         "goto",
			keys:          []tea.KeyPressMsg{esc},
			wantCancelled: true,
		},
		{
			name:          "backspace out",
			typed:         "g",
			keys:          []tea.KeyPressMsg{bksp, bksp},
			wantCancelled: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tm := teatest.NewTestModel(t, host{line: New(tt.opts...)}, teatest.WithInitialTermSize(80, 4))
			// The renderer may split words with escape sequences, so match
			// the output without them.
			teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
				return strings.Contains(ansi.Strip(string(b)), "press : for a command")
			}, teatest.WithDuration(5*time.Second))
			tm.Send(runeKey(":"))
			// The program handles messages in order, so the keys after the
			// text need no wait.
			tm.Type(tt.typed)
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
			if final.line.Focused() {
				t.Error("the command line is still focused after it closed")
			}
		})
	}
}
