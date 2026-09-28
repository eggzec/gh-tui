package tui

import (
	"context"
	"log/slog"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// terminalWait is how long the terminal record waits for the terminal to
// say its version; one that doesn't answer XTVERSION never does.
const terminalWait = time.Second

// maxVersion is the most bytes of the terminal's version logged.
const maxVersion = 128

// terminalWaitMsg says that the terminal had its time to say its version.
type terminalWaitMsg struct{}

// terminal is what the first messages of the program say of the terminal,
// which the terminal record logs once: its size, its color profile, and
// its name and version, if it says them.
type terminal struct {
	width, height int
	sized         bool
	profile       string
	version       string
	// waited is set once the version came, or the wait for it ended.
	waited bool
	logged bool
}

// requestTerminal asks the terminal for its version, and ends the wait
// for it after terminalWait.
func requestTerminal() tea.Cmd {
	return tea.Batch(tea.RequestTerminalVersion, tea.Tick(terminalWait, func(time.Time) tea.Msg { return terminalWaitMsg{} }))
}

// observe learns from msg what it says of the terminal, and logs the
// terminal record once it knows the size and the profile and has waited
// for the version.
func (t *terminal) observe(ctx context.Context, msg tea.Msg) {
	if t.logged {
		return
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if t.sized {
			return
		}
		t.width, t.height, t.sized = msg.Width, msg.Height, true
	case tea.ColorProfileMsg:
		t.profile = msg.String()
	case tea.TerminalVersionMsg:
		// The terminal writes what it likes, so only a line of it is kept.
		v := termtext.OneLine(msg.Name)
		if len(v) > maxVersion {
			v = strings.ToValidUTF8(v[:maxVersion], "")
		}
		t.version, t.waited = v, true
	case terminalWaitMsg:
		t.waited = true
	default:
		return
	}
	if !t.sized || t.profile == "" || !t.waited {
		return
	}
	t.logged = true
	attrs := []slog.Attr{
		slog.String("span", "tui"),
		slog.Int("width", t.width),
		slog.Int("height", t.height),
		slog.String("color_profile", t.profile),
	}
	if t.version != "" {
		attrs = append(attrs, slog.String("xtversion", t.version))
	}
	slog.LogAttrs(ctx, slog.LevelInfo, "terminal", attrs...)
}
