// Package toast shows transient, non-blocking notifications such as the
// error reported when an optimistic update is rolled back.
//
// The parent keeps a [Model], calls [Model.Push] to show a toast and runs the
// command it returns, forwards messages to [Model.Update], and composites the
// stack over its layout with [Model.Overlay].
package toast

import (
	"slices"
	"strings"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eggzec/gh-tui/pkg/termtext"
)

// Level is the severity of a toast.
type Level int

// The levels, from least to most severe.
const (
	Info Level = iota
	Success
	Warning
	Error
)

// String returns the lower-case name of the level.
func (l Level) String() string {
	switch l {
	case Info:
		return "info"
	case Success:
		return "success"
	case Warning:
		return "warning"
	case Error:
		return "error"
	default:
		return "unknown"
	}
}

func (l Level) valid() bool { return l >= Info && l <= Error }

const (
	defaultDuration      = 4 * time.Second
	defaultErrorDuration = 8 * time.Second
	defaultMax           = 3
)

var lastID atomic.Int64

func nextID() int { return int(lastID.Add(1)) }

type toast struct {
	level Level
	text  string
	count int
	// seq identifies the latest push of this toast, so that the timer of an
	// earlier push can't expire a refreshed toast.
	seq int
}

// Model is a stack of toasts, oldest first.
type Model struct {
	id            int
	seq           int
	toasts        []toast
	max           int
	duration      time.Duration
	errorDuration time.Duration
	// rooms are indexed by Level.
	rooms         [Error + 1]Room
	width, height int
	keys          KeyMap
	styles        Styles
	derived       derivedStyles
	// view is rendered whenever the state changes, so View is free.
	view string
}

// New returns an empty stack with dark styles and the default key map.
func New(opts ...Option) Model {
	m := Model{
		id:            nextID(),
		max:           defaultMax,
		duration:      defaultDuration,
		errorDuration: defaultErrorDuration,
		rooms:         defaultRooms(),
		keys:          DefaultKeyMap(),
	}
	m.SetStyles(DefaultStyles(true))
	for _, opt := range opts {
		opt(&m)
	}
	m.changed()
	return m
}

// ID returns the identifier that scopes this instance's messages.
func (m Model) ID() int { return m.id }

// Init implements the Elm architecture. A toast stack starts empty, so it
// has nothing to do.
func (m Model) Init() tea.Cmd { return nil }

// Push shows a toast and returns the command that expires it. A toast with
// the same level and text that is still visible is refreshed instead: it
// moves to the bottom, its count goes up, and its timer starts again.
func (m *Model) Push(level Level, text string) tea.Cmd {
	if !level.valid() {
		level = Info
	}
	text = clean(text)
	m.seq++
	t := toast{level: level, text: text, count: 1, seq: m.seq}
	rest := m.toasts
	for i, old := range m.toasts {
		if old.level == level && old.text == text {
			t.count = old.count + 1
			rest = without(m.toasts, i)
			break
		}
	}
	// Keep the newest m.max, counting the new one.
	rest = rest[max(len(rest)-m.max+1, 0):]
	m.toasts = slices.Concat(rest, []toast{t})
	m.changed()
	return m.expireAfter(t)
}

// Dismiss removes the newest toast. Its pending timer becomes a no-op. It
// returns no command today; the signature matches Push so callers can batch
// either.
func (m *Model) Dismiss() tea.Cmd {
	if len(m.toasts) == 0 {
		return nil
	}
	m.toasts = without(m.toasts, len(m.toasts)-1)
	m.changed()
	return nil
}

// Clear removes every toast.
func (m *Model) Clear() {
	m.toasts = nil
	m.changed()
}

// Empty reports whether there is nothing to show.
func (m Model) Empty() bool { return len(m.toasts) == 0 }

// Len returns the number of visible toasts.
func (m Model) Len() int { return len(m.toasts) }

// Max returns the most toasts shown at once.
func (m Model) Max() int { return m.max }

// SetMax sets the most toasts shown at once, dropping the oldest if there
// are more. Values below one are treated as one.
func (m *Model) SetMax(n int) {
	m.max = max(n, 1)
	m.trim()
	m.changed()
}

// Duration returns how long info, success and warning toasts stay.
func (m Model) Duration() time.Duration { return m.duration }

// SetDuration sets how long info, success and warning toasts stay. Zero or
// less keeps them until they are dismissed. It applies to later pushes.
func (m *Model) SetDuration(d time.Duration) { m.duration = d }

// ErrorDuration returns how long error toasts stay.
func (m Model) ErrorDuration() time.Duration { return m.errorDuration }

// SetErrorDuration sets how long error toasts stay. Zero or less keeps them
// until they are dismissed. It applies to later pushes.
func (m *Model) SetErrorDuration(d time.Duration) { m.errorDuration = d }

// Width returns the width of the area the stack is placed in.
func (m Model) Width() int { return m.width }

// Height returns the height of the area the stack is placed in.
func (m Model) Height() int { return m.height }

// SetSize sets the area the stack is placed in. The stack takes the share of
// the width that the room of its toasts allows. A height of zero or less
// doesn't limit it.
func (m *Model) SetSize(width, height int) {
	m.width, m.height = width, height
	m.changed()
}

// SetWidth sets the width of the area the stack is placed in.
func (m *Model) SetWidth(width int) { m.SetSize(width, m.height) }

// SetHeight sets the height of the area the stack is placed in.
func (m *Model) SetHeight(height int) { m.SetSize(m.width, height) }

// KeyMap returns the key bindings.
func (m Model) KeyMap() KeyMap { return m.keys }

// SetKeyMap sets the key bindings.
func (m *Model) SetKeyMap(k KeyMap) {
	m.keys = k
	m.changed()
}

func (m Model) expireAfter(t toast) tea.Cmd {
	d := m.duration
	if t.level == Error {
		d = m.errorDuration
	}
	if d <= 0 {
		return nil
	}
	id, seq := m.id, t.seq
	return tea.Tick(d, func(time.Time) tea.Msg {
		return ExpireMsg{id: id, seq: seq}
	})
}

func (m *Model) expire(seq int) {
	for i, t := range m.toasts {
		if t.seq == seq {
			m.toasts = without(m.toasts, i)
			m.changed()
			return
		}
	}
}

// trim drops the oldest toasts beyond the maximum.
func (m *Model) trim() {
	m.toasts = m.toasts[max(len(m.toasts)-m.max, 0):]
}

// without returns a new slice without the toast at i. Toasts are never
// changed in place, because copies of a Model share them.
func without(ts []toast, i int) []toast {
	return slices.Concat(ts[:i], ts[i+1:])
}

// changed brings everything derived from the toasts up to date.
func (m *Model) changed() {
	m.keys.Dismiss.SetEnabled(len(m.toasts) > 0)
	m.view = m.render()
}

// clean puts text on one line without escape sequences, so that it can't
// break the layout it is drawn over.
func clean(text string) string {
	return strings.Join(strings.Fields(termtext.OneLine(text)), " ")
}
