package tui

import (
	"context"
	"log/slog"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"
)

// CommandHistory keeps the lines of the command line between sessions,
// such as in a file of the user's account.
type CommandHistory interface {
	// Load returns the lines kept, oldest first. It may do I/O.
	Load() ([]string, error)
	// Save keeps lines, oldest first, in place of those kept. It may do
	// I/O.
	Save(lines []string) error
}

// WithCommandHistory sets where the lines of the command line are kept
// between sessions. The app loads them as it starts and saves them after
// each command. Without it, the lines last for the session.
func WithCommandHistory(h CommandHistory) Option {
	return func(m *Model) { m.hist = &historyKeeper{store: h} }
}

// historyKeeper saves the history in the order the lines were submitted,
// although the commands that save it may run in any order.
type historyKeeper struct {
	store CommandHistory
	// loaded reports whether the lines kept have been loaded, and dirty
	// whether a line was submitted before, which must wait to be saved
	// with them. Update alone uses them.
	loaded, dirty bool
	// quitting reports whether a command quits once the lines are
	// loaded and saved.
	quitting bool
	// gen numbers the saves in Update, and saved is the last one written.
	gen   int
	mu    sync.Mutex
	saved int
}

// save saves lines as save gen, unless a later one was saved already.
func (h *historyKeeper) save(gen int, lines []string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if gen < h.saved {
		return nil
	}
	h.saved = gen
	return h.store.Save(lines)
}

// historyMsg carries the lines of the command line kept in an earlier
// session.
type historyMsg struct {
	lines []string
}

// loadHistory loads the lines kept, which the command line then recalls.
func (m *Model) loadHistory() tea.Cmd {
	if m.hist == nil {
		return nil
	}
	store, ctx := m.hist.store, m.ctx
	return func() tea.Msg {
		lines, err := store.Load()
		if err != nil {
			// The history only helps, so it starts empty.
			lines = nil
			slog.WarnContext(ctx, "command history unreadable", "span", "tui", "err", err.Error())
		}
		return historyMsg{lines: lines}
	}
}

// loadedHistory puts the lines kept before those submitted meanwhile, and
// saves them all if a line was.
func (m *Model) loadedHistory(msg historyMsg) tea.Cmd {
	if m.hist == nil || m.hist.loaded {
		return nil
	}
	m.hist.loaded = true
	m.line.SetHistory(append(msg.lines, m.line.History()...))
	var save tea.Cmd
	if m.hist.dirty {
		save = m.saveHistory()
	}
	if m.hist.quitting {
		return tea.Sequence(save, tea.Quit)
	}
	return save
}

// historyWait is how long quitting waits for the lines kept to load, so
// that saving doesn't write over them.
const historyWait = 2 * time.Second

// quitWaitedMsg reports that quitting waited long enough for the lines
// kept.
type quitWaitedMsg struct{}

// quit quits the app once save has saved the history. Before the lines
// kept are loaded, it waits for them, up to historyWait, and then quits
// without saving rather than write over them.
func (m *Model) quit(save tea.Cmd) tea.Cmd {
	if m.hist == nil || m.hist.loaded {
		return tea.Sequence(save, tea.Quit)
	}
	m.hist.quitting = true
	return tea.Tick(historyWait, func(time.Time) tea.Msg { return quitWaitedMsg{} })
}

// quitWaited quits if the lines kept never loaded.
func (m *Model) quitWaited() tea.Cmd {
	if m.hist == nil || m.hist.loaded {
		// The load quits once it has saved.
		return nil
	}
	slog.WarnContext(m.ctx, "command history not loaded in time, so not saved", "span", "tui")
	return tea.Quit
}

// saveHistory saves the lines of the command line, once those kept are
// loaded, so that it doesn't write over them.
func (m *Model) saveHistory() tea.Cmd {
	h := m.hist
	if h == nil {
		return nil
	}
	if !h.loaded {
		h.dirty = true
		return nil
	}
	h.gen++
	gen, lines, ctx := h.gen, m.line.History(), m.ctx
	return func() tea.Msg {
		if err := h.save(gen, lines); err != nil {
			logHistoryError(ctx, err)
		}
		return nil
	}
}

func logHistoryError(ctx context.Context, err error) {
	slog.WarnContext(ctx, "command history not saved", "span", "tui", "err", err.Error())
}
