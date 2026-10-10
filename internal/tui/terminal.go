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
// say its version; one that doesn't answer XTVERSION never does. It is a
// variable so that the tests, which run the wait to the end, can shorten it.
var terminalWait = time.Second

// maxVersion is the most bytes of the terminal's version logged.
const maxVersion = 128

// terminalWaitMsg says that the terminal had its time to say its version.
type terminalWaitMsg struct{}

// terminal is what the first messages of the program say of the terminal,
// which the terminal record logs once: its size, its color profile, its
// name and version, if it says them, and whether it shows images.
type terminal struct {
	width, height int
	sized         bool
	profile       string
	version       string
	// waited is set once the version came, or the wait for it ended.
	waited bool
	// images are the fields of the images verdict, once it came.
	images  []slog.Attr
	decided bool
	logged  bool
}

// waitTerminal ends the wait for the terminal's version after
// terminalWait. The images probe asks for it, with its own questions.
func waitTerminal() tea.Cmd {
	return tea.Tick(terminalWait, func(time.Time) tea.Msg { return terminalWaitMsg{} })
}

// imagesDecided takes attrs, the fields of the images verdict, and logs
// the terminal record if it waited only for them. A verdict that comes
// after the record, as one does when a better color profile starts the
// probe again, is logged on its own.
func (t *terminal) imagesDecided(ctx context.Context, attrs []slog.Attr) {
	if t.logged {
		slog.LogAttrs(ctx, slog.LevelInfo, "images", append([]slog.Attr{slog.String("span", "tui")}, attrs...)...)
		return
	}
	t.images, t.decided = attrs, true
	t.log(ctx)
}

// observe learns from msg what it says of the terminal, and logs the
// terminal record once it knows the size and the profile, has waited for
// the version, and knows whether images are drawn: the probe's verdict
// comes within its own timeout.
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
		t.version, t.waited = terminalName(msg.Name), true
	case terminalWaitMsg:
		t.waited = true
	default:
		return
	}
	t.log(ctx)
}

// log logs the terminal record, once it knows all it waits for.
func (t *terminal) log(ctx context.Context) {
	if t.logged || !t.sized || t.profile == "" || !t.waited || !t.decided {
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
	attrs = append(attrs, t.images...)
	slog.LogAttrs(ctx, slog.LevelInfo, "terminal", attrs...)
}

// terminalName keeps a line of name, as a terminal said it, at most
// maxVersion bytes long: the terminal writes what it likes.
func terminalName(name string) string {
	name = termtext.OneLine(name)
	if len(name) > maxVersion {
		name = strings.ToValidUTF8(name[:maxVersion], "")
	}
	return name
}
