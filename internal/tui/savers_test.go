package tui

import (
	"errors"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/internal/tui/ui"
)

// savingModal is a modal that holds unsent text.
type savingModal struct {
	fakeModal
	saved int
	err   error
}

func (s *savingModal) Save() error { s.saved++; return s.err }

// TestQuitSavesUnsentText checks that ctrl+c and the q command save what the
// open modal holds, without asking, and quit even if saving fails.
func TestQuitSavesUnsentText(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		quit func(*Model) tea.Cmd
	}{
		{"ctrl+c", func(m *Model) tea.Cmd { return m.key(ctrlC) }},
		{"q command", func(m *Model) tea.Cmd { return m.runLine("q", nil) }},
	} {
		for _, failing := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s failing=%v", tt.name, failing), func(t *testing.T) {
				t.Parallel()
				m, _ := newTestApp(t)
				mod := &savingModal{}
				mod.title = "Comment"
				if failing {
					mod.err = errors.New("disk full")
				}
				run(m, ui.OpenModal(mod))
				cmd := tt.quit(m)
				if mod.saved != 1 {
					t.Errorf("Save called %d times, want 1", mod.saved)
				}
				if !quits(cmd) {
					t.Error("didn't quit")
				}
			})
		}
	}
}
